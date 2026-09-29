package jobs

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

// CommandFunc builds a job's process, and must use exec.CommandContext.
type CommandFunc func(ctx context.Context, dir, name string, args []string) *exec.Cmd

const (
	// killDelay bounds how long a cancelled process may ignore its interrupt.
	killDelay = 3 * time.Second
	// eventBuffer keeps an output burst from blocking the process.
	eventBuffer = 256
)

// Manager runs Go commands in one project root and reports them on Events.
type Manager struct {
	root    string
	command CommandFunc
	events  chan Event
	ctx     context.Context
	cancel  context.CancelFunc
	wait    sync.WaitGroup

	detected sync.Once
	tool     Toolchain
	toolErr  error

	mu     sync.Mutex
	seq    uint64
	active map[Kind]*handle
	closed bool

	modeMu    sync.Mutex
	modeCache map[string]*modeEntry
	scanTree  func(string) moduleScan
}

// moduleScan remembers a nested module or the directories checked for one.
type moduleScan struct {
	module     string
	dirs       map[string]time.Time
	standalone bool
	scannedAt  time.Time
}

// modeEntry shares one subtree scan among commands for the same directory.
type modeEntry struct {
	done chan struct{}
	scan moduleScan
}

// handle is the manager's view of one requested job.
type handle struct {
	id     uint64
	cancel context.CancelFunc
	done   chan struct{}
}

// NewManager returns a manager that runs commands in root.
func NewManager(root string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		root:      root,
		command:   defaultCommand,
		events:    make(chan Event, eventBuffer),
		ctx:       ctx,
		cancel:    cancel,
		active:    make(map[Kind]*handle),
		modeCache: make(map[string]*modeEntry),
		scanTree:  scanModules,
	}
}

func defaultCommand(ctx context.Context, dir, name string, args []string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	return command
}

// RunAttached runs a command with the caller's terminal streams.
func (m *Manager) RunAttached(ctx context.Context, request Request, stdin io.Reader, stdout, stderr io.Writer) error {
	tool, err := m.toolchain()
	if err != nil {
		return err
	}
	// Keep the child in the terminal's foreground process group for keyboard input.
	var command *exec.Cmd
	if request.needsBuiltEntry() {
		binary, cleanup, err := m.buildEntry(ctx, tool.Path, request, stdout, stderr)
		if err != nil {
			return err
		}
		defer cleanup()
		command = defaultCommand(ctx, request.directory(m.root), binary, request.Arguments)
	} else {
		command = m.requestCommand(ctx, tool.Path, request)
	}
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if request.needsBuiltEntry() {
		return entryExecutionError(err, command.Path)
	}
	return err
}

// Events returns the manager's event stream. It is closed by Close.
func (m *Manager) Events() <-chan Event { return m.events }

// Warm resolves the Go toolchain in the background, as a Detected event.
func (m *Manager) Warm() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.wait.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wait.Done()
		tool, err := m.toolchain()
		m.emit(Event{Type: Detected, Tool: tool, Err: err})
	}()
}

// Start replaces any job of the same kind and returns the new identity.
func (m *Manager) Start(request Request) uint64 {
	kind := request.Kind
	if kind >= kindCount {
		return 0
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return 0
	}
	previous := m.active[kind]
	if previous != nil {
		previous.cancel()
	}
	m.seq++
	ctx, cancel := context.WithCancel(m.ctx)
	current := &handle{id: m.seq, cancel: cancel, done: make(chan struct{})}
	m.active[kind] = current
	m.wait.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wait.Done()
		defer cancel()
		defer m.release(kind, current)
		defer close(current.done)
		if previous != nil {
			select {
			case <-previous.done:
			case <-m.ctx.Done():
				return
			}
		}
		m.run(ctx, current.id, request)
	}()
	return current.id
}

// Cancel stops the running job of a kind and reports whether one was running.
func (m *Manager) Cancel(kind Kind) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.active[kind]
	if !ok {
		return false
	}
	current.cancel()
	return true
}

// CancelAll stops every running job and returns how many were running.
func (m *Manager) CancelAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.active {
		current.cancel()
	}
	return len(m.active)
}

// Running reports whether a job of the kind is requested and unfinished.
func (m *Manager) Running(kind Kind) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[kind]
	return ok
}

// Close cancels every job, waits for the processes, and closes Events.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.wait.Wait()
	close(m.events)
}

func (m *Manager) release(kind Kind, current *handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[kind] == current {
		delete(m.active, kind)
	}
}

func (m *Manager) toolchain() (Toolchain, error) {
	m.detected.Do(func() { m.tool, m.toolErr = Detect(m.ctx) })
	return m.tool, m.toolErr
}

// run executes one job, emitting one Finished event unless Close intervenes.
func (m *Manager) run(ctx context.Context, id uint64, request Request) {
	kind := request.Kind
	tool, err := m.toolchain()
	if err != nil {
		m.emit(Event{ID: id, Kind: kind, Type: Finished, State: Failed, Err: err})
		return
	}
	stdout := m.lines(id, kind, Stdout)
	stderr := m.lines(id, kind, Stderr)
	if kind == Test {
		stdout.emit = func(line string) { m.testOutput(id, Stdout, line) }
	}
	m.emit(Event{ID: id, Kind: kind, Type: Started, Command: request.Command()})
	var runErr error
	if request.needsBuiltEntry() {
		var binary string
		var cleanup func()
		binary, cleanup, runErr = m.buildEntry(ctx, tool.Path, request, stdout, stderr)
		if runErr == nil {
			defer cleanup()
			command := defaultCommand(ctx, request.directory(m.root), binary, request.Arguments)
			runErr = entryExecutionError(runJobCommand(ctx, command, stdout, stderr), binary)
		}
	} else {
		command := m.requestCommand(ctx, tool.Path, request)
		runErr = runJobCommand(ctx, command, stdout, stderr)
	}
	stdout.flush()
	stderr.flush()
	if ctx.Err() != nil {
		m.emit(Event{ID: id, Kind: kind, Type: Finished, State: Cancelled})
		return
	}
	if runErr != nil {
		m.emit(Event{ID: id, Kind: kind, Type: Finished, State: Failed, Err: runErr})
		return
	}
	m.emit(Event{ID: id, Kind: kind, Type: Finished, State: Succeeded})
}

func (m *Manager) lines(id uint64, kind Kind, stream Stream) *lineWriter {
	return &lineWriter{emit: func(line string) {
		m.emit(Event{ID: id, Kind: kind, Type: Output, Line: line, Stream: stream})
	}}
}

func (m *Manager) emit(event Event) {
	select {
	case m.events <- event:
	case <-m.ctx.Done():
	}
}

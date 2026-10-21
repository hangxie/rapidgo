package debug

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/i18n"
)

// Adapter is the part of a dap.Session that a debug session drives.
type Adapter interface {
	Events() <-chan dap.Event
	Launch(ctx context.Context, config dap.LaunchConfig) error
	SetBreakpoints(ctx context.Context, path string, requested []dap.SourceBreakpoint) ([]dap.Breakpoint, error)
	ConfigurationDone(ctx context.Context) error
	Continue(ctx context.Context, threadID int) error
	Next(ctx context.Context, threadID int) error
	StepIn(ctx context.Context, threadID int) error
	StepOut(ctx context.Context, threadID int) error
	StackTrace(ctx context.Context, threadID, start, levels int) ([]dap.StackFrame, int, error)
	Close() error
}

// Starter starts a debug adapter whose working directory is dir.
type Starter func(ctx context.Context, dir string) (Adapter, error)

// Plan is the program a session launches and the breakpoints it sets first.
type Plan struct {
	Dir         string
	Config      dap.LaunchConfig
	Breakpoints []Request
}

// EventKind identifies what a session Event reports.
type EventKind uint8

const (
	Running   EventKind = iota // breakpoints are set and the program starts
	Output                     // one line of output in Category and Text
	Stopped                    // the program paused for Reason on ThreadID
	Continued                  // the program resumed
	Exited                     // the program exited with ExitCode
	Resolved                   // the adapter answered Request with Results or Err
	Problem                    // Command on ThreadID failed with Err; the session goes on
	Located                    // Frames holds the top of ThreadID's stack
)

// Command names the request a Problem event reports.
type Command uint8

const (
	Continue Command = iota + 1 // resume a paused program
	Next                        // step over the current line
	StepIn                      // step into the call on the current line
	StepOut                     // run until the current function returns
	RunTo                       // continue to a temporary breakpoint
	Locate                      // load where the program paused
)

// Event is one change in a debug session; fields that do not apply to Kind are zero.
type Event struct {
	Kind     EventKind
	Category string // stdout, stderr, or console
	Text     string // an output line, or the adapter's description of a stop
	Reason   string
	ThreadID int
	ExitCode int
	Command  Command
	Request  Request
	Results  []dap.Breakpoint
	Frames   []dap.StackFrame
	Err      error
}

// Session runs one program under a debug adapter in the background.
type Session struct {
	ctx      context.Context
	cancel   context.CancelFunc
	events   chan Event
	mu       sync.Mutex
	adapter  Adapter
	closing  bool // the adapter is being closed, so commands fail for no reason worth reporting
	err      error
	commands sync.WaitGroup
	last     chan struct{} // closes when the most recent command has finished
	// Breakpoint changes wait in pending, newest per file, until configuration has set the launch ones.
	pending    []Request
	configured bool
}

// Launch starts the adapter and program in the background; ctx bounds the whole session.
func Launch(ctx context.Context, start Starter, plan Plan) *Session {
	ctx, cancel := context.WithCancel(ctx)
	s := &Session{ctx: ctx, cancel: cancel, events: make(chan Event, 256)}
	go s.run(start, plan)
	return s
}

// Events delivers session events in order and closes when the session has ended.
func (s *Session) Events() <-chan Event { return s.events }

// Err reports why the session failed once Events closes; Stop is not a failure.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Stop ends the session, terminating the program and the adapter.
func (s *Session) Stop() { s.cancel() }

// run cleans up before finish closes Events, so a closed channel means nothing is left behind.
func (s *Session) run(start Starter, plan Plan) {
	// Delve otherwise writes its binary into the working directory, which is the user's project.
	binaries, err := os.MkdirTemp("", "rapidgo-debug-")
	if err != nil {
		s.finish(i18n.Errorf("msg_create_debug_binary_directory_w", err))
		return
	}
	plan.Config.Output = filepath.Join(binaries, debugBinary())
	err = s.debug(start, plan)
	_ = os.RemoveAll(binaries)
	s.finish(err)
}

// debug runs the adapter and program until the program terminates or the session stops.
func (s *Session) debug(start Starter, plan Plan) error {
	adapter, err := start(s.ctx, plan.Dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.adapter = adapter
	s.mu.Unlock()
	initialized := make(chan struct{})
	pumped := make(chan struct{})
	var terminated bool
	go func() {
		defer close(pumped)
		terminated = s.pump(adapter, initialized)
	}()
	err = s.configure(adapter, plan, initialized, pumped)
	if err == nil {
		select {
		case <-s.ctx.Done():
		case <-pumped:
			if !terminated {
				err = i18n.Error("msg_debug_adapter_connection_closed")
			}
		}
	}
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	_ = adapter.Close()
	<-pumped
	return err
}

// configure follows the DAP launch sequence: launch, initialized, breakpoints, configurationDone.
func (s *Session) configure(adapter Adapter, plan Plan, initialized, pumped <-chan struct{}) error {
	if err := adapter.Launch(s.ctx, plan.Config); err != nil {
		return i18n.Errorf("msg_launch_program_in_delve_w", err)
	}
	select {
	case <-initialized:
	case <-pumped:
		return i18n.Error("msg_debug_adapter_connection_closed")
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	requests := slices.Clone(plan.Breakpoints)
	for {
		s.mu.Lock()
		for _, request := range s.pending {
			requests = queue(requests, request)
		}
		s.pending = nil
		done := len(requests) == 0
		s.configured = done
		s.mu.Unlock()
		if done {
			break
		}
		for _, request := range requests {
			results, err := adapter.SetBreakpoints(s.ctx, request.Path, request.Breakpoints)
			if !s.send(Event{Kind: Resolved, Request: request, Results: results, Err: err}) {
				return s.ctx.Err()
			}
		}
		requests = nil
	}
	// Announced first so it precedes any output the program writes once configuration is done.
	if !s.send(Event{Kind: Running}) {
		return s.ctx.Err()
	}
	if err := adapter.ConfigurationDone(s.ctx); err != nil {
		return i18n.Errorf("msg_start_program_in_delve_w", err)
	}
	return nil
}

// pump translates adapter events and reports whether the program terminated before the connection ended.
func (s *Session) pump(adapter Adapter, initialized chan<- struct{}) bool {
	var lines outputLines
	flush := func() bool {
		for _, line := range lines.flush() {
			if !s.send(line) {
				return false
			}
		}
		return true
	}
	defer flush()
	var once sync.Once
	for event := range adapter.Events() {
		delivered := true
		switch event.Name {
		case "initialized":
			once.Do(func() { close(initialized) })
		case "output":
			// Telemetry is meant for the adapter's authors, not the user.
			if event.Category == "telemetry" {
				break
			}
			for _, line := range lines.add(event.Category, event.Output) {
				delivered = delivered && s.send(line)
			}
		case "stopped":
			delivered = s.send(Event{Kind: Stopped, Reason: event.Reason, ThreadID: event.ThreadID, Text: cmp.Or(event.Description, event.Text)})
		case "continued":
			delivered = s.send(Event{Kind: Continued, ThreadID: event.ThreadID})
		case "exited":
			// Output the program left without a newline belongs before its exit.
			delivered = flush() && s.send(Event{Kind: Exited, ExitCode: event.ExitCode})
		case "terminated":
			// debug closes the adapter as soon as this returns, which releases its reader.
			return true
		}
		if !delivered {
			return false
		}
	}
	return false
}

// send waits for the consumer unless the session was stopped.
func (s *Session) send(event Event) bool {
	select {
	case s.events <- event:
		return true
	case <-s.ctx.Done():
		return false
	}
}

// debugBinary names Delve's build output; Windows needs the extension to run it.
func debugBinary() string {
	if runtime.GOOS == "windows" {
		return "debug.exe"
	}
	return "debug"
}

func (s *Session) finish(err error) {
	s.mu.Lock()
	if s.ctx.Err() == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.commands.Wait()
	close(s.events)
	s.cancel()
}

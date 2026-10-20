package dap

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/procgroup"
)

const (
	listenPrefix     = "DAP server listening at:"
	startTimeout     = 15 * time.Second
	disconnectWait   = time.Second
	killDelay        = 2 * time.Second
	stderrTailLines  = 5
	maxAdapterOutput = 1 << 20
)

// adapterProcess owns a dlv dap process and the goroutines reading its output.
type adapterProcess struct {
	command *exec.Cmd
	stop    context.CancelFunc
	exited  chan struct{}
	tail    sync.Mutex
	stderr  []string
	reaping sync.Mutex
	reaped  bool
}

// Start launches the user's dlv in DAP mode from dir and initializes a session; ctx bounds its lifetime.
func Start(ctx context.Context, dir string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath("dlv")
	if err != nil {
		return nil, i18n.Errorf("msg_find_dlv_on_path_w", err)
	}
	processCtx, stop := context.WithCancel(ctx)
	command := exec.CommandContext(processCtx, path, "dap", "--listen=127.0.0.1:0")
	command.Dir = dir
	procgroup.Configure(command)
	command.Cancel = func() error { return procgroup.Interrupt(command) }
	command.WaitDelay = killDelay
	stdout, stderr, err := outputPipes(command)
	if err != nil {
		stop()
		return nil, i18n.Errorf("msg_open_dlv_output_w", err)
	}
	err = command.Start()
	// The child holds its own copies of the write ends.
	_ = command.Stdout.(*os.File).Close()
	_ = command.Stderr.(*os.File).Close()
	if err != nil {
		_, _ = stdout.Close(), stderr.Close()
		stop()
		return nil, i18n.Errorf("msg_start_dlv_w", err)
	}
	process := &adapterProcess{command: command, stop: stop, exited: make(chan struct{})}
	s := newSession()
	s.process = process
	addresses := process.read(s, stdout, stderr)
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	var address string
	select {
	case found, ok := <-addresses:
		if !ok {
			// Stdout can end while dlv still runs, so stop it rather than waiting for it to exit.
			_ = s.Close()
			if err := ctx.Err(); err != nil {
				return nil, i18n.Errorf("msg_wait_for_dlv_w", err)
			}
			return nil, i18n.Errorf("msg_dlv_exited_before_listening_s", process.stderrTail())
		}
		address = found
	case <-startCtx.Done():
		_ = s.Close()
		return nil, i18n.Errorf("msg_wait_for_dlv_w", startCtx.Err())
	}
	conn, err := (&net.Dialer{}).DialContext(startCtx, "tcp", address)
	if err != nil {
		_ = s.Close()
		return nil, i18n.Errorf("msg_connect_to_dlv_w", err)
	}
	if err := s.attach(startCtx, conn); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// outputPipes connects the command's stdout and stderr to pipes whose read ends the caller owns.
func outputPipes(command *exec.Cmd) (stdout, stderr *os.File, err error) {
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		_, _ = stdout.Close(), stdoutWriter.Close()
		return nil, nil, err
	}
	command.Stdout, command.Stderr = stdoutWriter, stderrWriter
	return stdout, stderr, nil
}

// read forwards adapter output as events, reaps the process, and reports the listen address.
func (p *adapterProcess) read(s *Session, stdout, stderr *os.File) <-chan string {
	addresses := make(chan string, 1)
	var pipes sync.WaitGroup
	pipes.Add(2)
	s.producers.Add(2)
	go func() {
		defer s.producers.Done()
		defer pipes.Done()
		defer drain(stdout)
		// Closing before draining lets Start fail fast when stdout stops being readable.
		defer close(addresses)
		listening := false
		readLines(stdout, func(line string) {
			if address, ok := strings.CutPrefix(line, listenPrefix); ok && !listening {
				listening = true
				addresses <- strings.TrimSpace(address)
				return
			}
			s.deliver(Event{Name: "output", Category: "console", Output: line + "\n"})
		})
	}()
	go func() {
		defer s.producers.Done()
		defer pipes.Done()
		defer drain(stderr)
		readLines(stderr, func(line string) {
			p.remember(line)
			s.deliver(Event{Name: "output", Category: "stderr", Output: line + "\n"})
		})
	}()
	drained := make(chan struct{})
	go func() {
		pipes.Wait()
		close(drained)
	}()
	go func() {
		_ = p.command.Wait()
		p.reaping.Lock()
		p.reaped = true
		p.reaping.Unlock()
		// A descendant in another process group may keep the pipes open long after dlv exits.
		timer := time.NewTimer(killDelay)
		select {
		case <-drained:
		case <-timer.C:
			_, _ = stdout.Close(), stderr.Close()
			<-drained
		}
		timer.Stop()
		_, _ = stdout.Close(), stderr.Close()
		p.stop()
		close(p.exited)
	}()
	return addresses
}

// terminate asks the adapter to end the debuggee, killing it directly if the adapter does not answer.
func (p *adapterProcess) terminate(s *Session, attached bool) {
	// Delve gives the debuggee its own process group, so killing dlv's group would miss it.
	if attached && !disconnect(s) {
		procgroup.KillProcess(s.debuggeePID())
	}
	p.reaping.Lock()
	// Once dlv is reaped its process group id may belong to an unrelated group.
	if !p.reaped {
		procgroup.Kill(p.command)
	}
	p.reaping.Unlock()
	p.stop()
}

// disconnect asks the adapter to end the debuggee and reports whether it agreed within disconnectWait.
func disconnect(s *Session) bool {
	// A write blocked on a full socket holds the write lock and ignores contexts; closing the connection later frees it.
	result := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), disconnectWait)
		defer cancel()
		result <- s.Disconnect(ctx, true)
	}()
	timer := time.NewTimer(disconnectWait)
	defer timer.Stop()
	select {
	case err := <-result:
		return err == nil
	case <-timer.C:
		return false
	}
}

func (p *adapterProcess) wait() { <-p.exited }

func (p *adapterProcess) remember(line string) {
	p.tail.Lock()
	defer p.tail.Unlock()
	p.stderr = append(p.stderr, line)
	if len(p.stderr) > stderrTailLines {
		p.stderr = p.stderr[len(p.stderr)-stderrTailLines:]
	}
}

func (p *adapterProcess) stderrTail() string {
	p.tail.Lock()
	defer p.tail.Unlock()
	return strings.Join(p.stderr, "\n")
}

// readLines calls emit for each line until the stream ends or a line exceeds the limit.
func readLines(reader io.Reader, emit func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 4096), maxAdapterOutput)
	for scanner.Scan() {
		emit(strings.TrimRight(scanner.Text(), "\r"))
	}
}

// drain discards what readLines left so the writer never blocks on a full pipe.
func drain(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) }

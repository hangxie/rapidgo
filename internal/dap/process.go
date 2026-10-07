package dap

import (
	"bufio"
	"context"
	"io"
	"net"
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
	stdout, err := command.StdoutPipe()
	if err != nil {
		stop()
		return nil, i18n.Errorf("msg_open_dlv_output_w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		stop()
		return nil, i18n.Errorf("msg_open_dlv_output_w", err)
	}
	if err := command.Start(); err != nil {
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
			process.wait()
			_ = s.Close()
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

// read forwards adapter output as events, reaps the process after both pipes end, and reports the listen address.
func (p *adapterProcess) read(s *Session, stdout, stderr io.Reader) <-chan string {
	addresses := make(chan string, 1)
	var pipes sync.WaitGroup
	pipes.Add(2)
	s.producers.Add(2)
	go func() {
		defer s.producers.Done()
		defer pipes.Done()
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
		readLines(stderr, func(line string) {
			p.remember(line)
			s.deliver(Event{Name: "output", Category: "stderr", Output: line + "\n"})
		})
	}()
	go func() {
		pipes.Wait()
		_ = p.command.Wait()
		procgroup.Kill(p.command)
		p.stop()
		close(p.exited)
	}()
	return addresses
}

// terminate asks the adapter to end the debuggee, then kills whatever remains of the process group.
func (p *adapterProcess) terminate(s *Session, attached bool) {
	if attached {
		ctx, cancel := context.WithTimeout(context.Background(), disconnectWait)
		_ = s.Disconnect(ctx, true)
		cancel()
	}
	procgroup.Kill(p.command)
	p.stop()
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

// readLines calls emit for each line and discards the rest of a stream whose line exceeds the limit.
func readLines(reader io.Reader, emit func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 4096), maxAdapterOutput)
	for scanner.Scan() {
		emit(strings.TrimRight(scanner.Text(), "\r"))
	}
	_, _ = io.Copy(io.Discard, reader)
}

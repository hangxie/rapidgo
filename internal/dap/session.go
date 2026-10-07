// Package dap speaks the Debug Adapter Protocol to Delve without terminal dependencies.
package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// Event is a decoded adapter event; fields that do not apply to Name are zero.
type Event struct {
	Name             string      `json:"-"`
	Reason           string      `json:"reason"`
	Description      string      `json:"description"`
	Text             string      `json:"text"`
	ThreadID         int         `json:"threadId"`
	AllThreads       bool        `json:"allThreadsStopped"`
	HitBreakpointIDs []int       `json:"hitBreakpointIds"`
	Category         string      `json:"category"`
	Output           string      `json:"output"`
	ExitCode         int         `json:"exitCode"`
	ProcessID        int         `json:"systemProcessId"`
	Breakpoint       *Breakpoint `json:"breakpoint"`
}

// Capabilities lists the optional adapter features RapidGo uses.
type Capabilities struct {
	ConfigurationDone   bool `json:"supportsConfigurationDoneRequest"`
	ConditionalBreaks   bool `json:"supportsConditionalBreakpoints"`
	HitConditionalBreak bool `json:"supportsHitConditionalBreakpoints"`
	SetVariable         bool `json:"supportsSetVariable"`
	EvaluateForHovers   bool `json:"supportsEvaluateForHovers"`
}

// Session exchanges DAP messages with one debug adapter.
type Session struct {
	conn         io.ReadWriteCloser
	writes       sync.Mutex
	state        sync.Mutex
	seq          int
	waits        map[int]chan message
	events       chan Event
	closed       chan struct{} // the adapter connection ended
	done         chan struct{} // Close was called
	producers    sync.WaitGroup
	attached     bool
	err          error
	capabilities Capabilities
	once         sync.Once
	process      *adapterProcess
}

// Connect initializes a DAP session over conn; callers must drain Events while requests are pending.
func Connect(ctx context.Context, conn io.ReadWriteCloser) (*Session, error) {
	s := newSession()
	if err := s.attach(ctx, conn); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func newSession() *Session {
	s := &Session{waits: make(map[int]chan message), events: make(chan Event, 256), closed: make(chan struct{}), done: make(chan struct{})}
	// The connection holds one producer slot until its reader ends, so adapter output can join later.
	s.producers.Add(1)
	go func() {
		s.producers.Wait()
		close(s.events)
	}()
	return s
}

// attach starts reading conn and performs the initialize handshake.
func (s *Session) attach(ctx context.Context, conn io.ReadWriteCloser) error {
	s.state.Lock()
	s.conn, s.attached = conn, true
	s.state.Unlock()
	go s.readLoop()
	arguments := map[string]any{
		"clientID": "rapidgo", "clientName": "RapidGo", "adapterID": "go", "pathFormat": "path",
		"linesStartAt1": true, "columnsStartAt1": true, "supportsVariableType": true,
	}
	if err := s.request(ctx, "initialize", arguments, &s.capabilities); err != nil {
		return i18n.Errorf("msg_initialize_debug_adapter_w", err)
	}
	return nil
}

// Events delivers adapter events in order and closes after the adapter and its output end.
func (s *Session) Events() <-chan Event { return s.events }

// Capabilities reports the features the adapter announced during initialize.
func (s *Session) Capabilities() Capabilities { return s.capabilities }

// Close terminates the adapter, its debuggee, and the connection, unblocking pending requests.
func (s *Session) Close() error {
	s.once.Do(func() {
		// Closing done first lets the reader drop events, so the disconnect reply cannot stall behind them.
		close(s.done)
		s.state.Lock()
		conn, attached := s.conn, s.attached
		s.state.Unlock()
		if s.process != nil {
			s.process.terminate(s, attached)
		}
		if attached {
			_ = conn.Close()
		} else {
			close(s.closed)
			s.producers.Done()
		}
		if s.process != nil {
			s.process.wait()
		}
	})
	return nil
}

func (s *Session) request(ctx context.Context, command string, arguments, result any) error {
	var raw json.RawMessage
	if arguments != nil {
		data, err := json.Marshal(arguments)
		if err != nil {
			return i18n.Errorf("msg_encode_dap_message_w", err)
		}
		raw = data
	}
	wait := make(chan message, 1)
	s.state.Lock()
	s.seq++
	seq := s.seq
	s.waits[seq] = wait
	s.state.Unlock()
	defer func() {
		s.state.Lock()
		delete(s.waits, seq)
		s.state.Unlock()
	}()
	if err := s.send(message{Seq: seq, Type: "request", Command: command, Arguments: raw}); err != nil {
		return err
	}
	select {
	case response := <-wait:
		if !response.Success {
			return i18n.Errorf("msg_dap_s_s", command, response.failure())
		}
		if result == nil || len(response.Body) == 0 {
			return nil
		}
		if err := json.Unmarshal(response.Body, result); err != nil {
			return i18n.Errorf("msg_decode_dap_s_w", command, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return s.connectionError()
	}
}

func (s *Session) send(item message) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	select {
	case <-s.closed:
		return s.connectionError()
	default:
	}
	return writeFrame(s.conn, item)
}

func (s *Session) readLoop() {
	defer s.producers.Done()
	defer close(s.closed)
	reader := bufio.NewReader(s.conn)
	for {
		item, err := readFrame(reader)
		if err != nil {
			s.state.Lock()
			s.err = err
			s.state.Unlock()
			return
		}
		switch item.Type {
		case "response":
			s.state.Lock()
			wait := s.waits[item.RequestSeq]
			s.state.Unlock()
			if wait != nil {
				wait <- item
			}
		case "event":
			event := Event{Name: item.Event}
			if len(item.Body) != 0 && json.Unmarshal(item.Body, &event) != nil {
				event = Event{Name: item.Event}
			}
			s.deliver(event)
		case "request":
			s.refuse(item)
		}
	}
}

// refuse answers reverse requests such as runInTerminal, which RapidGo never enables.
func (s *Session) refuse(item message) {
	s.state.Lock()
	s.seq++
	seq := s.seq
	s.state.Unlock()
	_ = s.send(message{Seq: seq, Type: "response", RequestSeq: item.Seq, Command: item.Command, Message: i18n.Text("msg_method_not_supported")})
}

// deliver blocks until the consumer takes event or the session closes.
func (s *Session) deliver(event Event) {
	select {
	case s.events <- event:
	case <-s.done:
	}
}

func (s *Session) connectionError() error {
	s.state.Lock()
	defer s.state.Unlock()
	if s.err == nil || errors.Is(s.err, io.EOF) {
		return i18n.Error("msg_debug_adapter_connection_closed")
	}
	return i18n.Errorf("msg_debug_adapter_connection_closed_w", s.err)
}

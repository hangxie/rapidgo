package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdapter scripts the adapter side of a DAP connection.
type fakeAdapter struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	seq    int
}

func (f *fakeAdapter) expect(command string) message {
	f.t.Helper()
	require.NoError(f.t, f.conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	item, err := readFrame(f.reader)
	require.NoError(f.t, err)
	require.Equal(f.t, "request", item.Type)
	require.Equal(f.t, command, item.Command)
	return item
}

func (f *fakeAdapter) send(item message) {
	f.t.Helper()
	f.seq++
	item.Seq = f.seq
	require.NoError(f.t, writeFrame(f.conn, item))
}

func (f *fakeAdapter) respond(request message, body any) {
	f.t.Helper()
	f.send(message{Type: "response", RequestSeq: request.Seq, Command: request.Command, Success: true, Body: mustJSON(f.t, body)})
}

func (f *fakeAdapter) fail(request message, text string) {
	f.t.Helper()
	f.send(message{Type: "response", RequestSeq: request.Seq, Command: request.Command, Message: text})
}

func (f *fakeAdapter) event(name string, body any) {
	f.t.Helper()
	f.send(message{Type: "event", Event: name, Body: mustJSON(f.t, body)})
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

// connectFake returns an initialized session and its scripted adapter.
func connectFake(t *testing.T) (*Session, *fakeAdapter) {
	t.Helper()
	client, server := net.Pipe()
	adapter := &fakeAdapter{t: t, conn: server, reader: bufio.NewReader(server)}
	initialized := make(chan message, 1)
	go func() {
		item, err := readFrame(adapter.reader)
		if err != nil {
			close(initialized)
			return
		}
		initialized <- item
		adapter.seq++
		_ = writeFrame(server, message{Seq: adapter.seq, Type: "response", RequestSeq: item.Seq, Command: item.Command, Success: true, Body: json.RawMessage(`{"supportsConfigurationDoneRequest":true,"supportsSetVariable":true}`)})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Connect(ctx, client)
	require.NoError(t, err)
	item := <-initialized
	require.Equal(t, "initialize", item.Command)
	var arguments map[string]any
	require.NoError(t, json.Unmarshal(item.Arguments, &arguments))
	assert.Equal(t, "rapidgo", arguments["clientID"])
	assert.Equal(t, "path", arguments["pathFormat"])
	assert.Equal(t, true, arguments["linesStartAt1"])
	t.Cleanup(func() {
		require.NoError(t, session.Close())
		_ = server.Close()
	})
	return session, adapter
}

func TestConnectRecordsCapabilities(t *testing.T) {
	session, _ := connectFake(t)
	assert.Equal(t, Capabilities{ConfigurationDone: true, SetVariable: true}, session.Capabilities())
}

func TestConnectFailsWhenInitializeIsRejected(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	adapter := &fakeAdapter{t: t, conn: server, reader: bufio.NewReader(server)}
	go func() {
		item, err := readFrame(adapter.reader)
		if err == nil {
			_ = writeFrame(server, message{Seq: 1, Type: "response", RequestSeq: item.Seq, Command: item.Command, Message: "nope"})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Connect(ctx, client)
	require.ErrorContains(t, err, "initialize debug adapter: debug adapter initialize: nope")
}

func TestRequestFailureUsesErrorFormat(t *testing.T) {
	session, adapter := connectFake(t)
	errs := make(chan error, 1)
	go func() { errs <- session.ConfigurationDone(context.Background()) }()
	request := adapter.expect("configurationDone")
	adapter.send(message{Type: "response", RequestSeq: request.Seq, Command: request.Command, Message: "Failed", Body: json.RawMessage(`{"error":{"id":3000,"format":"Failed to launch: too old"}}`)})
	require.EqualError(t, <-errs, "debug adapter configurationDone: Failed to launch: too old")
}

func TestEventsAreDecodedInOrder(t *testing.T) {
	session, adapter := connectFake(t)
	go func() {
		adapter.event("output", map[string]any{"category": "stdout", "output": "hello\n"})
		adapter.event("stopped", map[string]any{"reason": "breakpoint", "threadId": 1, "allThreadsStopped": true, "hitBreakpointIds": []int{3}})
		adapter.event("breakpoint", map[string]any{"reason": "changed", "breakpoint": map[string]any{"id": 3, "verified": true, "line": 12}})
		adapter.event("exited", map[string]any{"exitCode": 2})
		adapter.send(message{Type: "event", Event: "terminated", Body: json.RawMessage(`"not an object"`)})
	}()
	want := []Event{
		{Name: "output", Category: "stdout", Output: "hello\n"},
		{Name: "stopped", Reason: "breakpoint", ThreadID: 1, AllThreads: true, HitBreakpointIDs: []int{3}},
		{Name: "breakpoint", Reason: "changed", Breakpoint: &Breakpoint{ID: 3, Verified: true, Line: 12}},
		{Name: "exited", ExitCode: 2},
		{Name: "terminated"},
	}
	for _, expected := range want {
		select {
		case event := <-session.Events():
			assert.Equal(t, expected, event)
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s event", expected.Name)
		}
	}
}

func TestReverseRequestIsRefused(t *testing.T) {
	_, adapter := connectFake(t)
	adapter.send(message{Type: "request", Command: "runInTerminal", Arguments: json.RawMessage(`{}`)})
	request := adapter.seq
	require.NoError(t, adapter.conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	reply, err := readFrame(adapter.reader)
	require.NoError(t, err)
	assert.Equal(t, "response", reply.Type)
	assert.Equal(t, request, reply.RequestSeq)
	assert.Equal(t, "runInTerminal", reply.Command)
	assert.False(t, reply.Success)
}

func TestRequestCancellation(t *testing.T) {
	session, adapter := connectFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() { errs <- session.Pause(ctx, 1) }()
	request := adapter.expect("pause")
	cancel()
	require.ErrorIs(t, <-errs, context.Canceled)
	// A late reply for the abandoned request is ignored.
	adapter.respond(request, nil)
	go func() { errs <- session.Continue(context.Background(), 1) }()
	adapter.respond(adapter.expect("continue"), map[string]bool{"allThreadsContinued": true})
	require.NoError(t, <-errs)
}

func TestConnectionLossFailsPendingRequestsAndClosesEvents(t *testing.T) {
	session, adapter := connectFake(t)
	errs := make(chan error, 1)
	go func() { errs <- session.Next(context.Background(), 1) }()
	adapter.expect("next")
	require.NoError(t, adapter.conn.Close())
	require.EqualError(t, <-errs, "debug adapter connection closed")
	_, open := <-session.Events()
	assert.False(t, open)
	require.EqualError(t, session.StepIn(context.Background(), 1), "debug adapter connection closed")
}

func TestMalformedFrameReportsCause(t *testing.T) {
	session, adapter := connectFake(t)
	_, err := adapter.conn.Write([]byte("garbage\r\n\r\n"))
	require.NoError(t, err)
	select {
	case _, open := <-session.Events():
		assert.False(t, open)
	case <-time.After(3 * time.Second):
		t.Fatal("events did not close after a malformed frame")
	}
	require.ErrorContains(t, session.StepOut(context.Background(), 1), `debug adapter connection closed: invalid DAP header "garbage\r\n"`)
}

func TestCloseDoesNotWaitForUndrainedEvents(t *testing.T) {
	session, adapter := connectFake(t)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		for range cap(session.events) + 10 {
			if writeFrame(adapter.conn, message{Type: "event", Event: "output"}) != nil {
				return
			}
		}
	}()
	require.Eventually(t, func() bool { return len(session.events) == cap(session.events) }, 3*time.Second, time.Millisecond)
	closed := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close blocked behind undrained events")
	}
	<-sent
	for range session.Events() {
	}
}

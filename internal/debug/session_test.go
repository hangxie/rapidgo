package debug

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/i18n"
)

// fakeAdapter scripts Delve: it announces initialized on launch and records every request.
type fakeAdapter struct {
	events      chan dap.Event
	mu          sync.Mutex
	calls       []string
	launch      func(context.Context) error
	breakpoints error
	continueErr error
	afterConfig []dap.Event
	silent      bool // never announce initialized
	configErr   error
	output      string
	holdResume  bool // Continue waits for Close, as a request in flight when the connection ends
	closed      chan struct{}
	closeOnce   sync.Once
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{events: make(chan dap.Event, 64), closed: make(chan struct{})}
}

func (f *fakeAdapter) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeAdapter) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeAdapter) launchedOutput() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.output
}

func (f *fakeAdapter) Events() <-chan dap.Event { return f.events }

func (f *fakeAdapter) Launch(ctx context.Context, config dap.LaunchConfig) error {
	f.record("launch " + config.Mode + " " + config.Program + " " + strings.Join(config.Args, " "))
	f.mu.Lock()
	f.output = config.Output
	f.mu.Unlock()
	if f.launch != nil {
		if err := f.launch(ctx); err != nil {
			return err
		}
	}
	if !f.silent {
		f.events <- dap.Event{Name: "initialized"}
	}
	return nil
}

func (f *fakeAdapter) SetBreakpoints(_ context.Context, path string, requested []dap.SourceBreakpoint) ([]dap.Breakpoint, error) {
	f.record("setBreakpoints " + path)
	if f.breakpoints != nil {
		return nil, f.breakpoints
	}
	results := make([]dap.Breakpoint, len(requested))
	for index, request := range requested {
		results[index] = dap.Breakpoint{Verified: true, Line: request.Line}
	}
	return results, nil
}

func (f *fakeAdapter) ConfigurationDone(context.Context) error {
	f.record("configurationDone")
	if f.configErr != nil {
		return f.configErr
	}
	for _, event := range f.afterConfig {
		f.events <- event
	}
	return nil
}

func (f *fakeAdapter) Continue(_ context.Context, threadID int) error {
	f.record("continue " + string(rune('0'+threadID)))
	if f.holdResume {
		<-f.closed
	}
	return f.continueErr
}

func (f *fakeAdapter) Close() error {
	f.record("close")
	f.closeOnce.Do(func() {
		close(f.events)
		close(f.closed)
	})
	return nil
}

func starter(adapter *fakeAdapter, dirs *[]string) Starter {
	return func(_ context.Context, dir string) (Adapter, error) {
		*dirs = append(*dirs, dir)
		return adapter, nil
	}
}

// collect reads events until the session closes its channel.
func collect(t *testing.T, session *Session) []Event {
	t.Helper()
	var events []Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-session.Events():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timeout:
			require.FailNow(t, "session did not end")
		}
	}
}

func testPlan() Plan {
	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	return Plan{
		Dir:         "/p",
		Config:      dap.LaunchConfig{Mode: dap.ModeDebug, Program: "/p/cmd"},
		Breakpoints: []Request{set.Request("/p/main.go")},
	}
}

func TestSessionRunsToExit(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.afterConfig = []dap.Event{
		{Name: "output", Category: "stdout", Output: "hello\nwor"},
		{Name: "output", Category: "stderr", Output: "oops"},
		{Name: "output", Category: "stdout", Output: "ld\r\n"},
		{Name: "exited", ExitCode: 3},
		{Name: "terminated"},
	}
	var dirs []string
	plan := testPlan()
	session := Launch(context.Background(), starter(adapter, &dirs), plan)
	events := collect(t, session)

	assert.Equal(t, []string{"/p"}, dirs)
	assert.Equal(t, []string{"launch debug /p/cmd ", "setBreakpoints /p/main.go", "configurationDone", "close"}, adapter.recorded())
	assert.Equal(t, []Event{
		{Kind: Resolved, Request: plan.Breakpoints[0], Results: []dap.Breakpoint{{Verified: true, Line: 4}}},
		{Kind: Running},
		{Kind: Output, Category: "stdout", Text: "hello"},
		{Kind: Output, Category: "stdout", Text: "world"},
		{Kind: Output, Category: "stderr", Text: "oops"},
		{Kind: Exited, ExitCode: 3},
	}, events, "unterminated output precedes the exit")
	require.NoError(t, session.Err())
}

func TestSessionStartFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("dlv not found")
	session := Launch(context.Background(), func(context.Context, string) (Adapter, error) { return nil, failure }, testPlan())
	assert.Empty(t, collect(t, session))
	require.ErrorIs(t, session.Err(), failure)
	session.Continue(1)
	session.Stop()
}

func TestSessionLaunchFailure(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	failure := errors.New("build failed")
	adapter.launch = func(context.Context) error { return failure }
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	assert.Empty(t, collect(t, session))
	require.ErrorIs(t, session.Err(), failure)
	assert.Equal(t, []string{"launch debug /p/cmd ", "close"}, adapter.recorded())
}

func TestSessionStopWhileLaunching(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	launching := make(chan struct{})
	adapter.launch = func(ctx context.Context) error {
		close(launching)
		<-ctx.Done()
		return ctx.Err()
	}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	<-launching
	session.Stop()
	assert.Empty(t, collect(t, session))
	require.NoError(t, session.Err(), "stopping is not a failure")
	assert.Contains(t, adapter.recorded(), "close")
}

func TestSessionStopWhileRunning(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	for event := range session.Events() {
		if event.Kind == Running {
			break
		}
	}
	session.Stop()
	collect(t, session)
	require.NoError(t, session.Err())
	assert.Equal(t, "close", adapter.recorded()[len(adapter.recorded())-1])
}

func TestSessionContinuesAfterStop(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.afterConfig = []dap.Event{{Name: "stopped", Reason: "breakpoint", ThreadID: 7, Description: "Paused on breakpoint"}}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	var stopped Event
	for event := range session.Events() {
		if event.Kind == Stopped {
			stopped = event
			break
		}
	}
	assert.Equal(t, Event{Kind: Stopped, Reason: "breakpoint", ThreadID: 7, Text: "Paused on breakpoint"}, stopped)

	adapter.continueErr = errors.New("not stopped")
	session.Continue(7)
	problem := <-session.Events()
	assert.Equal(t, Problem, problem.Kind)
	assert.Equal(t, Continue, problem.Command)
	assert.Equal(t, 7, problem.ThreadID)
	require.ErrorIs(t, problem.Err, adapter.continueErr)

	adapter.continueErr = nil
	adapter.events <- dap.Event{Name: "continued", ThreadID: 7}
	assert.Equal(t, Event{Kind: Continued, ThreadID: 7}, <-session.Events())
	adapter.events <- dap.Event{Name: "terminated"}
	collect(t, session)
	assert.Contains(t, adapter.recorded(), "continue 7")
	session.Continue(7) // after the session ended
}

func TestSessionBreakpointFailureContinues(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.breakpoints = errors.New("bad path")
	adapter.afterConfig = []dap.Event{{Name: "terminated"}}
	var dirs []string
	plan := testPlan()
	session := Launch(context.Background(), starter(adapter, &dirs), plan)
	events := collect(t, session)
	require.Len(t, events, 2)
	assert.Equal(t, Resolved, events[0].Kind)
	assert.Equal(t, plan.Breakpoints[0], events[0].Request)
	require.ErrorIs(t, events[0].Err, adapter.breakpoints)
	assert.Equal(t, Running, events[1].Kind)
	require.NoError(t, session.Err())
}

func TestSessionEndsWhenAdapterDisconnects(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	for event := range session.Events() {
		if event.Kind == Running {
			break
		}
	}
	adapter.closeOnce.Do(func() { close(adapter.events) })
	collect(t, session)
	require.ErrorIs(t, session.Err(), i18n.Error("msg_debug_adapter_connection_closed"), "a dropped connection is not a normal end")
}

func TestSessionIgnoresAContinueCutShortByTermination(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.holdResume = true
	adapter.continueErr = errors.New("connection closed")
	adapter.afterConfig = []dap.Event{{Name: "stopped", Reason: "breakpoint", ThreadID: 7}}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	for event := range session.Events() {
		if event.Kind == Stopped {
			break
		}
	}
	session.Continue(7)
	require.Eventually(t, func() bool { return slices.Contains(adapter.recorded(), "continue 7") }, 5*time.Second, time.Millisecond)
	adapter.events <- dap.Event{Name: "terminated"}
	for _, event := range collect(t, session) {
		assert.NotEqual(t, Problem, event.Kind, "the program ended on its own")
	}
	require.NoError(t, session.Err())
}

func TestSessionDropsTelemetry(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.afterConfig = []dap.Event{
		{Name: "output", Category: "telemetry", Output: "usage\n"},
		{Name: "output", Category: "stdout", Output: "hello\n"},
		{Name: "terminated"},
	}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), Plan{Dir: "/p"})
	assert.Equal(t, []Event{{Kind: Running}, {Kind: Output, Category: "stdout", Text: "hello"}}, collect(t, session))
}

func TestSessionSplitsLongOutput(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.afterConfig = []dap.Event{
		{Name: "output", Category: "stdout", Output: strings.Repeat("x", maxLineBytes+1) + "\n"},
		{Name: "terminated"},
	}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	var lines []int
	for _, event := range collect(t, session) {
		if event.Kind == Output {
			lines = append(lines, len(event.Text))
		}
	}
	assert.Equal(t, []int{maxLineBytes, 1}, lines)
}

func TestSessionParentCancellation(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	var dirs []string
	session := Launch(ctx, starter(adapter, &dirs), testPlan())
	cancel()
	collect(t, session)
	assert.Contains(t, adapter.recorded(), "close")
}

func TestSessionAdapterGoneBeforeInitialized(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.silent = true
	adapter.launch = func(context.Context) error {
		adapter.closeOnce.Do(func() { close(adapter.events) })
		return nil
	}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	assert.Empty(t, collect(t, session))
	require.Error(t, session.Err())
	assert.NotContains(t, adapter.recorded(), "configurationDone")
}

func TestSessionConfigurationDoneFailure(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.configErr = errors.New("refused")
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	events := collect(t, session)
	assert.Equal(t, Running, events[len(events)-1].Kind)
	require.ErrorIs(t, session.Err(), adapter.configErr)
}

func TestSessionBuildsIntoATemporaryDirectory(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.afterConfig = []dap.Event{{Name: "terminated"}}
	var dirs []string
	collect(t, Launch(context.Background(), starter(adapter, &dirs), testPlan()))
	output := adapter.launchedOutput()
	require.NotEmpty(t, output)
	assert.NotEqual(t, "/p", filepath.Dir(output))
	_, err := os.Stat(filepath.Dir(output))
	assert.True(t, os.IsNotExist(err), "the directory is removed when the session ends")
}

package debug

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
)

// stoppedSession launches a session and waits until the program pauses on thread 7.
func stoppedSession(t *testing.T, adapter *fakeAdapter) *Session {
	t.Helper()
	adapter.afterConfig = []dap.Event{{Name: "stopped", Reason: "breakpoint", ThreadID: 7}}
	var dirs []string
	session := Launch(context.Background(), starter(adapter, &dirs), testPlan())
	for event := range session.Events() {
		if event.Kind == Stopped {
			return session
		}
	}
	require.FailNow(t, "the session never paused")
	return nil
}

func waitFor(t *testing.T, adapter *fakeAdapter, call string) {
	t.Helper()
	require.Eventually(t, func() bool { return slices.Contains(adapter.recorded(), call) }, 5*time.Second, time.Millisecond)
}

func endSession(t *testing.T, adapter *fakeAdapter, session *Session) []Event {
	t.Helper()
	adapter.events <- dap.Event{Name: "terminated"}
	return collect(t, session)
}

func TestSessionSteps(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	session := stoppedSession(t, adapter)
	session.Next(7)
	session.StepIn(7)
	session.StepOut(7)
	waitFor(t, adapter, "stepOut 7")
	assert.Empty(t, endSession(t, adapter, session))
	calls := adapter.recorded()
	assert.Equal(t, []string{"next 7", "stepIn 7", "stepOut 7"}, calls[3:6])
}

func TestSessionReportsAFailedStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command Command
		step    func(*Session, int)
	}{
		{Next, (*Session).Next},
		{StepIn, (*Session).StepIn},
		{StepOut, (*Session).StepOut},
	}
	for _, test := range tests {
		adapter := newFakeAdapter()
		adapter.stepErr = errors.New("not paused")
		session := stoppedSession(t, adapter)
		test.step(session, 7)
		problem := <-session.Events()
		assert.Equal(t, Event{Kind: Problem, Command: test.command, ThreadID: 7, Err: adapter.stepErr}, problem)
		endSession(t, adapter, session)
	}
}

func TestSessionRunsCommandsInOrder(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.gate = make(chan struct{})
	session := stoppedSession(t, adapter)
	session.Continue(7)
	session.Next(7)
	// Next would overtake the held continue if commands raced.
	time.Sleep(20 * time.Millisecond)
	close(adapter.gate)
	waitFor(t, adapter, "next 7")
	endSession(t, adapter, session)
	calls := adapter.recorded()
	assert.Equal(t, []string{"continue 7", "next 7"}, calls[3:5])
}

func TestSessionSetsBreakpointsWhileRunning(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	session := stoppedSession(t, adapter)
	set := NewBreakpoints()
	set.Toggle("/p/b.go", 2)
	request := set.Request("/p/b.go")
	session.SetBreakpoints(request)
	assert.Equal(t, Event{Kind: Resolved, Request: request, Results: []dap.Breakpoint{{Verified: true, Line: 2}}}, <-session.Events())

	adapter.breakpoints = errors.New("bad path")
	session.SetBreakpoints(request)
	assert.Equal(t, Event{Kind: Resolved, Request: request, Err: adapter.breakpoints}, <-session.Events())
	endSession(t, adapter, session)
}

func TestSessionRunsToALine(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	session := stoppedSession(t, adapter)
	request := NewBreakpoints().RunTo("/p/b.go", 9)
	session.RunTo(request, 7)
	assert.Equal(t, Event{Kind: Resolved, Request: request, Results: []dap.Breakpoint{{Verified: true, Line: 9}}}, <-session.Events())
	endSession(t, adapter, session)
	assert.Equal(t, []string{"setBreakpoints /p/b.go", "continue 7"}, adapter.recorded()[3:5])
}

func TestSessionRunToStaysPausedWhenTheBreakpointFails(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	session := stoppedSession(t, adapter)
	adapter.breakpoints = errors.New("bad path")
	session.RunTo(NewBreakpoints().RunTo("/p/b.go", 9), 7)
	assert.Equal(t, Event{Kind: Problem, Command: RunTo, ThreadID: 7, Err: adapter.breakpoints}, <-session.Events())
	endSession(t, adapter, session)
	assert.NotContains(t, adapter.recorded(), "continue 7")
}

func TestSessionLocatesThePause(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.frames = []dap.StackFrame{{ID: 1000, Name: "main.main", Source: dap.Source{Path: "/p/main.go"}, Line: 4, Column: 2}}
	session := stoppedSession(t, adapter)
	session.Locate(7)
	assert.Equal(t, Event{Kind: Located, ThreadID: 7, Frames: adapter.frames}, <-session.Events())

	adapter.stackErr = errors.New("no goroutine")
	session.Locate(7)
	assert.Equal(t, Event{Kind: Problem, Command: Locate, ThreadID: 7, Err: adapter.stackErr}, <-session.Events())
	endSession(t, adapter, session)
	assert.Contains(t, adapter.recorded(), "stackTrace 7 0 50")
}

func TestSessionInspectsAFrame(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.scopes = []dap.Scope{{Name: "Locals", Reference: 1}, {Name: "Globals", Reference: 2, Expensive: true}}
	adapter.variables = map[int][]dap.Variable{1: {{Name: "n", Value: "3"}}, 10: {{Name: "X", Value: "1"}}}
	session := stoppedSession(t, adapter)
	session.Inspect(1000)
	assert.Equal(t, Event{Kind: Inspected, FrameID: 1000, Scopes: []Scope{
		{Scope: adapter.scopes[0], Loaded: true, Variables: adapter.variables[1]},
		{Scope: adapter.scopes[1]},
	}}, <-session.Events())
	session.Expand(10)
	assert.Equal(t, Event{Kind: Expanded, Reference: 10, Variables: adapter.variables[10]}, <-session.Events())
	endSession(t, adapter, session)
	calls := adapter.recorded()
	assert.Equal(t, []string{"scopes 1000", "variables 1 0 0", "variables 10 0 0"}, calls[3:6], "an expensive scope waits until it is opened")
}

func TestSessionReportsFailedInspection(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.scopes = []dap.Scope{{Name: "Locals", Reference: 1}}
	session := stoppedSession(t, adapter)
	adapter.variableErr = errors.New("unreadable")
	session.Inspect(1000)
	assert.Equal(t, Event{Kind: Problem, Command: Inspect, FrameID: 1000, Err: adapter.variableErr}, <-session.Events())
	session.Expand(10)
	assert.Equal(t, Event{Kind: Problem, Command: Expand, Reference: 10, Err: adapter.variableErr}, <-session.Events())
	adapter.stackErr = errors.New("no frame")
	session.Inspect(1000)
	assert.Equal(t, Event{Kind: Problem, Command: Inspect, FrameID: 1000, Err: adapter.stackErr}, <-session.Events())
	endSession(t, adapter, session)
}

func TestSessionLocatesBeforeTheNextStop(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.frames = []dap.StackFrame{{Source: dap.Source{Path: "/p/main.go"}, Line: 4}}
	adapter.afterNext = []dap.Event{{Name: "stopped", Reason: "step", ThreadID: 7}}
	session := stoppedSession(t, adapter)
	session.Locate(7)
	session.Next(7)
	// A location that arrived after the next stop would be stale.
	assert.Equal(t, Located, (<-session.Events()).Kind)
	assert.Equal(t, Stopped, (<-session.Events()).Kind)
	endSession(t, adapter, session)
}

func TestSessionRunToStaysPausedWhenDelveRejectsTheLine(t *testing.T) {
	t.Parallel()

	adapter := newFakeAdapter()
	adapter.refuseLine = 9
	session := stoppedSession(t, adapter)
	set := NewBreakpoints()
	set.Toggle("/p/b.go", 3)
	session.RunTo(set.RunTo("/p/b.go", 9), 7)
	resolved := <-session.Events()
	assert.Equal(t, Resolved, resolved.Kind, "the other breakpoints still get their results")
	problem := <-session.Events()
	assert.Equal(t, Problem, problem.Kind)
	assert.Equal(t, RunTo, problem.Command)
	require.ErrorContains(t, problem.Err, "no code at this line")
	require.ErrorContains(t, problem.Err, "9")
	endSession(t, adapter, session)
	assert.NotContains(t, adapter.recorded(), "continue 7", "the program would run past the line")
}

func TestSessionQueuesBreakpointsUntilConfiguration(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"starting", "launching"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			adapter := newFakeAdapter()
			adapter.afterConfig = []dap.Event{{Name: "terminated"}}
			waiting, release := make(chan struct{}), make(chan struct{})
			start := func(context.Context, string) (Adapter, error) {
				if stage == "starting" {
					close(waiting)
					<-release
				}
				return adapter, nil
			}
			if stage == "launching" {
				adapter.launch = func(context.Context) error {
					close(waiting)
					<-release
					return nil
				}
			}
			plan := testPlan()
			session := Launch(context.Background(), start, plan)
			<-waiting
			set := NewBreakpoints()
			set.Toggle("/p/main.go", 4)
			set.Toggle("/p/main.go", 6)
			set.Toggle("/p/b.go", 2)
			session.SetBreakpoints(set.Request("/p/b.go"))
			session.SetBreakpoints(set.Request("/p/main.go"))
			close(release)
			events := collect(t, session)

			var resolved []Request
			for _, event := range events {
				if event.Kind == Resolved {
					resolved = append(resolved, event.Request)
				}
			}
			assert.Equal(t, []Request{set.Request("/p/main.go"), set.Request("/p/b.go")}, resolved, "the newer snapshot replaces the launch one")
			calls := adapter.recorded()
			assert.Equal(t, []string{"setBreakpoints /p/main.go", "setBreakpoints /p/b.go", "configurationDone"}, calls[1:4])
		})
	}
}

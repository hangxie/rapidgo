package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// scriptedAdapter stands in for dlv: it accepts every request and records what it was asked.
type scriptedAdapter struct {
	events    chan dap.Event
	mu        sync.Mutex
	dirs      []string
	launched  []dap.LaunchConfig
	paths     []string
	continued []int
	steps     []string
	frames    []dap.StackFrame
	stackErr  error
	refuse    error // returned by Continue and steps
	misplace  error // returned by SetBreakpoints
	once      sync.Once
}

func newScriptedAdapter() *scriptedAdapter {
	return &scriptedAdapter{events: make(chan dap.Event, 64)}
}

func (a *scriptedAdapter) start(_ context.Context, dir string) (debug.Adapter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirs = append(a.dirs, dir)
	return a, nil
}

func (a *scriptedAdapter) Events() <-chan dap.Event { return a.events }

func (a *scriptedAdapter) Launch(_ context.Context, config dap.LaunchConfig) error {
	a.mu.Lock()
	a.launched = append(a.launched, config)
	a.mu.Unlock()
	a.events <- dap.Event{Name: "initialized"}
	return nil
}

func (a *scriptedAdapter) SetBreakpoints(_ context.Context, path string, requested []dap.SourceBreakpoint) ([]dap.Breakpoint, error) {
	a.mu.Lock()
	a.paths = append(a.paths, path)
	a.mu.Unlock()
	if a.misplace != nil {
		return nil, a.misplace
	}
	results := make([]dap.Breakpoint, len(requested))
	for index := range requested {
		results[index].Verified = true
	}
	return results, nil
}

func (a *scriptedAdapter) ConfigurationDone(context.Context) error { return nil }

func (a *scriptedAdapter) Continue(_ context.Context, threadID int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.continued = append(a.continued, threadID)
	return a.refuse
}

func (a *scriptedAdapter) Next(_ context.Context, threadID int) error {
	return a.step("next", threadID)
}

func (a *scriptedAdapter) StepIn(_ context.Context, threadID int) error {
	return a.step("stepIn", threadID)
}

func (a *scriptedAdapter) StepOut(_ context.Context, threadID int) error {
	return a.step("stepOut", threadID)
}

func (a *scriptedAdapter) step(name string, threadID int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steps = append(a.steps, fmt.Sprint(name, " ", threadID))
	return a.refuse
}

func (a *scriptedAdapter) StackTrace(context.Context, int, int, int) ([]dap.StackFrame, int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.frames, len(a.frames), a.stackErr
}

func (a *scriptedAdapter) Close() error {
	a.once.Do(func() { close(a.events) })
	return nil
}

// launches omits the temporary binary path the session chooses.
func (a *scriptedAdapter) launches() []dap.LaunchConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	configs := append([]dap.LaunchConfig(nil), a.launched...)
	for index := range configs {
		configs[index].Output = ""
	}
	return configs
}

// pumpDebug applies session events as the run loop does, until done accepts one or the session ends.
func pumpDebug(t *testing.T, state *shellState, done func(debug.Event) bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		events := state.debugEvents()
		if events == nil {
			return
		}
		select {
		case event, ok := <-events:
			if !ok {
				state.finishDebug()
				return
			}
			state.applyDebugEvent(event)
			if done != nil && done(event) {
				return
			}
		case <-timeout:
			require.FailNow(t, "debug session did not progress")
		}
	}
}

func untilKind(kind debug.EventKind) func(debug.Event) bool {
	return func(event debug.Event) bool { return event.Kind == kind }
}

const debugTestSource = "package demo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tt.Log(1)\n}\n"

func debugTestState(t *testing.T, adapter *scriptedAdapter) *shellState {
	t.Helper()
	state, _ := runState(t, mainPackage("cmd/tool"))
	state.startDebugger = adapter.start
	return state
}

func TestDebugTestUnderCaret(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	setTestDocument(t, state, path, debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.breakpointSet().Toggle(path, 6)

	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	dir := filepath.Join(runRoot, "demo")
	// Go builds relative to the root as Go jobs do, since GOPATH mode cannot build absolute paths.
	require.Equal(t, []dap.LaunchConfig{{Mode: dap.ModeTest, Program: "./demo", Cwd: dir, Args: []string{"-test.run", "^TestAdd$"}}}, adapter.launches())
	assert.Equal(t, []string{runRoot}, adapter.dirs)
	assert.Equal(t, []string{path}, adapter.paths)
	breakpoint, _ := state.breakpointSet().At(path, 6)
	assert.True(t, breakpoint.Verified)

	view := state.activeView()
	require.NotNil(t, view)
	assert.Equal(t, jobs.Running, view.state)
	assert.Contains(t, view.command, "TestAdd")
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Contains(t, state.message, "Ctrl+F2")

	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugMainPackage(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	state.runArguments = []string{"-v", "two words"}
	setTestDocument(t, state, filepath.Join(runRoot, "cmd", "tool", "main.go"), "package main\n\nfunc main() {}\n")

	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	require.Equal(t, []dap.LaunchConfig{{
		Mode: dap.ModeDebug, Program: "./cmd/tool", Cwd: runRoot, Args: []string{"-v", "two words"},
	}}, adapter.launches())
	assert.Equal(t, []string{runRoot}, adapter.dirs)
	assert.Contains(t, state.activeView().command, "./cmd/tool")
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugChoosesAmongMainPackages(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, _ := runState(t, mainPackage("cmd/a"), mainPackage("cmd/b"))
	state.startDebugger = adapter.start

	state.requestDebug()
	require.NotNil(t, state.chooser)
	assert.Equal(t, runInDebugger, state.chooser.mode)
	state.handleChooserKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	state.handleChooserKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	pumpDebug(t, state, untilKind(debug.Running))
	require.Len(t, adapter.launches(), 1)
	assert.Equal(t, "./cmd/b", adapter.launches()[0].Program)
	assert.Equal(t, "./cmd/b", state.runTarget)
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugPauseAndContinue(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	state.requestDebug()
	assert.Len(t, adapter.launches(), 1, "a running session is not started twice")
	assert.Contains(t, state.message, "Ctrl+F2")

	adapter.events <- dap.Event{Name: "stopped", Reason: "exception", ThreadID: 3, Description: "panic"}
	pumpDebug(t, state, untilKind(debug.Stopped))
	assert.Contains(t, state.message, "panic")
	assert.Contains(t, state.message, "Shift+F9")

	state.requestDebug()
	assert.Eventually(t, func() bool {
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		return len(adapter.continued) == 1 && adapter.continued[0] == 3
	}, 5*time.Second, time.Millisecond)
	assert.Len(t, adapter.launches(), 1)
	assert.False(t, state.debugger.paused)

	adapter.events <- dap.Event{Name: "output", Category: "stderr", Output: "boom\n"}
	adapter.events <- dap.Event{Name: "exited", ExitCode: 2}
	adapter.events <- dap.Event{Name: "terminated"}
	pumpDebug(t, state, nil)
	view := state.activeView()
	require.NotNil(t, view)
	assert.Equal(t, jobs.Failed, view.state)
	assert.Equal(t, "boom", view.lines[len(view.lines)-1].text)
	assert.Equal(t, jobs.Stderr, view.lines[len(view.lines)-1].stream)
	assert.Contains(t, state.message, "2")
	assert.Nil(t, state.debugEvents())
}

func TestDebugRetriesAFailedContinue(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	adapter.refuse = errors.New("not stopped")
	state := debugTestState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	adapter.events <- dap.Event{Name: "stopped", Reason: "breakpoint", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Stopped))

	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Problem))
	assert.Contains(t, state.message, "not stopped")
	assert.True(t, state.debugger.paused, "the program is still paused")

	adapter.mu.Lock()
	adapter.refuse = nil
	adapter.mu.Unlock()
	state.requestDebug()
	assert.Eventually(t, func() bool {
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		return slices.Equal(adapter.continued, []int{3, 3})
	}, 5*time.Second, time.Millisecond)
	assert.False(t, state.debugger.paused)
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugStop(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	state.stopDebug()
	assert.Contains(t, state.message, "No debug session")

	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	state.breakpointSet().Toggle(path, 6)
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	state.stopDebug()
	pumpDebug(t, state, nil)
	assert.Equal(t, jobs.Cancelled, state.activeView().state)
	breakpoint, _ := state.breakpointSet().At(path, 6)
	assert.False(t, breakpoint.Verified, "verification ends with the session")
}

func TestDebugStartFailureShowsTheError(t *testing.T) {
	t.Parallel()

	state, _ := runState(t, mainPackage("cmd/tool"))
	state.startDebugger = func(context.Context, string) (debug.Adapter, error) {
		return nil, errors.New("find dlv on PATH: not found\nsecond line")
	}
	state.requestDebug()
	pumpDebug(t, state, nil)
	view := state.activeView()
	require.NotNil(t, view)
	assert.Equal(t, jobs.Failed, view.state)
	require.Len(t, view.lines, 2)
	assert.Equal(t, "find dlv on PATH: not found", view.lines[0].text)
	assert.Equal(t, jobs.Stderr, view.lines[0].stream)
	assert.Contains(t, state.message, "not found")
}

func TestDebugUnavailable(t *testing.T) {
	t.Parallel()

	state, _ := runState(t, mainPackage("cmd/tool"))
	state.requestDebug()
	assert.Contains(t, state.message, "not available")
	assert.Nil(t, state.debugEvents())
}

func TestDebugOutputSitsBesideJobOutput(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	state.startJob(jobs.Build)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	debugView := state.activeView()

	state.showAdjacentJob(1)
	assert.Equal(t, state.views[jobs.Build], state.activeView())
	state.showAdjacentJob(1)
	assert.Equal(t, debugView, state.activeView())
	state.showAdjacentJob(-1)
	assert.Equal(t, state.views[jobs.Build], state.activeView())

	state.startJob(jobs.Test)
	assert.Equal(t, state.views[jobs.Test], state.activeView(), "a new job takes the pane")

	state.jobs = &fakeRunner{remaining: 1}
	state.stopJob()
	assert.Equal(t, "Stopping the running Go command and the debug session", state.message)
	pumpDebug(t, state, nil)
	assert.Equal(t, jobs.Cancelled, debugView.state, "Ctrl+K also stops the debug session")
}

func TestDebugKeys(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF9, 0, tcell.ModShift))
	assert.Empty(t, runner.started, "Shift+F9 debugs rather than builds")
	assert.Contains(t, state.message, "not available")

	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF2, 0, tcell.ModCtrl))
	assert.Contains(t, state.message, "No debug session")

	handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModAlt))
	assert.True(t, state.menuOpen)
	assert.Equal(t, menuDebug, state.menuIndex)
	state.menuItem = debugMenuStop
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Contains(t, state.message, "No debug session")
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModAlt))
	state.menuItem = debugMenuStart
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Contains(t, state.message, "not available")
}

func TestCloseDebugStopsTheSession(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	state.closeDebug()
	_, open := <-adapter.events
	assert.False(t, open, "quitting closes the adapter")
	state.closeDebug()
}

func TestDeferredDebugKeepsTheRunningSession(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	runner := &fakeRunner{packages: []jobs.Package{mainPackage("cmd/tool")}}
	state := &shellState{projectRoot: runRoot, jobs: runner, startDebugger: adapter.start}
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	state.requestDebug()
	require.True(t, state.discovering, "outside a test, debugging waits for the package listing")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	running := state.debugger

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: runner.packages})
	assert.Same(t, running, state.debugger, "the listing does not replace the session")
	assert.Len(t, adapter.launches(), 1)
	assert.Contains(t, state.message, "Ctrl+F2")
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebuggedTestOutputJumpsIntoTheTestPackage(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	adapter.events <- dap.Event{Name: "output", Category: "console", Output: "demo_test.go:9: from Delve\n"}
	adapter.events <- dap.Event{Name: "output", Category: "stdout", Output: "--- FAIL: TestAdd (0.00s)\n    demo_test.go:6: wrong result\nFAIL\n"}
	adapter.events <- dap.Event{Name: "exited", ExitCode: 1}
	adapter.events <- dap.Event{Name: "terminated"}
	pumpDebug(t, state, nil)

	view := state.activeView()
	require.NotNil(t, view)
	problems := view.diagnostics()
	require.Len(t, problems, 1, "Delve's own messages are not program diagnostics")
	assert.Equal(t, filepath.Join(runRoot, "demo", "demo_test.go"), problems[0].Path)
	assert.Equal(t, 6, problems[0].Line)
	assert.Equal(t, diagnostic.Error, problems[0].Severity)
	assert.True(t, view.lines[0].console)
	assert.False(t, view.lines[1].console)
}

func TestDrainDebugEventsAppliesABurst(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	drainDebugEvents(state)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	adapter.events <- dap.Event{Name: "output", Category: "stdout", Output: "one\ntwo\nthree\n"}
	adapter.events <- dap.Event{Name: "exited"}
	adapter.events <- dap.Event{Name: "terminated"}
	assert.Eventually(t, func() bool {
		drainDebugEvents(state)
		return !state.debugger.active()
	}, 5*time.Second, time.Millisecond)
	view := state.activeView()
	require.Len(t, view.lines, 3)
	assert.Equal(t, "three", view.lines[2].text)
	assert.Equal(t, jobs.Succeeded, view.state)
}

func TestStopJobNamesTheDebugSession(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		running int
		want    string
	}{
		{name: "debugger alone", want: "Stopping dlv test ./demo -test.run ^TestAdd$"},
		{name: "debugger and jobs", running: 2, want: "Stopping 2 running Go commands and the debug session"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			state := debugTestState(t, newScriptedAdapter())
			setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
			require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
			state.requestDebug()
			pumpDebug(t, state, untilKind(debug.Running))
			state.jobs = &fakeRunner{remaining: test.running}
			state.stopJob()
			assert.Equal(t, test.want, state.message)
			pumpDebug(t, state, nil)
		})
	}
}

func TestDebugKeepsABreakpointFailureInView(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	adapter.misplace = errors.New("no such file")
	state := debugTestState(t, adapter)
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	setTestDocument(t, state, path, debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.breakpointSet().Toggle(path, 6)
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	assert.Contains(t, state.message, "no such file", "starting the program does not hide the failure")
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugProblemAfterExitLeavesTheProgramEnded(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "demo", "demo_test.go"), debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	state.applyDebugEvent(debug.Event{Kind: debug.Exited})
	state.applyDebugEvent(debug.Event{Kind: debug.Problem, Command: debug.Continue, ThreadID: 3, Err: errors.New("closed")})
	assert.False(t, state.debugger.paused)
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestDebugOutputCategories(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		category string
		console  bool
		stream   jobs.Stream
	}{
		{category: "stdout", stream: jobs.Stdout},
		{category: "stderr", stream: jobs.Stderr},
		{category: "console", console: true},
		{category: "important", console: true},
		{category: "", console: true},
	} {
		t.Run(test.category, func(t *testing.T) {
			t.Parallel()

			run := &debugRun{output: &jobView{parser: diagnostic.New(diagnostic.Program)}}
			run.appendOutput(debug.Event{Kind: debug.Output, Category: test.category, Text: "line"})
			require.Len(t, run.output.lines, 1)
			assert.Equal(t, test.console, run.output.lines[0].console)
			if !test.console {
				assert.Equal(t, test.stream, run.output.lines[0].stream)
			}
		})
	}
}

package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/editor"
)

// pausedState debugs TestAdd and pauses it on thread 3 at line 6 of its file.
func pausedState(t *testing.T, adapter *scriptedAdapter) (*shellState, string) {
	t.Helper()
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	adapter.frames = []dap.StackFrame{{Name: "demo.TestAdd", Source: dap.Source{Path: path}, Line: 6, Column: 2}}
	state := debugTestState(t, adapter)
	setTestDocument(t, state, path, debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 4}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	adapter.events <- dap.Event{Name: "stopped", Reason: "breakpoint", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Located))
	t.Cleanup(func() {
		state.closeDebug()
	})
	return state, path
}

func (a *scriptedAdapter) recordedSteps() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.steps...)
}

func TestDebugPauseShowsTheLine(t *testing.T) {
	t.Parallel()

	state, path := pausedState(t, newScriptedAdapter())
	line, ok := state.executionLine(path)
	require.True(t, ok)
	assert.Equal(t, 6, line)
	assert.Equal(t, editor.Position{Line: 5, Column: 1}, state.buffer.Cursor(), "the caret moves to the paused line")
	_, ok = state.executionLine(filepath.Join(runRoot, "other.go"))
	assert.False(t, ok)
}

func TestDebugStepKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  *tcell.EventKey
		want string
	}{
		{"F8 steps over", tcell.NewEventKey(tcell.KeyF8, 0, 0), "next 3"},
		{"F7 steps into", tcell.NewEventKey(tcell.KeyF7, 0, 0), "stepIn 3"},
		{"Shift+F8 steps out", tcell.NewEventKey(tcell.KeyF8, 0, tcell.ModShift), "stepOut 3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			adapter := newScriptedAdapter()
			state, path := pausedState(t, adapter)
			handleKey(screen, state, test.key)
			assert.Eventually(t, func() bool { return len(adapter.recordedSteps()) == 1 }, 5*time.Second, time.Millisecond)
			assert.Equal(t, []string{test.want}, adapter.recordedSteps())
			assert.False(t, state.debugger.paused)
			_, ok := state.executionLine(path)
			assert.False(t, ok, "the line is stale once the program moves")

			handleKey(screen, state, test.key)
			assert.Contains(t, state.message, "paused")
			assert.Len(t, adapter.recordedSteps(), 1, "a running program does not step")
		})
	}
}

func TestDebugStepWithoutSession(t *testing.T) {
	t.Parallel()

	state := &shellState{projectRoot: runRoot}
	state.requestStep(debug.Next)
	assert.Contains(t, state.message, "No debug session")
	state.runToCursor()
	assert.Contains(t, state.message, "No debug session")
}

func TestDebugRetriesARefusedStep(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, _ := pausedState(t, adapter)
	adapter.mu.Lock()
	adapter.refuse = errors.New("not stopped")
	adapter.mu.Unlock()
	state.requestStep(debug.StepIn)
	pumpDebug(t, state, untilKind(debug.Problem))
	assert.True(t, state.debugger.paused, "Delve kept the program paused")
	assert.Contains(t, state.message, "not stopped")
}

func TestDebugRefusedCommandKeepsTheExecutionLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		send func(*shellState)
	}{
		{"continue", (*shellState).requestDebug},
		{"step", func(state *shellState) { state.requestStep(debug.Next) }},
		{"run to cursor", (*shellState).runToCursor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			adapter := newScriptedAdapter()
			state, path := pausedState(t, adapter)
			adapter.mu.Lock()
			adapter.refuse = errors.New("not stopped")
			adapter.mu.Unlock()
			test.send(state)
			_, ok := state.executionLine(path)
			assert.False(t, ok, "the line is hidden while the command is in flight")
			pumpDebug(t, state, untilKind(debug.Problem))
			line, ok := state.executionLine(path)
			assert.True(t, ok, "the program never moved")
			assert.Equal(t, 6, line)
		})
	}
}

func TestDebugRefusedStepBeforeTheLineArrives(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	adapter.frames = []dap.StackFrame{{Source: dap.Source{Path: path}, Line: 6}}
	adapter.refuse = errors.New("not stopped")
	state := debugTestState(t, adapter)
	setTestDocument(t, state, path, debugTestSource)
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	adapter.events <- dap.Event{Name: "stopped", Reason: "breakpoint", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Stopped))
	state.requestStep(debug.Next)
	pumpDebug(t, state, untilKind(debug.Problem))
	t.Cleanup(state.closeDebug)
	line, ok := state.executionLine(path)
	assert.True(t, ok, "the location arrived while the step was in flight")
	assert.Equal(t, 6, line)
}

func TestDebugStopForgetsTheOldLine(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, path := pausedState(t, adapter)
	adapter.mu.Lock()
	adapter.stackErr = errors.New("no frames")
	adapter.mu.Unlock()
	state.requestStep(debug.Next)
	adapter.events <- dap.Event{Name: "stopped", Reason: "step", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Problem))
	_, ok := state.executionLine(path)
	assert.False(t, ok, "a new stop without a location shows no stale line")
}

func TestRunToCursorNeedsAGoFile(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, _ := pausedState(t, adapter)
	setTestDocument(t, state, filepath.Join(runRoot, "README.md"), "# demo\n")
	state.runToCursor()
	assert.Contains(t, state.message, "Open a Go file")
	assert.True(t, state.debugger.paused, "nothing was sent")
	adapter.mu.Lock()
	assert.Len(t, adapter.requested, 0)
	adapter.mu.Unlock()
}

func TestRunToCursor(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	adapter := newScriptedAdapter()
	state, path := pausedState(t, adapter)
	state.breakpointSet().Toggle(path, 5)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 6}, false))

	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF4, 0, 0))
	pumpDebug(t, state, untilKind(debug.Resolved))
	assert.Eventually(t, func() bool {
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		return len(adapter.continued) == 1
	}, 5*time.Second, time.Millisecond)
	assert.Equal(t, []int{5}, breakpointLines(state, path), "the stop is temporary")

	adapter.events <- dap.Event{Name: "stopped", Reason: "breakpoint", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Resolved))
	adapter.mu.Lock()
	assert.Equal(t, [][]int{{5, 7}, {5}}, adapter.requested, "the file's breakpoints are restored after the stop")
	adapter.mu.Unlock()
}

func TestRunToCursorFailureRestoresBreakpoints(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, path := pausedState(t, adapter)
	state.breakpointSet().Toggle(path, 5)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 6}, false))
	adapter.mu.Lock()
	adapter.refuse = errors.New("not stopped")
	adapter.mu.Unlock()
	state.runToCursor()
	pumpDebug(t, state, untilKind(debug.Problem))
	assert.True(t, state.debugger.paused)
	pumpDebug(t, state, untilKind(debug.Resolved))
	adapter.mu.Lock()
	assert.Equal(t, [][]int{{5, 7}, {5}}, adapter.requested)
	adapter.mu.Unlock()
}

func TestDebugPauseScrollsToTheLine(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 16)
	adapter := newScriptedAdapter()
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	adapter.frames = []dap.StackFrame{{Source: dap.Source{Path: path}, Line: 50}}
	state := debugTestState(t, adapter)
	setTestDocument(t, state, path, debugTestSource+strings.Repeat("\n", 60))
	state.setFocus(focusEditor)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	t.Cleanup(state.closeDebug)

	adapter.events <- dap.Event{Name: "stopped", Reason: "breakpoint", ThreadID: 3}
	timeout := time.After(5 * time.Second)
	for state.debugger.stopLine == 0 {
		select {
		case event, ok := <-state.debugEvents():
			state.applyDebugUpdate(screen, event, ok)
		case <-timeout:
			require.FailNow(t, "the pause was never located")
		}
	}
	rows := state.layout(screen).editor.height - 2
	assert.GreaterOrEqual(t, 49, state.fileScroll)
	assert.Less(t, 49, state.fileScroll+rows, "the paused line is on screen")
}

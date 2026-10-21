package ui

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/editor"
)

// inspectState pauses a test in TestAdd, called from a second frame, with Locals holding n and a struct p.
func inspectState(t *testing.T, adapter *scriptedAdapter) (*shellState, tcell.Screen) {
	t.Helper()
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	adapter.frames = []dap.StackFrame{
		{ID: 1000, Name: "demo.TestAdd", Source: dap.Source{Path: path}, Line: 6, Column: 2},
		{ID: 1001, Name: "testing.tRunner", Source: dap.Source{Path: path}, Line: 3, Column: 1},
	}
	adapter.scopes = []dap.Scope{{Name: "Locals", Reference: 1}}
	adapter.variables = map[int][]dap.Variable{
		1:  {{Name: "n", Value: "3", Type: "int"}, {Name: "p", Value: "demo.Point {X: 1}", Type: "demo.Point", Reference: 10}},
		10: {{Name: "X", Value: "1", Type: "int"}},
	}
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 30)
	state, _ := pausedState(t, adapter)
	return state, screen
}

func (a *scriptedAdapter) recordedInspections() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.inspected...)
}

func variableNames(state *shellState) []string {
	if state.debugger.tree == nil {
		return nil
	}
	var names []string
	for _, row := range state.debugger.tree.Rows() {
		names = append(names, row.Name)
	}
	return names
}

var (
	callStackKey = tcell.NewEventKey(tcell.KeyF3, 0, tcell.ModCtrl)
	variablesKey = tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModCtrl|tcell.ModAlt)
)

func TestDebugCallStack(t *testing.T) {
	t.Parallel()

	state, screen := inspectState(t, newScriptedAdapter())
	handleKey(screen, state, callStackKey)
	assert.Equal(t, bottomStack, state.bottomMode)
	assert.Equal(t, focusOutput, state.focus)

	handleKey(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0))
	assert.Equal(t, 0, state.debugger.frame, "moving the selection does not switch frames")
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, 1, state.debugger.frame)
	assert.Equal(t, editor.Position{Line: 2}, state.buffer.Cursor(), "Enter shows the frame's line")
	assert.Equal(t, focusEditor, state.focus, "Enter jumps to the source, as in the Errors view")
	assert.Equal(t, bottomStack, state.bottomMode)

	state.setFocus(focusOutput)
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestDebugVariables(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, screen := inspectState(t, adapter)
	handleKey(screen, state, variablesKey)
	assert.Equal(t, bottomVariables, state.bottomMode)
	assert.Equal(t, focusOutput, state.focus)
	pumpDebug(t, state, untilKind(debug.Inspected))
	assert.Equal(t, []string{"Locals", "n", "p"}, variableNames(state))

	press := func(key tcell.Key) { handleKey(screen, state, tcell.NewEventKey(key, 0, 0)) }
	press(tcell.KeyEnd)
	press(tcell.KeyRight)
	pumpDebug(t, state, untilKind(debug.Expanded))
	assert.Equal(t, []string{"Locals", "n", "p", "X"}, variableNames(state))
	press(tcell.KeyDown)
	assert.Equal(t, 3, state.debugger.variable)
	press(tcell.KeyLeft)
	assert.Equal(t, 2, state.debugger.variable, "Left on a leaf selects its parent")
	press(tcell.KeyLeft)
	assert.Equal(t, []string{"Locals", "n", "p"}, variableNames(state))
	press(tcell.KeyEnter)
	assert.Equal(t, []string{"Locals", "n", "p", "X"}, variableNames(state), "Enter reopens without loading again")
	press(tcell.KeyEnter)
	assert.Equal(t, []string{"Locals", "n", "p"}, variableNames(state), "Enter also closes")
	assert.Equal(t, []string{"frame 1000", "variables 1", "variables 10"}, adapter.recordedInspections())
}

func TestDebugVariablesFollowTheSelectedFrame(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, screen := inspectState(t, adapter)
	handleKey(screen, state, callStackKey)
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0))
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	handleKey(screen, state, variablesKey)
	pumpDebug(t, state, untilKind(debug.Inspected))
	assert.Equal(t, 1001, state.debugger.tree.FrameID)

	handleKey(screen, state, variablesKey)
	assert.Equal(t, []string{"frame 1001", "variables 1"}, adapter.recordedInspections(), "loaded variables are reused")
}

func TestDebugVariablesReloadAtEachStop(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, screen := inspectState(t, adapter)
	handleKey(screen, state, variablesKey)
	pumpDebug(t, state, untilKind(debug.Inspected))
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF8, 0, 0))
	assert.NotNil(t, state.debugger.tree, "a step in flight keeps the variables for a refusal")
	adapter.events <- dap.Event{Name: "stopped", Reason: "step", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Stopped))
	assert.Nil(t, state.debugger.tree, "a new stop drops the old values")
	pumpDebug(t, state, untilKind(debug.Inspected))
	assert.Equal(t, []string{"Locals", "n", "p"}, variableNames(state))
	assert.Equal(t, []string{"frame 1000", "variables 1", "frame 1000", "variables 1"}, adapter.recordedInspections())
}

func TestDebugStackStaysClosedWithoutAPause(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, screen := inspectState(t, adapter)
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF8, 0, 0))
	adapter.events <- dap.Event{Name: "stopped", Reason: "step", ThreadID: 3}
	pumpDebug(t, state, untilKind(debug.Located))
	assert.Empty(t, adapter.recordedInspections(), "variables load only while their view is open")
}

func TestDebugInspectionFailures(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state, screen := inspectState(t, adapter)
	handleKey(screen, state, variablesKey)
	pumpDebug(t, state, untilKind(debug.Inspected))
	adapter.mu.Lock()
	adapter.varErr = errors.New("unreadable")
	adapter.mu.Unlock()
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnd, 0, 0))
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyRight, 0, 0))
	pumpDebug(t, state, untilKind(debug.Problem))
	assert.Contains(t, state.message, "unreadable")
	assert.False(t, state.debugger.tree.Rows()[2].Expanded, "a failed value closes so it can be opened again")

	handleKey(screen, state, callStackKey)
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0))
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	handleKey(screen, state, variablesKey)
	// The program moves on before Delve answers, which must not mark it paused again.
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF8, 0, 0))
	pumpDebug(t, state, untilKind(debug.Problem))
	assert.False(t, state.debugger.paused)
	assert.Empty(t, variableNames(state), "a frame whose variables failed shows none")
}

func TestDebugInspectionNeedsASession(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{projectRoot: runRoot}
	for _, key := range []*tcell.EventKey{callStackKey, variablesKey} {
		state.message = ""
		handleKey(screen, state, key)
		assert.Equal(t, bottomOutput, state.bottomMode)
		assert.Contains(t, state.message, "No debug session")
	}
	for _, item := range []int{debugMenuCallStack, debugMenuVariables} {
		state.message = ""
		state.runDebugMenuAction(item)
		assert.Contains(t, state.message, "No debug session")
	}
}

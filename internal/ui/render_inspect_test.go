package ui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
)

// inspectedRun is a paused run with two frames, the outer one selected, and its variables loaded.
func inspectedRun(root string) *debugRun {
	run := &debugRun{session: &debug.Session{}, paused: true, output: &jobView{command: "dlv test"}}
	run.frames = []dap.StackFrame{
		{ID: 1, Name: "demo.inner", Source: dap.Source{Path: filepath.Join(root, "demo", "demo.go")}, Line: 7},
		{ID: 2, Name: "runtime.goexit"},
	}
	run.frame, run.stackCursor = 1, 1
	run.tree = debug.NewTree(2, []debug.Scope{{Scope: dap.Scope{Name: "Locals", Reference: 1}, Loaded: true, Variables: []dap.Variable{
		{Name: "n", Value: "3", Type: "int"},
		{Name: "p", Value: "demo.Point {X: 1}", Type: "demo.Point", Reference: 10},
	}}})
	return run
}

func TestRenderCallStack(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 20)
	root := t.TempDir()
	state := shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomStack, debugger: inspectedRun(root)}
	render(screen, state)
	view := paneText(screen, 0, 19)
	assert.Contains(t, view, "CALL STACK  2 frame(s)")
	assert.Contains(t, view, "  demo.inner  "+filepath.Join("demo", "demo.go")+":7", "paths are relative to the project")
	assert.Contains(t, view, "► runtime.goexit", "the selected frame is marked")
	screen.SetSize(8, 4)
	assert.NotPanics(t, func() { render(screen, state) })
}

func TestRenderVariables(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 20)
	root := t.TempDir()
	run := inspectedRun(root)
	run.tree.Expand(2)
	state := shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomVariables, debugger: run}
	render(screen, state)
	view := paneText(screen, 0, 19)
	assert.Contains(t, view, "VARIABLES  runtime.goexit")
	assert.Contains(t, view, "▾ Locals")
	assert.Contains(t, view, "    n int = 3")
	assert.Contains(t, view, "  ▾ p demo.Point = demo.Point {X: 1} loading...")

	run.tree.Fill(10, nil)
	run.tree.Collapse(2)
	render(screen, state)
	assert.Contains(t, paneText(screen, 0, 19), "  ▸ p demo.Point = demo.Point {X: 1}")
}

func TestRenderVariablesScrolls(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(60, 12)
	root := t.TempDir()
	run := inspectedRun(root)
	variables := make([]dap.Variable, 30)
	for index := range variables {
		variables[index] = dap.Variable{Name: fmt.Sprint("v", index), Value: "0", Type: "int"}
	}
	run.tree = debug.NewTree(2, []debug.Scope{{Scope: dap.Scope{Name: "Locals", Reference: 1}, Loaded: true, Variables: variables}})
	run.variable = 30
	state := shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomVariables, debugger: run}
	render(screen, state)
	view := paneText(screen, 0, 11)
	assert.Contains(t, view, "v29 int = 0")
	assert.NotContains(t, view, "v0 int")
	assert.Contains(t, view, "/31")
}

func TestRenderInspectionPlaceholders(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	running := inspectedRun(root)
	running.paused = false
	loading := inspectedRun(root)
	loading.tree = nil
	empty := inspectedRun(root)
	empty.tree = debug.NewTree(2, nil)
	unlocated := inspectedRun(root)
	unlocated.frames = nil
	tests := []struct {
		name string
		mode bottomPaneMode
		run  *debugRun
		want string
	}{
		{"stack without a session", bottomStack, nil, "No debug session is running"},
		{"variables without a session", bottomVariables, nil, "No debug session is running"},
		{"ended session", bottomStack, &debugRun{output: &jobView{}}, "No debug session is running"},
		{"running stack", bottomStack, running, "The program is running"},
		{"running variables", bottomVariables, running, "The program is running"},
		{"stack on its way", bottomStack, unlocated, "loading..."},
		{"variables on their way", bottomVariables, loading, "loading..."},
		{"a frame without variables", bottomVariables, empty, "No variables"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(80, 20)
			render(screen, shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: test.mode, debugger: test.run})
			assert.Contains(t, paneText(screen, 0, 19), test.want)
		})
	}
}

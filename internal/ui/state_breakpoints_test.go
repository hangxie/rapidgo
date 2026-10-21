package ui

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

const breakpointSource = "package main\n\nfunc main() {\n\tprintln(1)\n\tprintln(2)\n}\n"

func breakpointLines(state *shellState, path string) []int {
	var lines []int
	for _, breakpoint := range state.breakpointSet().InFile(path) {
		lines = append(lines, breakpoint.Line)
	}
	return lines
}

func TestToggleBreakpointAtTheCaret(t *testing.T) {
	t.Parallel()

	state := &shellState{projectRoot: runRoot}
	state.toggleBreakpoint()
	assert.Contains(t, state.message, "Open a Go file")

	path := filepath.Join(runRoot, "main.go")
	setTestDocument(t, state, path, breakpointSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 3, Column: 2}, false))
	state.toggleBreakpoint()
	assert.Equal(t, []int{4}, breakpointLines(state, path))
	assert.Contains(t, state.message, "main.go:4")

	state.toggleBreakpoint()
	assert.Empty(t, breakpointLines(state, path))
	assert.Contains(t, state.message, "main.go:4")

	setTestDocument(t, state, filepath.Join(runRoot, "notes.txt"), "text\n")
	state.toggleBreakpoint()
	assert.Empty(t, state.breakpointSet().All(), "only Go files take breakpoints")
	assert.Contains(t, state.message, "Open a Go file")
}

func TestToggleBreakpointKey(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{projectRoot: runRoot, focus: focusEditor}
	path := filepath.Join(runRoot, "main.go")
	setTestDocument(t, state, path, breakpointSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 4}, false))
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyF8, 0, tcell.ModCtrl))
	assert.Equal(t, []int{5}, breakpointLines(state, path))
	assert.Equal(t, breakpointSource, state.buffer.Text())
}

func TestBreakpointsFollowEdits(t *testing.T) {
	t.Parallel()

	state := &shellState{projectRoot: runRoot}
	path := filepath.Join(runRoot, "main.go")
	setTestDocument(t, state, path, breakpointSource)
	state.trackBreakpoints()
	state.breakpointSet().Toggle(path, 4)

	require.NoError(t, state.buffer.Insert("// header\n"))
	state.trackBreakpoints()
	assert.Equal(t, []int{5}, breakpointLines(state, path))

	other := filepath.Join(runRoot, "other.go")
	setTestDocument(t, state, other, "package main\n")
	state.trackBreakpoints()
	assert.Equal(t, []int{5}, breakpointLines(state, path), "switching files is not an edit")
}

func TestBreakpointChangesReachTheRunningProgram(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	setTestDocument(t, state, path, debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.trackBreakpoints()
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))

	state.toggleBreakpoint()
	pumpDebug(t, state, untilKind(debug.Resolved))
	breakpoint, ok := state.breakpointSet().At(path, 6)
	require.True(t, ok)
	assert.True(t, breakpoint.Verified)

	require.NoError(t, state.buffer.MoveTo(editor.Position{}, false))
	require.NoError(t, state.buffer.Insert("// header\n"))
	state.trackBreakpoints()
	pumpDebug(t, state, untilKind(debug.Resolved))
	adapter.mu.Lock()
	assert.Equal(t, [][]int{{6}, {7}}, adapter.requested)
	adapter.mu.Unlock()

	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestBreakpointFailureWhileRunningIsReported(t *testing.T) {
	t.Parallel()

	adapter := newScriptedAdapter()
	state := debugTestState(t, adapter)
	path := filepath.Join(runRoot, "demo", "demo_test.go")
	setTestDocument(t, state, path, debugTestSource)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 5}, false))
	state.requestDebug()
	pumpDebug(t, state, untilKind(debug.Running))
	adapter.mu.Lock()
	adapter.misplace = errors.New("bad path")
	adapter.mu.Unlock()
	state.toggleBreakpoint()
	pumpDebug(t, state, untilKind(debug.Resolved))
	assert.Contains(t, state.message, "bad path")
	state.stopDebug()
	pumpDebug(t, state, nil)
}

func TestBreakpointsFollowFormattingOnSave(t *testing.T) {
	t.Parallel()

	for _, background := range []bool{false, true} {
		var requests []workRequest
		state := newShellState(runRoot, func(request workRequest) bool { requests = append(requests, request); return true })
		path := filepath.Join(runRoot, "a.go")
		state.installDocument(workResult{document: project.Document{Path: path, Text: breakpointSource}})
		state.trackBreakpoints()
		state.breakpointSet().Toggle(path, 4)
		state.requestSave()
		save := requests[len(requests)-1]
		if background {
			state.installDocument(workResult{document: project.Document{Path: filepath.Join(runRoot, "b.go"), Text: "package main\n"}})
			state.trackBreakpoints()
		}
		state.applySaveResult(workResult{request: save, saved: project.SaveResult{Content: "// header\n" + breakpointSource}})
		state.trackBreakpoints()
		assert.Equal(t, []int{5}, breakpointLines(&state, path), "background %v", background)
		if background {
			require.True(t, state.activateWindow(path))
			state.trackBreakpoints()
			assert.Equal(t, []int{5}, breakpointLines(&state, path))
		}
	}
}

func TestFirstEditAfterAnOutsideFileSwitchMovesBreakpoints(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := newShellState(runRoot, func(workRequest) bool { return true })
	path := filepath.Join(runRoot, "a.go")
	state.installDocument(workResult{document: project.Document{Path: path, Text: breakpointSource}})
	state.breakpointSet().Toggle(path, 4)
	state.installDocument(workResult{document: project.Document{Path: filepath.Join(runRoot, "b.go"), Text: "package main\n"}})
	state.trackBreakpoints()

	// A debugger stop or a jump activates a file without a key event.
	state.openAt(path, 1, 1)
	require.Equal(t, path, state.document.Path)
	handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	state.trackBreakpoints()
	assert.Equal(t, []int{5}, breakpointLines(&state, path))
}

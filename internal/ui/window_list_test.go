package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

func TestWindowListSelectsDuplicateView(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(80, 24)
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "/project/界.go", Text: "界\nhello"}})
	first := state.workspace.Current()
	state.windowAction(screen, windowNewView)
	duplicate := state.workspace.Current()
	require.NoError(t, duplicate.Buffer.MoveTo(editor.Position{Line: 1, Column: 2}, false))
	require.NoError(t, duplicate.Buffer.Insert("x"))
	state.installDocument(workResult{document: project.Document{Path: "other.go", Text: "other"}})
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRune, '0', tcell.ModAlt))
	render(screen, state)
	require.Contains(t, rowText(screen, 10, 0, 80), "界")
	require.Contains(t, rowText(screen, 10, 0, 80), ".go *")
	require.NotContains(t, rowText(screen, 10, 0, 80), "/project")
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone))
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	require.Same(t, duplicate, state.workspace.Current())
	require.NotSame(t, first.Buffer, state.buffer)
	require.Equal(t, editor.Position{Line: 1, Column: 3}, state.buffer.Cursor())
	require.Equal(t, focusEditor, state.focus)
}

func TestWindowListScrollsAndSelectsOriginalWindowAfterBackgroundOpen(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(40, 10)
	state := newShellState("/project", func(workRequest) bool { return true })
	for _, path := range []string{"a.go", "b.go", "c.go", "d.go", "last.go"} {
		state.installDocument(workResult{document: project.Document{Path: path, Text: path}})
	}
	last := state.workspace.Current()
	state.menuOpen, state.menuIndex, state.menuItem = true, menuWindow, windowList
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	state.installDocument(workResult{document: project.Document{Path: "background.go", Text: "new"}})
	for _, step := range []struct {
		key   tcell.Key
		index int
	}{{tcell.KeyDown, 0}, {tcell.KeyUp, 4}, {tcell.KeyHome, 0}, {tcell.KeyPgDn, 4}, {tcell.KeyPgUp, 0}, {tcell.KeyEnd, 4}} {
		handleKey(screen, &state, tcell.NewEventKey(step.key, 0, tcell.ModNone))
		require.Equal(t, step.index, state.windowChooser.index)
	}
	render(screen, state)
	require.Contains(t, rowText(screen, 6, 0, 40), "last.go")
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	require.Same(t, last, state.workspace.Current())
	require.Nil(t, state.windowChooser)
}

func TestWindowListQuitProtectsDirtyDocuments(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	require.NoError(t, state.buffer.Insert("x"))
	state.openWindowList()
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone)))
	require.Nil(t, state.windowChooser)
	require.Equal(t, confirmQuit, state.confirm)
}

func TestWindowListReportsRemovedSelection(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	state.openWindowList()
	state.workspace.CloseCurrent()
	state.loadWindow()
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)))
	require.Nil(t, state.windowChooser)
	require.Equal(t, "Selected window is no longer open", state.message)
	require.Equal(t, "a.go", state.document.Path)
}

func TestWindowListCancelAndEmptyWorkspace(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRune, '0', tcell.ModAlt))
	require.Equal(t, "No editor windows to list", state.message)
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.setFocus(focusTree)
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRune, '0', tcell.ModAlt))
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	for _, size := range [][2]int{{1, 1}, {12, 5}, {40, 10}, {80, 24}} {
		screen.SetSize(size[0], size[1])
		handleEvent(screen, &state, tcell.NewEventResize(size[0], size[1]))
		require.NotPanics(t, func() { render(screen, state) })
	}
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	require.Equal(t, focusTree, state.focus)
	require.Equal(t, "a", state.buffer.Text())
}

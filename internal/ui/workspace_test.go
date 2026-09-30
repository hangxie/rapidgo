package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/project"
	"github.com/hangxie/rapidgo/internal/workspace"
)

func TestWorkspaceEditing(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(100, 30)
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "界\nhello"}})
	_ = state.buffer.Insert("x")
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "second"}})
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModShift))
	if state.document.Path != "a.go" || state.buffer.Text() != "x界\nhello" {
		t.Fatal("switch lost unsaved document")
	}
	state.windowAction(screen, windowNewView)
	_ = state.buffer.Insert("y")
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone))
	if state.buffer.Text() != "xy界\nhello" || state.buffer.Cursor().Column != 2 {
		t.Fatal("duplicate did not share edits")
	}
	state.windowAction(screen, windowTile)
	for _, size := range [][2]int{{25, 9}, {1, 1}, {100, 30}} {
		screen.SetSize(size[0], size[1])
		handleEvent(screen, &state, tcell.NewEventResize(size[0], size[1]))
		render(screen, state)
	}
	state.windowAction(screen, windowCascade)
	if state.requestQuit() || state.confirm != confirmQuit {
		t.Fatal("quit missed dirty inactive document")
	}
}

func TestCloseWindowShortcutAndMenu(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF3, 0, tcell.ModAlt)))
	require.Equal(t, "a.go", state.document.Path)
	require.Len(t, state.workspace.Windows, 1)
	require.Equal(t, focusEditor, state.focus)
	state.menuOpen = true
	state.menuIndex, state.menuItem = menuWindow, windowClose
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)))
	require.Empty(t, state.workspace.Windows)
	require.Nil(t, state.document)
	require.Nil(t, state.buffer)
	require.Equal(t, focusTree, state.focus)
	state.windowAction(screen, windowClose)
	require.Equal(t, "No editor window to close", state.message)
}

func TestCloseDirtyWindowConfirmsOnlyLastView(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	require.NoError(t, state.buffer.Insert("x"))
	state.windowAction(screen, windowNewView)
	state.windowAction(screen, windowClose)
	require.Equal(t, confirmNone, state.confirm)
	require.Len(t, state.workspace.Windows, 1)
	require.True(t, state.buffer.Dirty())
	state.windowAction(screen, windowClose)
	require.Equal(t, confirmCloseWindow, state.confirm)
	require.Len(t, state.workspace.Windows, 1)
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	require.Len(t, state.workspace.Windows, 1)
	state.windowAction(screen, windowClose)
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRune, 'D', tcell.ModNone))
	require.Empty(t, state.workspace.Windows)
	require.Nil(t, state.buffer)
}

func TestCloseWindowWaitsForSaveAndCancelsPendingOpen(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.saving = true
	state.windowAction(screen, windowClose)
	require.Len(t, state.workspace.Windows, 1)
	require.Contains(t, state.message, "Save in progress")
	state.saving = false
	state.opening = true
	state.openSeq = 3
	state.windowAction(screen, windowClose)
	require.False(t, state.opening)
	require.Greater(t, state.openSeq, uint64(3))
}

func TestCloseWindowCancelsPendingJump(t *testing.T) {
	var request workRequest
	state := newShellState("/project", func(r workRequest) bool { request = r; return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "界"}})
	state.openAt("b.go", 2, 3)
	require.NotNil(t, state.pendingPosition)
	state.requestCloseWindow()
	require.Nil(t, state.pendingPosition)
	state.applyResult(workResult{request: request, document: project.Document{Path: "b.go", Text: "stale"}})
	require.Nil(t, state.buffer)
	state.installDocument(workResult{document: project.Document{Path: "c.go", Text: "界\nhello"}})
	require.Equal(t, 0, state.buffer.Cursor().Line)
	require.Equal(t, 0, state.buffer.Cursor().Column)
}

func TestWorkspaceCommandsWithoutDocument(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	state := newShellState("/project", func(workRequest) bool { return true })

	require.False(t, state.activateWindow("missing.go"))
	require.False(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone)))
	state.windowAction(screen, windowNewView)
	require.Equal(t, "Open a file before arranging windows", state.message)
	state.workspace = nil
	state.storeWindow()
	require.False(t, state.activateWindow("missing.go"))
	state.windowAction(screen, windowTile)
	require.Equal(t, "Open a file before arranging windows", state.message)
}

func TestWindowShortcutsAndSizing(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(100, 30)
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})

	require.False(t, state.activateWindow("missing.go"))
	require.True(t, state.activateWindow("a.go"))
	state.menuOpen = true
	require.False(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone)))
	state.menuOpen = false
	require.False(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModCtrl)))
	require.True(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone)))
	require.Equal(t, "b.go", state.document.Path)
	require.True(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModShift)))
	require.Equal(t, "a.go", state.document.Path)
	state.windowAction(screen, windowNext)
	require.Equal(t, "b.go", state.document.Path)
	state.windowAction(screen, windowPrevious)
	require.Equal(t, "a.go", state.document.Path)
	require.True(t, state.handleWindowShortcut(screen, tcell.NewEventKey(tcell.KeyF5, 0, tcell.ModCtrl)))
	require.True(t, state.windowSizing)

	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)))
	for _, key := range []tcell.Key{tcell.KeyRight, tcell.KeyUp, tcell.KeyDown} {
		require.False(t, state.handleWindowSizing(screen, tcell.NewEventKey(key, 0, tcell.ModNone)))
	}
	before := state.workspace.Current().Rect
	require.False(t, state.handleWindowSizing(screen, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModShift)))
	require.GreaterOrEqual(t, state.workspace.Current().Rect.Width, before.Width)
	require.False(t, state.handleWindowSizing(screen, tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone)))
	require.False(t, state.handleWindowSizing(screen, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)))
	require.False(t, state.windowSizing)
	require.Equal(t, "Window arranged", state.message)

	state.windowAction(screen, windowSizeMove)
	require.True(t, state.windowSizing)
	require.True(t, state.handleWindowSizing(screen, tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone)))
	require.False(t, state.windowSizing)
	state.menuOpen = true
	state.menuIndex, state.menuItem = menuWindow, windowNewView
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)))
	require.Equal(t, menuHelp, state.menuIndex)
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)))
	require.Equal(t, menuWindow, state.menuIndex)
	state.menuItem = windowSizeMove
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)))
	require.True(t, state.windowSizing)
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)))
	saved := state.workspace.Current().Rect
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF5, 0, tcell.ModNone)))
	area := calculateLayout(100, 30, false).editor
	require.Equal(t, workspace.Rect{Width: area.width, Height: area.height}, state.workspace.Current().Rect)
	require.False(t, handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF5, 0, tcell.ModNone)))
	require.Equal(t, saved, state.workspace.Current().Rect)
	state.windowAction(screen, windowZoom)
	require.Equal(t, workspace.Rect{Width: area.width, Height: area.height}, state.workspace.Current().Rect)
	state.windowAction(screen, windowZoom)
	require.Equal(t, saved, state.workspace.Current().Rect)
}

func TestInactiveWorkspaceDocumentIsDirty(t *testing.T) {
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	require.NoError(t, state.buffer.Insert("x"))
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	require.True(t, state.hasDirtyDocuments())
}

func TestQueueOpenActivatesExistingWindow(t *testing.T) {
	queued := 0
	state := newShellState("/project", func(workRequest) bool { queued++; return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	before := queued
	state.queueOpen("a.go", false)
	require.Equal(t, before, queued)
	require.Equal(t, "a.go", state.document.Path)
	require.False(t, state.opening)
	require.Equal(t, focusEditor, state.focus)

	state.enqueue = func(workRequest) bool { return false }
	state.queueOpen("missing.go", false)
	require.False(t, state.opening)
	require.Equal(t, errWorkQueueFull.Error(), state.message)
	var approved workRequest
	state.enqueue = func(request workRequest) bool { approved = request; return true }
	state.queueOpen("fresh.go", true)
	require.Equal(t, state.buffer.Text(), approved.approvedText)
}

func TestWorkspaceSaveKeepsNewerEdits(t *testing.T) {
	var save workRequest
	state := newShellState("/project", func(request workRequest) bool { save = request; return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	require.NoError(t, state.buffer.Insert("x"))
	state.requestSave()
	require.NoError(t, state.buffer.Insert("y"))
	background := state.buffer
	require.True(t, state.activateWindow("a.go"))
	state.applyDocumentSave(workResult{request: workRequest{path: "missing.go", seq: save.seq}})
	require.True(t, state.saving)
	state.applySaveResult(workResult{request: save, saved: project.SaveResult{Content: save.save.Content}})
	require.Equal(t, "a.go", state.document.Path)
	require.True(t, background.Dirty())
	require.Equal(t, "Saved b.go (newer edits remain unsaved)", state.message)
}

func TestWorkspaceSaveAfterSwitch(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	var requests []workRequest
	state := newShellState("/project", func(r workRequest) bool { requests = append(requests, r); return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	_ = state.buffer.Insert("x")
	first := state.buffer
	state.requestSave()
	request := requests[len(requests)-1]
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	state.applySaveResult(workResult{request: request, saved: project.SaveResult{Content: "xa"}})
	if state.saving || first.Dirty() || state.document.Path != "b.go" || state.buffer.Text() != "b" {
		t.Fatal("save applied to wrong document")
	}
	state.setFocus(focusEditor)
	state.windowAction(screen, windowSizeMove)
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModShift))
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if state.windowSizing {
		t.Fatal("placement did not finish")
	}
}

func TestInstallDocumentUsesSharedView(t *testing.T) {
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "original"}})
	first := state.buffer
	_ = first.Insert("x")
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "stale"}})
	if state.buffer.Text() != first.Text() {
		t.Fatal("installed view does not use the shared document")
	}
}

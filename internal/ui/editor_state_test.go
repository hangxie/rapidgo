package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

func TestEditorKeyboardAndDirtyQuit(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "a界\nb")
	key := func(code tcell.Key, value rune, mods tcell.ModMask) bool {
		return handleEvent(screen, &state, tcell.NewEventKey(code, value, mods))
	}
	key(tcell.KeyRight, 0, 0)
	key(tcell.KeyRight, 0, tcell.ModShift)
	assert.True(t, state.buffer.HasSelection())
	key(tcell.KeyRune, 'x', 0)
	assert.Equal(t, "ax\nb", state.buffer.Text())
	assert.True(t, state.buffer.Dirty())
	key(tcell.KeyCtrlZ, 0, 0)
	assert.Equal(t, "a界\nb", state.buffer.Text())
	assert.False(t, state.buffer.Dirty())
	key(tcell.KeyCtrlY, 0, 0)
	key(tcell.KeyEnter, 0, 0)
	key(tcell.KeyTab, 0, 0)
	assert.Equal(t, "ax\n\t\nb", state.buffer.Text())
	key(tcell.KeyBacktab, 0, 0)
	assert.Equal(t, "ax\n\nb", state.buffer.Text())
	assert.False(t, key(tcell.KeyCtrlQ, 0, 0))
	assert.Equal(t, confirmQuit, state.confirm)
	assert.False(t, key(tcell.KeyEscape, 0, 0))
	assert.Equal(t, confirmNone, state.confirm)
	assert.False(t, key(tcell.KeyCtrlQ, 0, 0))
	assert.True(t, key(tcell.KeyRune, 'D', 0))
}

func TestDirtyFileSwitchAndLateResult(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "next.go"}}})
	state.moveSelection(screen, 1)
	setTestDocument(t, &state, "/tmp/work/old.go", "old")
	state.focus = focusEditor
	state.insertRune(screen, 'x')
	state.focus = focusTree
	state.openSelected(screen)
	assert.Equal(t, confirmOpen, state.confirm)
	assert.Len(t, requests, 1)
	assert.False(t, state.handleConfirmation(tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.Equal(t, "xold", state.buffer.Text())
	state.openSelected(screen)
	state.handleConfirmation(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	require.Len(t, requests, 2)
	assert.True(t, requests[1].discardApproved)
	state.applyResult(workResult{request: requests[1], document: project.Document{Path: requests[1].path, Text: "new"}})
	assert.Equal(t, "new", state.buffer.Text())

	state.insertRune(screen, 'z')
	state.queueOpen("/tmp/work/third.go", false)
	late := requests[2]
	state.insertRune(screen, 'q')
	state.applyResult(workResult{request: late, document: project.Document{Path: late.path, Text: "third"}})
	assert.Equal(t, confirmLoaded, state.confirm)
	assert.Equal(t, "zqnew", state.buffer.Text())
	state.handleConfirmation(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	assert.Equal(t, "zqnew", state.buffer.Text())

	state.queueOpen("/tmp/work/fourth.go", true)
	approved := requests[3]
	state.insertRune(screen, 'r')
	state.applyResult(workResult{request: approved, document: project.Document{Path: approved.path, Text: "fourth"}})
	assert.Equal(t, confirmLoaded, state.confirm, "edits after discard approval need new confirmation")
	assert.Equal(t, "zqrnew", state.buffer.Text())
	state.handleConfirmation(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	assert.Equal(t, "fourth", state.buffer.Text())
}

func TestQuitPromptInvalidatesPendingOpen(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/work/old.go", "old")
	state.queueOpen("/tmp/work/new.go", false)
	old := requests[1]
	state.insertRune(screen, 'x')
	assert.False(t, state.requestQuit())
	state.applyResult(workResult{request: old, document: project.Document{Path: old.path, Text: "new"}})
	assert.Equal(t, confirmQuit, state.confirm)
	assert.Equal(t, "xold", state.buffer.Text())
}

func TestEditorCursorScrollAndTabDisplay(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 8)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "a\t界\nsecond\nthird\nfourth\nfifth")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 0, Column: 3}, false))
	state.ensureCursorVisible(screen)
	assert.Equal(t, 7, visualColumn(state.buffer.Lines()[0], 3))
	assert.Zero(t, state.fileColumn)
	render(screen, state)
	x, y, visible := screen.GetCursor()
	assert.True(t, visible)
	assert.Equal(t, 13, x) // Five-cell gutter plus seven displayed cells.
	assert.Equal(t, 2, y)
	require.NoError(t, state.buffer.Select(editor.Position{Line: 0}, editor.Position{Line: 0, Column: 1}))
	render(screen, state)
	_, style, _ := screen.Get(6, 2)
	foreground, background, _ := style.Decompose()
	assert.Equal(t, turboBlack, foreground)
	assert.Equal(t, turboLightCyan, background)
	state.helpVisible = true
	render(screen, state)
	_, _, visible = screen.GetCursor()
	assert.False(t, visible)
	state.helpVisible = false
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 0, Column: 3}, false))
	for range 4 {
		handleEditorKey(screen, &state, tcell.NewEventKey(tcell.KeyDown, 0, 0))
	}
	assert.Positive(t, state.fileScroll)
	assert.Equal(t, editor.Position{Line: 4, Column: 3}, state.buffer.Cursor())
}

package ui

import (
	"errors"
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

func TestSaveResultFormatsAndMarksBufferClean(t *testing.T) {
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
	setTestDocument(t, &state, "/tmp/work/main.go", "package main\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1}, false))
	require.NoError(t, state.buffer.Insert("func main(){println(1)}\n"))
	state.requestSave()
	require.Len(t, requests, 2)
	request := requests[1]
	assert.Equal(t, saveFile, request.kind)
	assert.True(t, state.saving)
	assert.False(t, state.requestQuit(), "quitting waits for the save")
	state.applySaveResult(workResult{request: request, saved: project.SaveResult{Content: "package main\nfunc main() { println(1) }\n"}})
	assert.False(t, state.saving)
	assert.False(t, state.buffer.Dirty())
	assert.Equal(t, "package main\nfunc main() { println(1) }\n", state.buffer.Text())
	assert.Equal(t, state.document.Text, state.buffer.SerializedText())
	assert.True(t, state.buffer.Undo(), "formatter changes remain undoable")
	assert.Equal(t, request.save.Content, state.buffer.Text())
	assert.True(t, state.buffer.Dirty())
}

func TestSaveFailureAndConcurrentEditsRemainDirty(t *testing.T) {
	t.Parallel()
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/work/main.go", "old")
	require.NoError(t, state.buffer.Insert("a"))
	state.requestSave()
	first := requests[1]
	writeErr := errors.New("write failed")
	state.applySaveResult(workResult{request: first, err: writeErr})
	assert.True(t, state.buffer.Dirty())
	assert.Equal(t, "old", state.document.Text)
	assert.Contains(t, state.message, "write failed")

	state.requestSave()
	second := requests[2]
	require.NoError(t, state.buffer.Insert("b"))
	state.applySaveResult(workResult{request: second, saved: project.SaveResult{Content: "aold"}})
	assert.Equal(t, "aold", state.document.Text)
	assert.Equal(t, "abold", state.buffer.Text())
	assert.True(t, state.buffer.Dirty())
	assert.Contains(t, state.message, "newer edits")
	state.requestSave()
	assert.Equal(t, "aold", requests[3].save.Original, "next save compares with the last written bytes")
}

func TestSaveRequestPreservesOriginalAndSerializedCRLF(t *testing.T) {
	t.Parallel()
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/work/notes.txt", "a\r\nb\r\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 1}, false))
	require.NoError(t, state.buffer.Insert("c"))
	state.requestSave()
	require.Len(t, requests, 2)
	assert.Equal(t, "a\r\nb\r\n", requests[1].save.Original)
	assert.Equal(t, "a\r\nbc\r\n", requests[1].save.Content)
}

func TestSaveCancelsPendingFileOpen(t *testing.T) {
	t.Parallel()
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/work/old.go", "old")
	state.queueOpen("/tmp/work/new.go", false)
	pending := requests[1]
	state.requestSave()
	assert.Greater(t, state.openSeq, pending.seq)
	state.applyResult(workResult{request: pending, document: project.Document{Path: pending.path, Text: "new"}})
	assert.Equal(t, "/tmp/work/old.go", state.document.Path)
}

func TestFormattedSaveDoesNotReplaceNewerEdits(t *testing.T) {
	t.Parallel()
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/work/main.go", "package main\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1}, false))
	require.NoError(t, state.buffer.Insert("func main(){ }\n"))
	state.requestSave()
	request := requests[1]
	require.NoError(t, state.buffer.Insert("// later"))
	state.applySaveResult(workResult{request: request, saved: project.SaveResult{Content: "package main\nfunc main() {}\n"}})
	assert.Equal(t, "package main\nfunc main(){ }\n// later", state.buffer.Text())
	assert.Equal(t, "package main\nfunc main() {}\n", state.document.Text)
	assert.True(t, state.buffer.Dirty())
}

func TestSearchPromptAndWrap(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "one two one")
	key := func(code tcell.Key, value rune) {
		assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(code, value, 0)))
	}
	key(tcell.KeyCtrlF, 0)
	assert.True(t, state.searching)
	for _, letter := range "one" {
		key(tcell.KeyRune, letter)
	}
	render(screen, state)
	_, _, visible := screen.GetCursor()
	assert.True(t, visible, "search input owns the cursor")
	key(tcell.KeyEnter, 0)
	assert.False(t, state.searching)
	start, end, selected := state.buffer.Selection()
	assert.True(t, selected)
	assert.Equal(t, editor.Position{}, start)
	assert.Equal(t, editor.Position{Column: 3}, end)
	key(tcell.KeyCtrlG, 0)
	assert.Equal(t, editor.Position{Column: 8}, state.buffer.Anchor())
	key(tcell.KeyCtrlG, 0)
	assert.Equal(t, editor.Position{}, state.buffer.Anchor())
	assert.Contains(t, state.message, "wrapped")
}

func TestSearchPromptCancelAndGraphemeBackspace(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 8)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "e\u0301")
	key := func(code tcell.Key, value rune) {
		assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(code, value, 0)))
	}
	key(tcell.KeyCtrlF, 0)
	key(tcell.KeyRune, 'e')
	key(tcell.KeyRune, '\u0301')
	assert.Equal(t, "e\u0301", state.searchInput)
	key(tcell.KeyBackspace2, 0)
	assert.Empty(t, state.searchInput)
	key(tcell.KeyRune, 'z')
	key(tcell.KeyEnter, 0)
	assert.Contains(t, state.message, "Not found")
	key(tcell.KeyCtrlF, 0)
	key(tcell.KeyEscape, 0)
	assert.False(t, state.searching)
	assert.Equal(t, "z", state.searchQuery, "cancel keeps the prior query")
}

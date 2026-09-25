package ui

import (
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/editor"
)

func (state *shellState) requestQuit() bool {
	if state.buffer != nil && state.buffer.Dirty() {
		state.openSeq++ // Pending file reads must not replace the discard prompt.
		state.opening = false
		state.confirm = confirmQuit
		state.message = "Unsaved changes: press D to discard and quit, Esc to cancel"
		return false
	}
	return true
}

func (state *shellState) handleConfirmation(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEscape {
		state.clearConfirmation()
		state.message = "Cancelled; unsaved changes remain"
		return false
	}
	if event.Key() != tcell.KeyRune || event.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) != 0 || (event.Rune() != 'd' && event.Rune() != 'D') {
		return false
	}
	action, path, result := state.confirm, state.pendingPath, state.pendingFile
	state.clearConfirmation()
	switch action {
	case confirmQuit:
		return true
	case confirmOpen:
		state.queueOpen(path, true)
	case confirmLoaded:
		if result != nil {
			state.installDocument(*result)
		}
	}
	return false
}

func (state *shellState) clearConfirmation() {
	state.confirm = confirmNone
	state.pendingPath = ""
	state.pendingFile = nil
}

func (state *shellState) insertRune(screen tcell.Screen, value rune) {
	if unicode.IsControl(value) || state.buffer == nil {
		return
	}
	state.edit(screen, string(value))
}

func (state *shellState) edit(screen tcell.Screen, value string) {
	if err := state.buffer.Insert(value); err != nil {
		state.message = err.Error()
		return
	}
	state.message = "Modified " + state.document.Path + " (save comes next)"
	state.ensureCursorVisible(screen)
}

func handleEditorKey(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	buffer := state.buffer
	if buffer == nil {
		return false
	}
	shift := event.Modifiers()&tcell.ModShift != 0
	ctrl := event.Modifiers()&tcell.ModCtrl != 0
	switch event.Key() {
	case tcell.KeyLeft:
		buffer.MoveLeft(shift)
	case tcell.KeyRight:
		buffer.MoveRight(shift)
	case tcell.KeyUp:
		buffer.MoveUp(shift)
	case tcell.KeyDown:
		buffer.MoveDown(shift)
	case tcell.KeyHome:
		if ctrl {
			_ = buffer.MoveTo(editor.Position{}, shift)
		} else {
			buffer.MoveHome(shift)
		}
	case tcell.KeyEnd:
		if ctrl {
			lines := buffer.Lines()
			_ = buffer.MoveTo(editor.Position{Line: len(lines) - 1}, shift)
		}
		buffer.MoveEnd(shift)
	case tcell.KeyPgUp, tcell.KeyPgDn:
		rows := max(1, calculateLayoutSize(screen).editor.height-2)
		for range rows {
			if event.Key() == tcell.KeyPgUp {
				buffer.MoveUp(shift)
			} else {
				buffer.MoveDown(shift)
			}
		}
	case tcell.KeyEnter:
		state.edit(screen, "\n")
	case tcell.KeyTab:
		if shift {
			state.unindent(screen)
		} else {
			state.edit(screen, "\t")
		}
	case tcell.KeyBacktab:
		state.unindent(screen)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		buffer.DeleteBackward()
	case tcell.KeyDelete:
		buffer.DeleteForward()
	case tcell.KeyCtrlA:
		lines := buffer.Lines()
		last := len(lines) - 1
		_ = buffer.Select(editor.Position{}, editor.Position{Line: last, Column: graphemeCount(lines[last])})
	case tcell.KeyCtrlZ:
		buffer.Undo()
	case tcell.KeyCtrlY:
		buffer.Redo()
	}
	state.ensureCursorVisible(screen)
	return false
}

func (state *shellState) unindent(screen tcell.Screen) {
	position := state.buffer.Cursor()
	line := state.buffer.Lines()[position.Line]
	count := 0
	if len(line) > 0 && line[0] == '\t' {
		count = 1
	} else {
		for count < 4 && count < len(line) && line[count] == ' ' {
			count++
		}
	}
	if count == 0 {
		return
	}
	_ = state.buffer.Select(editor.Position{Line: position.Line}, editor.Position{Line: position.Line, Column: count})
	_ = state.buffer.Insert("")
	_ = state.buffer.MoveTo(editor.Position{Line: position.Line, Column: max(0, position.Column-count)}, false)
	state.message = "Modified " + state.document.Path + " (save comes next)"
	state.ensureCursorVisible(screen)
}

func (state *shellState) ensureCursorVisible(screen tcell.Screen) {
	if state.buffer == nil {
		return
	}
	area := calculateLayoutSize(screen).editor
	rows := area.height - 2
	if rows <= 0 {
		return
	}
	position := state.buffer.Cursor()
	if position.Line < state.fileScroll {
		state.fileScroll = position.Line
	} else if position.Line >= state.fileScroll+rows {
		state.fileScroll = position.Line - rows + 1
	}
	width := editorTextWidth(area)
	if width <= 0 {
		return
	}
	line := state.buffer.Lines()[position.Line]
	column := visualColumn(line, position.Column)
	if column < state.fileColumn {
		state.fileColumn = column
	} else if column >= state.fileColumn+width {
		state.fileColumn = column - width + 1
	}
}

func graphemeCount(value string) int {
	count := 0
	clusters := uniseg.NewGraphemes(value)
	for clusters.Next() {
		count++
	}
	return count
}

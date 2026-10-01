package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/workspace"
)

func (state *shellState) storeWindow() {
	if state.workspace == nil {
		return
	}
	if win := state.workspace.Current(); win != nil {
		win.Scroll, win.Column = state.fileScroll, state.fileColumn
	}
}

func (state *shellState) loadWindow() {
	if win := state.workspace.Current(); win != nil {
		state.document, state.buffer = win.Document, win.Buffer
		state.fileScroll, state.fileColumn = win.Scroll, win.Column
		state.cancelPendingCompletion()
		state.hoverVisible = false
	}
}

func (state *shellState) activateWindow(path string) bool {
	if state.workspace == nil {
		return false
	}
	state.storeWindow()
	if !state.workspace.Activate(path) {
		return false
	}
	state.loadWindow()
	return true
}

func (state *shellState) nextWindow(screen tcell.Screen, delta int) {
	state.storeWindow()
	state.workspace.Next(delta)
	state.loadWindow()
	state.focusSeq++
	state.setFocus(focusEditor)
	state.ensureCursorVisible(screen)
}

func (state *shellState) hasDirtyDocuments() bool {
	if state.buffer != nil && state.buffer.Dirty() {
		return true
	}
	if state.workspace != nil {
		for _, win := range state.workspace.Windows {
			if win.Buffer.Dirty() {
				return true
			}
		}
	}
	return false
}

func (state *shellState) requestCloseWindow() {
	if state.workspace == nil || state.workspace.Current() == nil {
		state.message = i18n.Text("msg_no_editor_window_to_close")
		return
	}
	if state.saving {
		state.message = i18n.Text("msg_save_in_progress_wait_before_closing_a_window")
		return
	}
	state.openSeq++
	state.opening = false
	state.pendingPosition = nil
	current := state.workspace.Current()
	if current.Buffer.Dirty() {
		lastView := true
		for _, win := range state.workspace.Windows {
			if win != current && win.Document == current.Document {
				lastView = false
				break
			}
		}
		if lastView {
			state.confirm = confirmCloseWindow
			state.message = i18n.Text("msg_unsaved_changes_press_d_to_discard_and_close_window_esc_to_cancel")
			return
		}
	}
	state.closeWindow()
}

func (state *shellState) closeWindow() {
	state.storeWindow()
	closed := state.workspace.CloseCurrent()
	state.focusSeq++
	state.navigationSeq++
	state.hoverSeq++
	state.cancelPendingCompletion()
	state.completionVisible = false
	state.hoverVisible = false
	if state.workspace.Current() != nil {
		state.loadWindow()
		state.setFocus(focusEditor)
	} else {
		state.document, state.buffer = nil, nil
		state.fileScroll, state.fileColumn = 0, 0
		state.setFocus(focusTree)
	}
	state.message = i18n.Format("msg_closed_s", closed.Document.Path)
}

func (state *shellState) windowAction(screen tcell.Screen, item int) {
	if item == windowClose {
		state.requestCloseWindow()
		return
	}
	if item == windowList {
		state.openWindowList()
		return
	}
	if state.workspace == nil || state.workspace.Current() == nil {
		state.message = i18n.Text("msg_open_a_file_before_arranging_windows")
		return
	}
	state.storeWindow()
	width, height := screen.Size()
	area := calculateLayout(width, height, state.focus == focusOutput).editor
	state.workspace.Resize(area.width, area.height)
	switch item {
	case windowNewView:
		state.workspace.Duplicate()
		state.loadWindow()
		state.setFocus(focusEditor)
	case windowNext:
		state.nextWindow(screen, 1)
	case windowPrevious:
		state.nextWindow(screen, -1)
	case windowTile:
		state.workspace.Arrange(workspace.Tile, area.width, area.height)
	case windowCascade:
		state.workspace.Arrange(workspace.Cascade, area.width, area.height)
	case windowSizeMove:
		state.windowSizing = true
		state.message = i18n.Text("msg_move_arrows_resize_shift_arrows_enter_esc_to_finish")
	case windowZoom:
		if state.workspace.Zoom() {
			state.message = i18n.Text("msg_window_zoomed")
		} else {
			state.message = i18n.Text("msg_window_restored")
		}
	}
	state.ensureCursorVisible(screen)
}

func (state *shellState) handleWindowSizing(screen tcell.Screen, event *tcell.EventKey) bool {
	dx, dy := 0, 0
	switch event.Key() {
	case tcell.KeyLeft:
		dx = -1
	case tcell.KeyRight:
		dx = 1
	case tcell.KeyUp:
		dy = -1
	case tcell.KeyDown:
		dy = 1
	case tcell.KeyEnter, tcell.KeyEscape:
		state.windowSizing = false
		state.message = i18n.Text("msg_window_arranged")
		return false
	case tcell.KeyCtrlQ, tcell.KeyCtrlC:
		state.windowSizing = false
		return state.requestQuit()
	default:
		return false
	}
	width, height := screen.Size()
	area := calculateLayout(width, height, state.focus == focusOutput).editor
	dw, dh := 0, 0
	if event.Modifiers()&tcell.ModShift != 0 {
		dw, dh = dx, dy
		dx, dy = 0, 0
	}
	state.workspace.Adjust(dx, dy, dw, dh, area.width, area.height)
	state.ensureCursorVisible(screen)
	return false
}

func windowRectangle(area rectangle, r workspace.Rect) rectangle {
	return rectangle{x: area.x + r.X, y: area.y + r.Y, width: r.Width, height: r.Height}
}

func (state *shellState) handleWindowShortcut(screen tcell.Screen, event *tcell.EventKey) bool {
	if state.menuOpen {
		return false
	}
	if event.Key() == tcell.KeyF5 && event.Modifiers() == tcell.ModNone {
		state.windowAction(screen, windowZoom)
		return true
	}
	if event.Key() == tcell.KeyF5 && event.Modifiers() == tcell.ModCtrl {
		state.windowAction(screen, windowSizeMove)
		return true
	}
	if event.Key() != tcell.KeyF6 || state.workspace == nil || len(state.workspace.Windows) == 0 {
		return false
	}
	switch event.Modifiers() {
	case tcell.ModNone:
		state.nextWindow(screen, 1)
	case tcell.ModShift:
		state.nextWindow(screen, -1)
	default:
		return false
	}
	return true
}

func (state *shellState) handleWindowFunctionKey(screen tcell.Screen, event *tcell.EventKey) {
	if state.handleWindowShortcut(screen, event) {
		return
	}
	if event.Key() == tcell.KeyF6 && (event.Modifiers() == tcell.ModNone || event.Modifiers() == tcell.ModCtrl) && !state.menuOpen {
		state.focusSeq++
		state.focusNextPane(screen)
	}
}

package ui

import (
	"github.com/gdamore/tcell/v2"

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

func (state *shellState) windowAction(screen tcell.Screen, item int) {
	if state.workspace == nil || state.workspace.Current() == nil {
		state.message = "Open a file before arranging windows"
		return
	}
	state.storeWindow()
	width, height := screen.Size()
	area := calculateLayout(width, height, state.focus == focusOutput).editor
	state.workspace.Resize(area.width, area.height)
	switch item {
	case 0:
		state.workspace.Duplicate()
		state.loadWindow()
		state.setFocus(focusEditor)
	case 1:
		state.nextWindow(screen, 1)
	case 2:
		state.nextWindow(screen, -1)
	case 3:
		state.workspace.Arrange(workspace.Tile, area.width, area.height)
	case 4:
		state.workspace.Arrange(workspace.Cascade, area.width, area.height)
	case 5:
		state.windowSizing = true
		state.message = "Move: arrows; resize: Shift+arrows; Enter/Esc to finish"
	case 6:
		if state.workspace.Zoom() {
			state.message = "Window zoomed"
		} else {
			state.message = "Window restored"
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
		state.message = "Window arranged"
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
		state.windowAction(screen, 6)
		return true
	}
	if event.Key() == tcell.KeyF5 && event.Modifiers() == tcell.ModCtrl {
		state.windowAction(screen, 5)
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

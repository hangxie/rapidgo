package ui

import "github.com/gdamore/tcell/v2"

func renderWorkspace(screen tcell.Screen, area rectangle, state shellState) {
	if state.workspace == nil || len(state.workspace.Windows) == 0 {
		renderDocument(screen, area, state)
		return
	}
	state.storeWindow()
	state.workspace.Resize(area.width, area.height)
	active := state.workspace.Current()
	for layer := range state.workspace.Windows {
		win := state.workspace.WindowAt(layer)
		view := state
		view.document, view.buffer = win.Document, win.Buffer
		view.fileScroll, view.fileColumn = win.Scroll, win.Column
		if win != active {
			view.focus = focusTree
			view.languageStatus = ""
			view.languageDiagnostics = nil
		}
		r := windowRectangle(area, win.Rect)
		if r.width == 0 || r.height == 0 {
			continue
		}
		for y := r.y; y < r.y+r.height; y++ {
			for x := r.x; x < r.x+r.width; x++ {
				screen.SetContent(x, y, ' ', nil, baseStyle)
			}
		}
		renderDocument(screen, r, view)
	}
}

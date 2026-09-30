package ui

import "github.com/gdamore/tcell/v2"

func renderWorkspace(screen tcell.Screen, area rectangle, state shellState) {
	if state.workspace == nil || len(state.workspace.Windows) == 0 {
		renderDocument(screen, area, state)
		return
	}
	state.storeWindow()
	state.workspace.Resize(area.width, area.height)
	active := state.workspace.Active
	order := make([]int, 0, len(state.workspace.Windows))
	for i := range state.workspace.Windows {
		if i != active {
			order = append(order, i)
		}
	}
	order = append(order, active)
	for _, i := range order {
		win := state.workspace.Windows[i]
		view := state
		view.document, view.buffer = win.Document, win.Buffer
		view.fileScroll, view.fileColumn = win.Scroll, win.Column
		if i != active {
			view.focus = focusTree
		}
		r := windowRectangle(area, win.Rect)
		for y := r.y; y < r.y+r.height; y++ {
			for x := r.x; x < r.x+r.width; x++ {
				screen.SetContent(x, y, ' ', nil, baseStyle)
			}
		}
		renderDocument(screen, r, view)
	}
}

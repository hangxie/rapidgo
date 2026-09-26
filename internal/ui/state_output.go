package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// outputRows is how many output lines fit in the pane at its current size.
func (state *shellState) outputRows(screen tcell.Screen) int {
	return max(1, state.layout(screen).output.height-2)
}

// keepOutputAnchored re-clamps each viewport after the pane changes size.
func (state *shellState) keepOutputAnchored(screen tcell.Screen) {
	rows := state.outputRows(screen)
	for _, view := range state.views {
		if !view.follow {
			view.scrollTo(view.scroll, rows)
		}
	}
}

// handleOutputKey scrolls the pane and switches between the job views.
func (state *shellState) handleOutputKey(screen tcell.Screen, event *tcell.EventKey) {
	view := state.activeView()
	if view == nil {
		return
	}
	rows := state.outputRows(screen)
	switch event.Key() {
	case tcell.KeyUp:
		view.moveSelection(-1, rows)
	case tcell.KeyDown:
		view.moveSelection(1, rows)
	case tcell.KeyPgUp:
		view.moveSelection(-rows, rows)
	case tcell.KeyPgDn:
		view.moveSelection(rows, rows)
	case tcell.KeyHome:
		view.selectLine(0, rows)
	case tcell.KeyEnd:
		view.selectLine(len(view.lines)-1, rows)
		view.follow = true
	case tcell.KeyLeft:
		state.showAdjacentJob(-1)
	case tcell.KeyRight:
		state.showAdjacentJob(1)
	case tcell.KeyEnter:
		state.jumpToSelected()
	}
}

// moveSelection moves the highlighted line, holding its row in the pane.
func (view *jobView) moveSelection(delta, rows int) {
	offset := view.selected - view.top(rows)
	view.place(view.selected+delta, view.selected+delta-offset, rows)
}

// selectLine highlights a line and brings it into view.
func (view *jobView) selectLine(index, rows int) {
	view.place(index, index, rows)
}

// place sets the selection and first visible line, keeping the two consistent.
func (view *jobView) place(selected, top, rows int) {
	if len(view.lines) == 0 {
		return
	}
	view.selected = max(0, min(selected, len(view.lines)-1))
	switch {
	case view.selected < top:
		top = view.selected
	case view.selected >= top+rows:
		top = view.selected - rows + 1
	}
	view.scrollTo(top, rows)
	view.follow = view.selected == len(view.lines)-1
}

// showAdjacentJob moves between the kinds that have output.
func (state *shellState) showAdjacentJob(step int) {
	order := state.jobOrder()
	if len(order) < 2 {
		return
	}
	current := 0
	for index, kind := range order {
		if kind == state.visibleJob {
			current = index
			break
		}
	}
	state.visibleJob = order[(current+step+len(order))%len(order)]
	state.message = "Showing " + state.views[state.visibleJob].command
}

// jobOrder lists the kinds that have run, in menu order.
func (state *shellState) jobOrder() []jobs.Kind {
	var order []jobs.Kind
	for _, kind := range []jobs.Kind{jobs.Build, jobs.Test, jobs.Run} {
		if state.views[kind] != nil {
			order = append(order, kind)
		}
	}
	return order
}

func (view *jobView) scrollTo(top, rows int) {
	last := max(0, len(view.lines)-rows)
	view.scroll = max(0, min(top, last))
	view.follow = view.scroll == last
}

// top is the first visible line, tracking the end while following.
func (view *jobView) top(rows int) int {
	if view.follow {
		return max(0, len(view.lines)-rows)
	}
	return max(0, min(view.scroll, max(0, len(view.lines)-rows)))
}

// visibleLines returns the slice of output the pane should draw.
func (view *jobView) visibleLines(rows int) []outputLine {
	top := view.top(rows)
	end := min(len(view.lines), top+rows)
	return view.lines[top:end]
}

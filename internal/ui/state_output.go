package ui

import (
	"slices"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// outputRows is how many output lines fit in the pane at its current size.
func (state *shellState) outputRows(screen tcell.Screen) int {
	return max(1, state.layout(screen).output.height-2)
}

// keepOutputAnchored re-clamps each viewport after the pane changes size.
func (state *shellState) keepOutputAnchored(screen tcell.Screen) {
	rows := state.outputRows(screen)
	if state.bottomMode == bottomErrors {
		items := state.currentErrorItems()
		state.errorScroll = max(0, min(state.errorScroll, max(0, len(items)-rows)))
	}
	for _, view := range state.outputViews() {
		if !view.follow {
			view.scrollTo(view.scroll, rows)
		}
	}
}

// handleOutputKey scrolls the pane and switches between the job views.
func (state *shellState) handleOutputKey(screen tcell.Screen, event *tcell.EventKey) {
	if state.bottomMode == bottomErrors {
		state.handleErrorsKey(screen, event)
		return
	}
	switch state.bottomMode {
	case bottomLocations:
		state.handleLocationsKey(screen, event)
		return
	case bottomStack:
		state.handleStackKey(screen, event)
		return
	case bottomVariables:
		state.handleVariablesKey(screen, event)
		return
	}
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

// showAdjacentJob moves between the outputs that exist.
func (state *shellState) showAdjacentJob(step int) {
	order := state.outputViews()
	if len(order) < 2 {
		return
	}
	current := max(0, slices.Index(order, state.activeView()))
	next := order[(current+step+len(order))%len(order)]
	state.showingDebug = state.debugger != nil && next == state.debugger.output
	for kind, view := range state.views {
		if view == next {
			state.visibleJob = kind
		}
	}
	state.message = i18n.Format("msg_showing_s", next.command)
}

// outputViews lists the job kinds that have run, in menu order, then the debug session.
func (state *shellState) outputViews() []*jobView {
	var order []*jobView
	for _, kind := range []jobs.Kind{jobs.Build, jobs.Test, jobs.Run} {
		if view := state.views[kind]; view != nil {
			order = append(order, view)
		}
	}
	if state.debugger != nil {
		order = append(order, state.debugger.output)
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

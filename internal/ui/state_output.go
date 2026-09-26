package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// outputRows is how many output lines fit in the pane at its current size.
func (state *shellState) outputRows(screen tcell.Screen) int {
	return max(1, state.layout(screen).output.height-2)
}

// keepOutputAnchored re-clamps each viewport after the pane changes size and
// resumes following when the new viewport reaches the last line. Growing the
// terminal can bring a scrolled view to the end of its output, and clamping
// alone would leave it sitting there without tailing. The pane also resizes
// when it gains or loses focus, which this covers for the same reason.
func (state *shellState) keepOutputAnchored(screen tcell.Screen) {
	rows := state.outputRows(screen)
	for _, view := range state.views {
		if !view.follow {
			view.scrollTo(view.scroll, rows)
		}
	}
}

// handleOutputKey scrolls the output pane and switches between the build,
// test, and run views.
func (state *shellState) handleOutputKey(screen tcell.Screen, event *tcell.EventKey) {
	view := state.activeView()
	if view == nil {
		return
	}
	rows := state.outputRows(screen)
	switch event.Key() {
	case tcell.KeyUp:
		view.scrollBy(-1, rows)
	case tcell.KeyDown:
		view.scrollBy(1, rows)
	case tcell.KeyPgUp:
		view.scrollBy(-rows, rows)
	case tcell.KeyPgDn:
		view.scrollBy(rows, rows)
	case tcell.KeyHome:
		view.scrollTo(0, rows)
	case tcell.KeyEnd:
		view.follow = true
	case tcell.KeyLeft:
		state.showAdjacentJob(-1)
	case tcell.KeyRight:
		state.showAdjacentJob(1)
	}
}

// showAdjacentJob moves to the next or previous kind that has output, so a
// build result stays reachable after a test run replaces the pane.
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

// scrollBy moves the viewport and leaves follow mode unless the move reaches
// the last line, where a running command should keep tailing its output.
func (view *jobView) scrollBy(delta, rows int) {
	view.scrollTo(view.top(rows)+delta, rows)
}

func (view *jobView) scrollTo(top, rows int) {
	last := max(0, len(view.lines)-rows)
	view.scroll = max(0, min(top, last))
	view.follow = view.scroll == last
}

// top is the first visible line, which tracks the end of the output while the
// view is following it.
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

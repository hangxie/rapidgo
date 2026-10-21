package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/i18n"
)

// inspection is what the debugger views show of the current pause; references in it expire when the program moves.
type inspection struct {
	frames      []dap.StackFrame
	frame       int // the selected frame, whose source and variables the views show
	stackCursor int
	stackScroll int
	tree        *debug.Tree
	inspecting  int  // the frame whose variables were last requested
	loading     bool // that request has not been answered
	variable    int  // the selected row of tree
	varScroll   int
}

// forgetStack drops what the views show once the program has moved, keeping where the variables view was scrolled.
func (run *debugRun) forgetStack() {
	run.frames, run.frame, run.stackCursor, run.stackScroll = nil, 0, 0, 0
	run.tree, run.loading = nil, false
}

func (run *debugRun) selectedFrame() (dap.StackFrame, bool) {
	if run == nil || run.frame >= len(run.frames) {
		return dap.StackFrame{}, false
	}
	return run.frames[run.frame], true
}

func (state *shellState) openCallStack() {
	state.openDebugView(bottomStack)
}

func (state *shellState) openVariables() {
	if state.openDebugView(bottomVariables) {
		state.inspectFrame()
	}
}

func (state *shellState) openDebugView(mode bottomPaneMode) bool {
	state.menuOpen = false
	if !state.debugger.active() {
		state.message = i18n.Text("msg_no_debug_session_is_running")
		return false
	}
	state.bottomMode = mode
	state.focusSeq++
	state.setFocus(focusOutput)
	return true
}

// showStack keeps the stack Delve reported and loads its variables when their view is open.
func (state *shellState) showStack(event debug.Event) {
	run := state.debugger
	if event.ThreadID != run.threadID {
		return
	}
	run.frames, run.frame = event.Frames, 0
	if state.bottomMode == bottomVariables {
		state.inspectFrame()
	}
}

// inspectFrame requests the selected frame's variables unless they are loaded or on their way.
func (state *shellState) inspectFrame() {
	run := state.debugger
	frame, ok := run.selectedFrame()
	if !ok || !run.paused || run.tree != nil && run.tree.FrameID == frame.ID || run.loading && run.inspecting == frame.ID {
		return
	}
	run.tree, run.inspecting, run.loading = nil, frame.ID, true
	run.session.Inspect(frame.ID)
}

// applyInspection records a variables answer or failure for the frame last requested.
func (run *debugRun) applyInspection(event debug.Event) {
	if !run.loading || event.FrameID != run.inspecting {
		return
	}
	run.tree, run.loading = debug.NewTree(event.FrameID, event.Scopes), false
}

func (state *shellState) handleStackKey(screen tcell.Screen, event *tcell.EventKey) {
	run := state.debugger
	switch event.Key() {
	case tcell.KeyLeft, tcell.KeyRight:
		state.bottomMode = bottomOutput
		return
	case tcell.KeyEnter:
		state.selectFrame()
		return
	}
	if !run.active() {
		return
	}
	rows := state.outputRows(screen)
	run.stackCursor = moveSelection(event.Key(), run.stackCursor, len(run.frames), rows)
	run.stackScroll = scrollTo(run.stackScroll, run.stackCursor, rows)
}

// selectFrame makes the frame under the cursor the one the views show and jumps to its line.
func (state *shellState) selectFrame() {
	run := state.debugger
	if !run.active() || !run.paused || run.stackCursor >= len(run.frames) {
		return
	}
	run.frame = run.stackCursor
	frame := run.frames[run.frame]
	path := frame.Source.Path
	if path == "" || (state.document == nil || state.document.Path != path) && !exists(path) {
		state.message = i18n.Format("msg_no_source_for_s", frame.Name)
		return
	}
	state.openAt(path, frame.Line, frame.Column)
}

func (state *shellState) handleVariablesKey(screen tcell.Screen, event *tcell.EventKey) {
	run := state.debugger
	if !run.active() || run.tree == nil {
		return
	}
	run.variable = min(run.variable, max(0, len(run.tree.Rows())-1))
	switch event.Key() {
	case tcell.KeyRight:
		state.expandVariable()
	case tcell.KeyLeft:
		run.variable = run.tree.Collapse(run.variable)
	case tcell.KeyEnter:
		if rows := run.tree.Rows(); run.variable < len(rows) && rows[run.variable].Expanded {
			run.tree.Collapse(run.variable)
		} else {
			state.expandVariable()
		}
	default:
		rows := state.outputRows(screen)
		run.variable = moveSelection(event.Key(), run.variable, len(run.tree.Rows()), rows)
		run.varScroll = scrollTo(run.varScroll, run.variable, rows)
	}
}

func (state *shellState) expandVariable() {
	run, ok := state.pausedRun()
	if !ok {
		return
	}
	if reference, load := run.tree.Expand(run.variable); load {
		run.session.Expand(reference)
	}
}

// moveSelection applies a list movement key to selected among count rows, rows of which fit on screen.
func moveSelection(key tcell.Key, selected, count, rows int) int {
	switch key {
	case tcell.KeyUp:
		selected--
	case tcell.KeyDown:
		selected++
	case tcell.KeyPgUp:
		selected -= rows
	case tcell.KeyPgDn:
		selected += rows
	case tcell.KeyHome:
		selected = 0
	case tcell.KeyEnd:
		selected = count - 1
	}
	return max(0, min(selected, count-1))
}

// scrollTo moves scroll the least that shows selected among rows lines.
func scrollTo(scroll, selected, rows int) int {
	scroll = min(scroll, selected)
	if selected >= scroll+rows {
		scroll = selected - rows + 1
	}
	return max(0, scroll)
}

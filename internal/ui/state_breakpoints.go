package ui

import (
	"path/filepath"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// toggleBreakpoint adds or removes a breakpoint on the caret's line of a Go file.
func (state *shellState) toggleBreakpoint() {
	path, line, ok := state.caretInGoFile()
	if !ok {
		return
	}
	label := i18n.Format("msg_s_d", filepath.Base(path), line)
	if state.breakpointSet().Toggle(path, line) {
		state.message = i18n.Format("msg_breakpoint_set_at_s", label)
	} else {
		state.message = i18n.Format("msg_breakpoint_removed_from_s", label)
	}
	state.sendBreakpoints(path)
}

// caretInGoFile returns the current Go file and the caret's one-based line, and otherwise explains that one is needed.
func (state *shellState) caretInGoFile() (string, int, bool) {
	if state.document == nil || state.buffer == nil || filepath.Ext(state.document.Path) != ".go" {
		state.message = i18n.Text("msg_open_a_go_file_to_set_breakpoints")
		return "", 0, false
	}
	return state.document.Path, state.buffer.Cursor().Line + 1, true
}

// trackBreakpoints moves the current file's breakpoints with the edits made since the last call.
func (state *shellState) trackBreakpoints() {
	if state.document == nil || state.buffer == nil {
		state.trackedPath, state.trackedText = "", ""
		return
	}
	path, text := state.document.Path, state.buffer.Text()
	if path == state.trackedPath {
		state.followEdit(path, state.trackedText, text)
	}
	state.trackedPath, state.trackedText = path, text
}

// followEdit moves a file's breakpoints with one change to its text, which need not be the current file's.
func (state *shellState) followEdit(path, before, after string) {
	if state.breakpointSet().Track(path, before, after) {
		state.sendBreakpoints(path)
	}
	if path == state.trackedPath {
		state.trackedText = after
	}
}

// sendBreakpoints replaces a file's breakpoints in the running program, keeping a pending run-to-cursor stop.
func (state *shellState) sendBreakpoints(path string) {
	run := state.debugger
	if !run.active() {
		return
	}
	request := state.breakpointSet().Request(path)
	if run.runToLine > 0 && request.Path == run.runToPath {
		request = state.breakpointSet().RunTo(path, run.runToLine)
	}
	run.session.SetBreakpoints(request)
}

// breakpointMarker draws a breakpoint in the gutter; one the running program refused is gray.
func (state *shellState) breakpointMarker(line int) (rune, bool, bool) {
	if state.document == nil || state.breakpoints == nil {
		return 0, false, false
	}
	breakpoint, ok := state.breakpoints.At(state.document.Path, line+1)
	if !ok {
		return 0, false, false
	}
	if breakpoint.Disabled {
		return '○', false, true
	}
	return '●', state.debugger.active() && !breakpoint.Verified, true
}

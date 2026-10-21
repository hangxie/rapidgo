package ui

import (
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/i18n"
)

// requestStep steps the paused thread over, into, or out of the current line.
func (state *shellState) requestStep(command debug.Command) {
	run, ok := state.pausedRun()
	if !ok {
		return
	}
	run.resume()
	switch command {
	case debug.Next:
		run.session.Next(run.threadID)
	case debug.StepIn:
		run.session.StepIn(run.threadID)
	case debug.StepOut:
		run.session.StepOut(run.threadID)
	}
	state.message = i18n.Format("msg_stepping_s", run.output.command)
}

// runToCursor continues the paused program until it reaches the caret's line.
func (state *shellState) runToCursor() {
	run, ok := state.pausedRun()
	if !ok {
		return
	}
	path, line, ok := state.caretInGoFile()
	if !ok {
		return
	}
	run.resume()
	run.runToPath, run.runToLine = path, line
	run.session.RunTo(state.breakpointSet().RunTo(run.runToPath, run.runToLine), run.threadID)
	state.message = i18n.Format("msg_continuing_s", run.output.command)
}

// pausedRun returns the session when its program is paused, and otherwise explains why it is not.
func (state *shellState) pausedRun() (*debugRun, bool) {
	run := state.debugger
	switch {
	case !run.active():
		state.message = i18n.Text("msg_no_debug_session_is_running")
	case !run.paused:
		state.message = i18n.Text("msg_wait_until_the_program_is_paused")
	default:
		return run, true
	}
	return nil, false
}

// executionLine is the one-based line the paused program stopped on, when it is in path.
func (state *shellState) executionLine(path string) (int, bool) {
	run := state.debugger
	if !run.active() || !run.paused || run.stopLine == 0 || run.stopPath != path {
		return 0, false
	}
	return run.stopLine, true
}

// resume hides the execution line but keeps it, since a command Delve refuses leaves the program where it was.
func (run *debugRun) resume() {
	run.paused = false
}

// moved forgets where the program stopped once it has run on.
func (run *debugRun) moved() {
	run.paused = false
	run.stopPath, run.stopLine = "", 0
	run.forgetStack()
}

// restoreRunTo drops the temporary run-to-cursor stop from its file.
func (state *shellState) restoreRunTo() {
	run := state.debugger
	if run.runToLine == 0 {
		return
	}
	run.runToLine = 0
	state.sendBreakpoints(run.runToPath)
}

// applyProblem reports a failed command and undoes what the UI assumed when sending it.
func (state *shellState) applyProblem(event debug.Event) {
	run := state.debugger
	switch event.Command {
	case debug.Locate:
	case debug.Inspect:
		run.applyInspection(event)
	case debug.Expand:
		if run.tree != nil {
			run.tree.Fail(event.Reference)
		}
	default:
		// The program stays paused when Delve refuses to resume it, so the user can try again.
		if !run.exited {
			run.paused, run.threadID = true, event.ThreadID
			state.restoreRunTo()
		}
	}
	state.message = i18n.Format("msg_debugger_s", event.Err.Error())
}

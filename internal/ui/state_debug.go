package ui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/hangxie/rapidgo/internal/dap"
	"github.com/hangxie/rapidgo/internal/debug"
	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// debugRun is the UI side of the current or most recent debug session.
type debugRun struct {
	session  *debug.Session // nil once the session has ended
	output   *jobView
	paused   bool
	threadID int
	exited   bool
	exitCode int
	stopping bool
	warned   bool   // a breakpoint failure is on the message line
	testDir  string // the package a debugged test runs in, which its output names files relative to
}

func (run *debugRun) active() bool { return run != nil && run.session != nil }

func (state *shellState) breakpointSet() *debug.Breakpoints {
	if state.breakpoints == nil {
		state.breakpoints = debug.NewBreakpoints()
	}
	return state.breakpoints
}

// debugEvents is nil without a running session, so the run loop's select ignores it.
func (state *shellState) debugEvents() <-chan debug.Event {
	if !state.debugger.active() {
		return nil
	}
	return state.debugger.session.Events()
}

// requestDebug continues a paused program, or debugs the test under the caret or else the package Run would use.
func (state *shellState) requestDebug() {
	if run := state.debugger; run.active() {
		if run.paused {
			run.paused = false
			run.session.Continue(run.threadID)
			state.message = i18n.Format("msg_continuing_s", run.output.command)
			return
		}
		state.message = i18n.Format("msg_debugging_s_ctrl_f2_stops_it", run.output.command)
		return
	}
	if state.startDebugger == nil {
		state.message = i18n.Text("msg_debugging_is_not_available_in_this_session")
		return
	}
	if state.document != nil && state.buffer != nil {
		if test, ok := debug.FindTest(state.document.Path, state.buffer.Text(), state.buffer.Cursor().Line+1); ok {
			state.debugTest(test)
			return
		}
	}
	state.withPackages(runIntentDebug)
}

func (state *shellState) debugTest(test debug.Test) {
	args := test.Args()
	target, ok := relativeTarget(state.projectRoot, test.Dir)
	if !ok {
		target = test.Dir
	}
	label := strings.Join(append([]string{"dlv", "test", target}, args...), " ")
	state.launchDebug(debug.Plan{Dir: state.projectRoot, Config: dap.LaunchConfig{Mode: dap.ModeTest, Program: target, Cwd: test.Dir, Args: args}}, label)
}

// debugPackage debugs a main package named the way Run names it, with the session's run arguments.
func (state *shellState) debugPackage(target string) {
	// Delve builds from the project root, where Run's relative targets resolve even in GOPATH mode.
	config := dap.LaunchConfig{Mode: dap.ModeDebug, Program: target, Cwd: state.projectRoot, Args: append([]string(nil), state.runArguments...)}
	state.launchDebug(debug.Plan{Dir: state.projectRoot, Config: config}, "dlv debug "+target)
}

func (state *shellState) launchDebug(plan debug.Plan, label string) {
	// A package listing or chooser can finish after another session started.
	if run := state.debugger; run.active() {
		state.message = i18n.Format("msg_debugging_s_ctrl_f2_stops_it", run.output.command)
		return
	}
	set := state.breakpointSet()
	for _, path := range set.Paths() {
		plan.Breakpoints = append(plan.Breakpoints, set.Request(path))
	}
	run := &debugRun{output: &jobView{command: label, state: jobs.Pending, follow: true, parser: diagnostic.New(diagnostic.Program)}}
	if plan.Config.Mode == dap.ModeTest {
		// A test binary writes only test output, which has no package header to unlock the parser.
		run.testDir = plan.Config.Cwd
		run.output.parser = diagnostic.New(diagnostic.Tool)
	}
	run.session = debug.Launch(context.Background(), state.startDebugger, plan)
	state.debugger = run
	state.showingDebug = true
	state.bottomMode = bottomOutput
	state.message = i18n.Format("msg_starting_s", label)
}

func (state *shellState) applyDebugEvent(event debug.Event) {
	run := state.debugger
	if !run.active() {
		return
	}
	switch event.Kind {
	case debug.Running:
		run.output.state = jobs.Running
		if !run.warned {
			state.message = i18n.Format("msg_debugging_s_ctrl_f2_stops_it", run.output.command)
		}
	case debug.Output:
		run.appendOutput(event)
	case debug.Stopped:
		run.paused, run.threadID = true, event.ThreadID
		reason := event.Text
		if reason == "" {
			reason = event.Reason
		}
		state.message = i18n.Format("msg_paused_s_shift_f9_continues_ctrl_f2_stops", reason)
	case debug.Continued:
		run.paused = false
	case debug.Exited:
		run.exited, run.exitCode = true, event.ExitCode
	case debug.Resolved:
		state.breakpointSet().Resolve(event.Request, event.Results)
		if event.Err != nil {
			run.warned = true
			state.message = i18n.Format("msg_set_breakpoints_in_s_failed_s", event.Request.Path, event.Err.Error())
		}
	case debug.Problem:
		// The program stays paused when Delve refuses to resume it, so Shift+F9 can try again.
		if event.Command == debug.Continue && !run.exited {
			run.paused, run.threadID = true, event.ThreadID
		}
		state.message = i18n.Format("msg_debugger_s", event.Err.Error())
	}
}

// appendOutput keeps Delve's console messages apart and places test locations in the test's package.
func (run *debugRun) appendOutput(event debug.Event) {
	var stream jobs.Stream
	switch event.Category {
	case "stdout":
		stream = jobs.Stdout
	case "stderr":
		stream = jobs.Stderr
	default:
		// DAP treats a missing or unknown category as console.
		run.output.keep(outputLine{text: event.Text, console: true})
		return
	}
	run.output.append(jobs.Event{Line: event.Text, Stream: stream})
	problem := run.output.lines[len(run.output.lines)-1].problem
	if run.testDir != "" && problem != nil && problem.Source == diagnostic.SourceTest && !filepath.IsAbs(problem.Path) {
		problem.Path = filepath.Join(run.testDir, filepath.FromSlash(problem.Path))
	}
}

// drainDebugEvents applies what the session has queued, so a chatty program redraws once per burst.
func drainDebugEvents(state *shellState) {
	for {
		select {
		case event, ok := <-state.debugEvents():
			if !ok {
				state.finishDebug()
				return
			}
			state.applyDebugEvent(event)
		default:
			return
		}
	}
}

// finishDebug records how the session ended once its events close.
func (state *shellState) finishDebug() {
	run := state.debugger
	if !run.active() {
		return
	}
	err := run.session.Err()
	run.session, run.paused = nil, false
	state.breakpointSet().ClearVerification()
	label := run.output.command
	switch {
	case err != nil:
		// Delve reports build failures in its error, so keep every line where Enter can jump to it.
		for _, line := range strings.Split(err.Error(), "\n") {
			run.output.append(jobs.Event{Line: line, Stream: jobs.Stderr})
		}
		run.output.state = jobs.Failed
		first, _, _ := strings.Cut(err.Error(), "\n")
		state.message = i18n.Format("msg_s_failed_s", label, first)
	case run.stopping:
		run.output.state = jobs.Cancelled
		state.message = i18n.Format("msg_s_cancelled", label)
	case run.exited:
		run.output.state = jobs.Succeeded
		if run.exitCode != 0 {
			run.output.state = jobs.Failed
		}
		state.message = i18n.Format("msg_s_exited_with_code_d", label, run.exitCode)
	default:
		run.output.state = jobs.Succeeded
		state.message = i18n.Format("msg_s_ended", label)
	}
}

// stopDebug ends the session; its events report when the program is gone.
func (state *shellState) stopDebug() {
	if !state.endDebug() {
		state.message = i18n.Text("msg_no_debug_session_is_running")
		return
	}
	state.message = i18n.Format("msg_stopping_s", state.debugger.output.command)
}

func (state *shellState) endDebug() bool {
	run := state.debugger
	if !run.active() {
		return false
	}
	run.stopping = true
	run.session.Stop()
	return true
}

// closeDebug stops the session and waits for it, so quitting leaves no dlv behind.
func (state *shellState) closeDebug() {
	if !state.endDebug() {
		return
	}
	for range state.debugger.session.Events() {
	}
	state.finishDebug()
}

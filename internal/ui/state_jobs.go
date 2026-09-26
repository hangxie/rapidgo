package ui

import (
	"fmt"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// maxOutputLines bounds the output kept for one job.
const maxOutputLines = 2000

// jobRunner is the part of the manager the shell uses, for testing without processes.
type jobRunner interface {
	Start(jobs.Request) uint64
	Discover()
	CancelAll() int
}

type outputLine struct {
	text    string
	stream  jobs.Stream
	problem *diagnostic.Diagnostic // nil when the line is plain output
}

// jobView records one run of a job kind, identified so stale events are dropped.
type jobView struct {
	id      uint64
	kind    jobs.Kind
	command string
	state   jobs.State
	lines   []outputLine
	dropped int
	scroll  int  // first visible line when not following the tail
	follow  bool // keep the newest output in view

	parser   diagnostic.Parser
	selected int // index into lines
	// verdict is where the current package's test output starts.
	verdict int
}

func (view *jobView) append(event jobs.Event) {
	line := outputLine{text: event.Line, stream: event.Stream}
	if reported, ok := view.parser.Line(event.Line); ok {
		line.problem = &reported
	}
	view.lines = append(view.lines, line)
	if view.follow {
		view.selected = len(view.lines) - 1
	}
	if named := view.parser.TakeVerdict(); named != "" {
		view.nameTestPackage(named)
	}
	if len(view.lines) > maxOutputLines {
		removed := len(view.lines) - maxOutputLines
		view.lines = append(view.lines[:0], view.lines[removed:]...)
		view.dropped += removed
		// Keep the same text, selection, and boundary in view.
		view.scroll = max(0, view.scroll-removed)
		view.selected = max(0, view.selected-removed)
		view.verdict = max(0, view.verdict-removed)
	}
}

// nameTestPackage fills in the package for the test output above a verdict line.
func (view *jobView) nameTestPackage(named string) {
	for index := view.verdict; index < len(view.lines); index++ {
		if problem := view.lines[index].problem; problem != nil && problem.Source == diagnostic.SourceTest && problem.Package == "" {
			problem.Package = named
		}
	}
	view.verdict = len(view.lines)
}

// diagnostics returns the problems the pane holds, in output order.
func (view *jobView) diagnostics() []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	for _, line := range view.lines {
		if line.problem != nil {
			found = append(found, *line.problem)
		}
	}
	return found
}

// title describes the run for the output pane frame.
func (view *jobView) title() string {
	title := "OUTPUT  " + view.command + " (" + view.state.String() + ")"
	if view.dropped > 0 {
		title += fmt.Sprintf(" %d earlier lines dropped", view.dropped)
	}
	return title
}

// position reports the visible range when the output does not all fit.
func (view *jobView) position(rows int) string {
	total := view.dropped + len(view.lines)
	if len(view.lines) <= rows {
		return ""
	}
	first := view.dropped + view.top(rows) + 1
	last := min(total, first+rows-1)
	return fmt.Sprintf("  %d-%d/%d", first, last, total)
}

// summary is the message shown when a run reaches a terminal state.
func (view *jobView) summary(err error) string {
	switch {
	case view.state == jobs.Succeeded:
		return view.command + " succeeded"
	case view.state == jobs.Cancelled:
		return view.command + " cancelled"
	case err != nil:
		return view.command + " failed: " + err.Error()
	default:
		return view.command + " failed"
	}
}

func (state *shellState) activeView() *jobView {
	if !state.jobStarted {
		return nil
	}
	return state.views[state.visibleJob]
}

// startJob runs a Go command; Run first resolves which package.
func (state *shellState) startJob(kind jobs.Kind) {
	if kind == jobs.Run {
		state.requestRun()
		return
	}
	state.startRequest(jobs.Request{Kind: kind})
}

// startRequest launches one command, replacing an earlier run of its kind.
func (state *shellState) startRequest(request jobs.Request) {
	if state.jobs == nil {
		state.message = "Go commands are not available in this session"
		return
	}
	id := state.jobs.Start(request)
	if id == 0 {
		state.message = "Could not start " + request.Command()
		return
	}
	if state.views == nil {
		state.views = make(map[jobs.Kind]*jobView)
	}
	state.views[request.Kind] = &jobView{
		id: id, kind: request.Kind, command: request.Command(),
		state: jobs.Pending, follow: true, parser: diagnostic.New(originOf(request.Kind)),
	}
	state.visibleJob = request.Kind
	state.jobStarted = true
	state.message = "Starting " + request.Command()
}

// stopJob cancels every running command, since only one kind is on screen.
func (state *shellState) stopJob() {
	if state.jobs == nil {
		state.message = "Go commands are not available in this session"
		return
	}
	switch running := state.jobs.CancelAll(); running {
	case 0:
		state.message = "No Go command is running"
	case 1:
		state.message = "Stopping the running Go command"
	default:
		state.message = fmt.Sprintf("Stopping %d running Go commands", running)
	}
}

// originOf says whether a kind's output can contain the program's own writing.
func originOf(kind jobs.Kind) diagnostic.Origin {
	if kind == jobs.Run {
		return diagnostic.Program
	}
	return diagnostic.Tool
}

func (state *shellState) applyJobEvent(event jobs.Event) {
	if event.Type == jobs.Discovered {
		state.applyDiscovery(event)
		return
	}
	if event.Type == jobs.Detected {
		state.toolchain = event.Tool
		state.toolchainErr = event.Err
		if event.Err != nil {
			state.message = "Go toolchain: " + event.Err.Error()
		}
		return
	}
	view := state.views[event.Kind]
	if view == nil || view.id != event.ID {
		return // A superseded or cancelled run must not replace a newer one.
	}
	switch event.Type {
	case jobs.Started:
		view.state = jobs.Running
		if event.Command != "" {
			view.command = event.Command
		}
		state.message = "Running " + view.command + " (Ctrl+K stops it)"
	case jobs.Output:
		view.append(event)
	case jobs.Finished:
		view.state = event.State
		state.message = view.summary(event.Err)
	}
}

// toolchainStatus describes the detected Go executable for the help surface.
func (state *shellState) toolchainStatus() string {
	switch {
	case state.toolchainErr != nil:
		return state.toolchainErr.Error()
	case state.toolchain.Path == "":
		return "detecting..."
	default:
		return state.toolchain.Describe()
	}
}

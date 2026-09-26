package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
	"github.com/hangxie/rapidgo/internal/project"
)

type fakeRunner struct {
	started    []jobs.Request
	next       uint64
	reject     bool
	cancelled  int
	remaining  int
	discovered int
	packages   []jobs.Package
	listErr    error
	state      *shellState // when set, Discover answers immediately
}

func (r *fakeRunner) Start(request jobs.Request) uint64 {
	if r.reject {
		return 0
	}
	r.started = append(r.started, request)
	r.next++
	return r.next
}

func (r *fakeRunner) Discover() {
	r.discovered++
	if r.state != nil {
		r.state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: r.packages, Err: r.listErr})
	}
}

// kinds lists the kinds started so far, for assertions that ignore targets.
func (r *fakeRunner) kinds() []jobs.Kind {
	var started []jobs.Kind
	for _, request := range r.started {
		started = append(started, request.Kind)
	}
	return started
}

func (r *fakeRunner) CancelAll() int {
	r.cancelled++
	running := r.remaining
	r.remaining = 0
	return running
}

func newJobState(runner jobRunner) *shellState {
	return &shellState{projectRoot: "/tmp/project", jobs: runner}
}

func TestStartJobTracksOneRunPerKind(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := newJobState(runner)
	state.startJob(jobs.Build)
	require.Equal(t, []jobs.Kind{jobs.Build}, runner.kinds())
	assert.Equal(t, "Starting go build ./...", state.message)

	view := state.activeView()
	require.NotNil(t, view)
	assert.Equal(t, jobs.Pending, view.state)
	assert.Equal(t, "OUTPUT  go build ./... (pending)", view.title())

	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Started, Command: "go build ./..."})
	assert.Equal(t, jobs.Running, view.state)
	assert.Contains(t, state.message, "Running go build ./...")

	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "./main.go:4:2: undefined: x", Stream: jobs.Stderr})
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed, Err: errors.New("exit status 1")})
	assert.Equal(t, []outputLine{{text: "./main.go:4:2: undefined: x", stream: jobs.Stderr}}, view.lines)
	assert.Equal(t, "go build ./... failed: exit status 1", state.message)
	assert.Equal(t, "OUTPUT  go build ./... (failed)", view.title())

	// Starting a different kind shows the new run without losing the old one.
	state.startJob(jobs.Test)
	assert.Equal(t, jobs.Test, state.activeView().kind)
	assert.Len(t, state.views, 2)
	assert.Equal(t, jobs.Failed, state.views[jobs.Build].state)
}

func TestStaleJobEventsNeverReplaceNewerRun(t *testing.T) {
	t.Parallel()

	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	stale := state.activeView().id

	state.startJob(jobs.Build)
	current := state.activeView()
	require.NotEqual(t, stale, current.id)

	// The replaced run finishes late; neither its output nor its result may
	// reach the newer run's view.
	state.applyJobEvent(jobs.Event{ID: stale, Kind: jobs.Build, Type: jobs.Output, Line: "stale output"})
	state.applyJobEvent(jobs.Event{ID: stale, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Cancelled})
	assert.Empty(t, current.lines)
	assert.Equal(t, jobs.Pending, current.state)
	assert.Equal(t, "Starting go build ./...", state.message)

	state.applyJobEvent(jobs.Event{ID: current.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Succeeded})
	assert.Equal(t, "go build ./... succeeded", state.message)

	// An event for a kind that was never started is ignored.
	state.applyJobEvent(jobs.Event{ID: 99, Kind: jobs.Run, Type: jobs.Output, Line: "unknown"})
	assert.Len(t, state.views, 1)
}

func TestJobSummaries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		state jobs.State
		err   error
		want  string
	}{
		{jobs.Succeeded, nil, "go test ./... succeeded"},
		{jobs.Cancelled, nil, "go test ./... cancelled"},
		{jobs.Failed, errors.New("exit status 2"), "go test ./... failed: exit status 2"},
		{jobs.Failed, nil, "go test ./... failed"},
	} {
		view := &jobView{command: jobs.Request{Kind: jobs.Test}.Command(), state: test.state}
		assert.Equal(t, test.want, view.summary(test.err))
	}
}

func TestStopJob(t *testing.T) {
	t.Parallel()

	missing := &shellState{}
	missing.startJob(jobs.Build)
	assert.Equal(t, "Go commands are not available in this session", missing.message)
	missing.stopJob()
	assert.Equal(t, "Go commands are not available in this session", missing.message)

	rejecting := newJobState(&fakeRunner{reject: true})
	rejecting.startJob(jobs.Build)
	assert.Equal(t, "Could not start go build ./...", rejecting.message)
	assert.Nil(t, rejecting.activeView())

	idle := &fakeRunner{}
	state := newJobState(idle)
	state.stopJob()
	assert.Equal(t, 1, idle.cancelled)
	assert.Equal(t, "No Go command is running", state.message)

	single := newJobState(&fakeRunner{remaining: 1})
	single.startJob(jobs.Test)
	single.stopJob()
	assert.Equal(t, "Stopping the running Go command", single.message)
}

// Ctrl+K must reach a job of a kind the output pane is not showing, because
// nothing on screen would otherwise reveal that it is still running.
func TestStopJobCancelsHiddenRuns(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{remaining: 2}
	state := newJobState(runner)
	state.startJob(jobs.Test)
	state.startJob(jobs.Build)
	require.Equal(t, jobs.Build, state.activeView().kind, "the newest kind is the visible one")

	state.stopJob()
	assert.Equal(t, 1, runner.cancelled, "Ctrl+K cancels every kind, not just the visible one")
	assert.Equal(t, "Stopping 2 running Go commands", state.message)
}

func TestToolchainStatus(t *testing.T) {
	t.Parallel()

	state := newJobState(&fakeRunner{})
	assert.Equal(t, "detecting...", state.toolchainStatus())

	state.applyJobEvent(jobs.Event{Type: jobs.Detected, Tool: jobs.Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"}})
	assert.Equal(t, "go1.26.0 (/usr/bin/go)", state.toolchainStatus())
	assert.Empty(t, state.message)

	state.applyJobEvent(jobs.Event{Type: jobs.Detected, Err: errors.New("locate go executable: not found")})
	assert.Equal(t, "locate go executable: not found", state.toolchainStatus())
	assert.Equal(t, "Go toolchain: locate go executable: not found", state.message)
}

func TestJobViewDropsOldestOutput(t *testing.T) {
	t.Parallel()

	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Test)
	view := state.activeView()
	for index := range maxOutputLines + 10 {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Test, Type: jobs.Output, Line: fmt.Sprintf("line %d", index)})
	}
	require.Len(t, view.lines, maxOutputLines)
	assert.Equal(t, "line 10", view.lines[0].text)
	assert.Equal(t, fmt.Sprintf("line %d", maxOutputLines+9), view.lines[maxOutputLines-1].text)
	assert.Contains(t, view.title(), "10 earlier lines dropped")
}

func TestJobShortcutsAndMenuStartJobs(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	runner := &fakeRunner{remaining: 3, packages: []jobs.Package{{Name: "main", Dir: "/tmp/project/cmd/tool", ImportPath: "example/cmd/tool"}}}
	state := newJobState(runner)
	runner.state = state

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF9, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyCtrlT, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF9, 0, tcell.ModCtrl)))
	assert.Equal(t, []jobs.Kind{jobs.Build, jobs.Test, jobs.Run}, runner.kinds())
	assert.Equal(t, "./cmd/tool", runner.started[2].Target, "Ctrl+F9 runs the module's only main package")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyCtrlK, 0, 0)))
	assert.Equal(t, 1, runner.cancelled)
	assert.Equal(t, "Stopping 3 running Go commands", state.message)

	// Alt+B opens the Build menu, and Enter runs the selected action.
	menu := newJobState(&fakeRunner{})
	assert.False(t, handleEvent(screen, menu, tcell.NewEventKey(tcell.KeyRune, 'b', tcell.ModAlt)))
	require.True(t, menu.menuOpen)
	assert.Equal(t, menuBuild, menu.menuIndex)
	for range 4 {
		assert.False(t, handleEvent(screen, menu, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	}
	assert.Equal(t, 4, menu.menuItem, "Stop is the last Build menu action")
	assert.False(t, handleEvent(screen, menu, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.False(t, menu.menuOpen)
	assert.Equal(t, "No Go command is running", menu.message)

	for item, kind := range buildMenuKinds {
		fresh := newJobState(&fakeRunner{})
		fresh.jobs.(*fakeRunner).state = fresh
		fresh.jobs.(*fakeRunner).packages = []jobs.Package{{Name: "main", Dir: "/tmp/project", ImportPath: "example/cmd"}}
		fresh.runMenuAction(item)
		assert.Equal(t, kind, fresh.activeView().kind)
	}
}

func TestRenderOutputPane(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)

	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	view := state.activeView()
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Started, Command: "go build ./..."})
	for index := range 8 {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: fmt.Sprintf("line %d", index), Stream: jobs.Stderr})
	}
	render(screen, *state)

	assert.Contains(t, rowText(screen, 18, 0, 60), "OUTPUT  go build ./... (running)")
	// The pane follows the tail of the output; three rows fit at this size.
	assert.Contains(t, rowText(screen, 19, 0, 20), "line 5")
	assert.Contains(t, rowText(screen, 21, 0, 20), "line 7")
	assertCellColors(t, screen, 1, 21, turboLightRed, turboBlue)

	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "done", Stream: jobs.Stdout})
	render(screen, *state)
	assert.Contains(t, rowText(screen, 21, 0, 20), "done")
	assertCellColors(t, screen, 1, 21, turboYellow, turboBlue)
}

func rowText(screen tcell.Screen, y, from, to int) string {
	var row strings.Builder
	for x := from; x < to; x++ {
		value, _, _ := screen.Get(x, y)
		row.WriteString(value)
	}
	return row.String()
}

func TestRunLoopStreamsGoBuildFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module rapidgotest\n\ngo 1.26\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {\n\tmissingFunction()\n}\n"), 0o600))

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 24)
	interrupts := make(chan os.Signal, 1)
	t.Cleanup(func() { interrupts <- os.Interrupt })
	finished := make(chan error, 1)
	go func() {
		finished <- runLoopWithServices(screen, root, interrupts, project.DiskSource{}, project.DiskSaver{})
	}()

	require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyF9, 0, 0)))
	require.Eventually(t, func() bool {
		return strings.Contains(paneText(screen, 18, 22), "missingFunction")
	}, 90*time.Second, 50*time.Millisecond, "the output pane should show the compiler diagnostic")
	assert.Contains(t, paneText(screen, 18, 22), "go build ./... (failed)")

	require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyCtrlQ, 0, 0)))
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("UI did not quit after the build finished")
	}
}

func paneText(screen tcell.Screen, top, bottom int) string {
	width, _ := screen.Size()
	var pane strings.Builder
	for y := top; y <= bottom; y++ {
		pane.WriteString(rowText(screen, y, 0, width))
		pane.WriteString("\n")
	}
	return pane.String()
}

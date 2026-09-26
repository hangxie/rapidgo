package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// outputState returns a shell whose build view already holds count lines.
func outputState(t *testing.T, screen tcell.Screen, count int) (*shellState, *jobView) {
	t.Helper()
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	view := state.activeView()
	require.NotNil(t, view)
	for index := range count {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: fmt.Sprintf("line %d", index)})
	}
	state.setFocus(focusOutput)
	return state, view
}

func TestOutputPaneScrolls(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, view := outputState(t, screen, 100)
	rows := state.outputRows(screen)
	require.Equal(t, 8, rows, "the focused pane takes half the work area")

	// A fresh view follows the tail.
	assert.True(t, view.follow)
	assert.Equal(t, "line 92", view.visibleLines(rows)[0].text)
	assert.Equal(t, "line 99", view.visibleLines(rows)[rows-1].text)

	key := func(code tcell.Key) { state.handleOutputKey(screen, tcell.NewEventKey(code, 0, 0)) }
	key(tcell.KeyPgUp)
	assert.False(t, view.follow, "scrolling away stops following")
	assert.Equal(t, "line 84", view.visibleLines(rows)[0].text)
	key(tcell.KeyUp)
	assert.Equal(t, "line 83", view.visibleLines(rows)[0].text)
	key(tcell.KeyDown)
	assert.Equal(t, "line 84", view.visibleLines(rows)[0].text)
	key(tcell.KeyHome)
	assert.Equal(t, "line 0", view.visibleLines(rows)[0].text)
	assert.False(t, view.follow)

	// Scrolling up from the top clamps rather than going negative.
	key(tcell.KeyPgUp)
	assert.Equal(t, "line 0", view.visibleLines(rows)[0].text)

	key(tcell.KeyEnd)
	assert.True(t, view.follow, "End resumes following the newest output")
	assert.Equal(t, "line 99", view.visibleLines(rows)[rows-1].text)

	// Paging down to the last line resumes following on its own, so a running
	// command keeps tailing.
	key(tcell.KeyHome)
	for range 20 {
		key(tcell.KeyPgDn)
	}
	assert.True(t, view.follow)
}

func TestOutputFollowKeepsNewLinesVisible(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, view := outputState(t, screen, 20)
	rows := state.outputRows(screen)

	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyHome, 0, 0))
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "newest"})
	assert.Equal(t, "line 0", view.visibleLines(rows)[0].text, "a scrolled view stays put")

	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnd, 0, 0))
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "newer still"})
	assert.Equal(t, "newer still", view.visibleLines(rows)[rows-1].text)
}

func TestOutputPositionIndicator(t *testing.T) {
	t.Parallel()

	view := &jobView{command: "go test ./...", state: jobs.Running, follow: true}
	for index := range 5 {
		view.append(jobs.Event{Line: fmt.Sprintf("line %d", index)})
	}
	assert.Empty(t, view.position(8), "no indicator while everything fits")
	assert.Equal(t, "  3-5/5", view.position(3))
	view.scrollTo(0, 3)
	assert.Equal(t, "  1-3/5", view.position(3))
	// Lines dropped from the front still count toward the total.
	view.dropped = 10
	assert.Equal(t, "  11-13/15", view.position(3))
}

func TestOutputSwitchesBetweenKinds(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	state.startJob(jobs.Test)
	state.setFocus(focusOutput)
	require.Equal(t, jobs.Test, state.visibleJob)

	key := func(code tcell.Key) { state.handleOutputKey(screen, tcell.NewEventKey(code, 0, 0)) }
	key(tcell.KeyLeft)
	assert.Equal(t, jobs.Build, state.visibleJob, "Left reaches the earlier run")
	assert.Equal(t, "Showing go build ./...", state.message)
	key(tcell.KeyLeft)
	assert.Equal(t, jobs.Test, state.visibleJob, "the list wraps")
	key(tcell.KeyRight)
	assert.Equal(t, jobs.Build, state.visibleJob)

	// With a single view there is nothing to switch to.
	only := newJobState(&fakeRunner{})
	only.startJob(jobs.Build)
	only.setFocus(focusOutput)
	only.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyRight, 0, 0))
	assert.Equal(t, jobs.Build, only.visibleJob)
}

func TestOutputKeysDoNothingWithoutAView(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := newJobState(&fakeRunner{})
	state.setFocus(focusOutput)
	for _, code := range []tcell.Key{tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd, tcell.KeyLeft, tcell.KeyRight} {
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(code, 0, 0)))
	}
	assert.Nil(t, state.activeView())
}

func TestFocusCycleSkipsEditorWithoutADocument(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{}
	state.focusNextPane(screen)
	assert.Equal(t, focusOutput, state.focus, "no file open, so the editor is skipped")
	assert.Equal(t, focusTree, state.mainFocus, "the output pane remembers where it came from")
	state.focusNextPane(screen)
	assert.Equal(t, focusTree, state.focus)
}

// A terminal too short to render the output pane must not focus it.
func TestFocusCycleSkipsAnUnrenderableOutputPane(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 3)
	require.Zero(t, calculateLayout(40, 3, false).output.height)

	state := &shellState{}
	setTestDocument(t, state, "/tmp/work/main.go", "package main")
	state.setFocus(focusTree)
	state.focusNextPane(screen)
	assert.Equal(t, focusEditor, state.focus)
	state.focusNextPane(screen)
	assert.Equal(t, focusTree, state.focus, "the cycle skips the invisible output pane")

	// Without a document there is nowhere else to go, so focus stays put.
	bare := &shellState{}
	bare.focusNextPane(screen)
	assert.Equal(t, focusTree, bare.focus)
}

// A narrow terminal keeps showing the work pane the output was reached from.
func TestNarrowTerminalKeepsTheMainPaneBehindOutput(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 16)
	state := newShellState("/tmp/work", func(workRequest) bool { return true })
	setTestDocument(t, &state, "/tmp/work/main.go", "package main")
	state.setFocus(focusEditor)
	state.setFocus(focusOutput)
	render(screen, state)
	assert.Contains(t, rowText(screen, 1, 0, 40), "EDITOR", "the editor stays visible behind the output pane")

	state.setFocus(focusTree)
	state.setFocus(focusOutput)
	render(screen, state)
	assert.Contains(t, rowText(screen, 1, 0, 40), "PROJECT")
	assert.True(t, state.treeAccessible(screen), "the tree stays navigable from the output pane")
}

func TestMessageLine(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)

	// A job owns the output pane, so its messages need their own row.
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	state.message = "Saved /tmp/project/main.go"
	render(screen, *state)
	assert.Contains(t, rowText(screen, 22, 0, 80), "Saved /tmp/project/main.go")
	assert.Contains(t, rowText(screen, 17, 0, 60), "go build ./...", "the output pane still shows the job")
	assertCellColors(t, screen, 1, 22, turboWhite, turboBlue)

	state.message = ""
	render(screen, *state)
	assert.Contains(t, rowText(screen, 22, 0, 20), "Ready")

	// The row is the first thing a short terminal gives up.
	for _, size := range [][2]int{{80, 4}, {80, 5}, {20, 24}, {1, 1}} {
		screen.SetSize(size[0], size[1])
		render(screen, *state)
		view := calculateLayout(size[0], size[1], false)
		assert.Equal(t, size[1] >= 5, view.message.height == 1, "size %v", size)
	}
}

func TestFocusedOutputPaneGrows(t *testing.T) {
	t.Parallel()

	unfocused := calculateLayout(80, 24, false)
	focused := calculateLayout(80, 24, true)
	assert.Equal(t, 5, unfocused.output.height)
	assert.Equal(t, 10, focused.output.height)
	assert.Greater(t, unfocused.editor.height, focused.editor.height)
	for _, view := range []layout{unfocused, focused} {
		assert.Equal(t, view.editor.y+view.editor.height, view.output.y)
		assert.Equal(t, 22, view.message.y)
		assert.Equal(t, view.output.y+view.output.height, view.message.y)
	}
}

func TestOutputPaneRendersScrollPosition(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _ := outputState(t, screen, 100)
	render(screen, *state)
	title := rowText(screen, 12, 0, 80)
	assert.Contains(t, title, "93-100/100")
	assert.Contains(t, title, "╔", "the focused pane has a double border")

	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyHome, 0, 0))
	render(screen, *state)
	assert.Contains(t, rowText(screen, 12, 0, 80), "1-8/100")
	assert.Contains(t, rowText(screen, 13, 0, 20), "line 0")
}

func TestEmptyOutputPaneHint(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project"})
	assert.Contains(t, strings.Join([]string{rowText(screen, 17, 0, 80), rowText(screen, 18, 0, 80)}, " "), "No output yet")
}

// Growing the pane onto the last line must resume following, not just clamp.
func TestResizingBackToTheBottomResumesFollowing(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, view := outputState(t, screen, 100)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyPgUp, 0, 0))
	require.False(t, view.follow)
	require.Equal(t, 84, view.scroll)

	// A pane tall enough for every line puts the viewport back at the end.
	screen.SetSize(80, 220)
	state.keepOutputAnchored(screen)
	assert.True(t, view.follow, "the viewport reached the last line")
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "after resize"})
	lines := view.visibleLines(state.outputRows(screen))
	assert.Equal(t, "after resize", lines[len(lines)-1].text)

	// A pane that grows but not enough keeps the user where they were.
	screen.SetSize(80, 24)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyHome, 0, 0))
	require.False(t, view.follow)
	screen.SetSize(80, 40)
	state.keepOutputAnchored(screen)
	assert.False(t, view.follow)
	assert.Equal(t, 0, view.scroll)
}

// A focus change resizes the pane too, but cannot strand a view.
func TestFocusChangeKeepsTheViewportConsistent(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, view := outputState(t, screen, 100)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyPgUp, 0, 0))
	require.False(t, view.follow)
	require.Equal(t, 84, view.scroll)

	// Leaving the pane shrinks it to a quarter of the work area.
	state.setFocus(focusTree)
	state.keepOutputAnchored(screen)
	assert.Equal(t, 84, view.scroll)
	assert.False(t, view.follow)

	// Returning grows it again and leaves the user where they were.
	state.setFocus(focusOutput)
	state.keepOutputAnchored(screen)
	assert.Equal(t, 84, view.scroll)
	assert.False(t, view.follow)
	assert.Equal(t, "line 84", view.visibleLines(state.outputRows(screen))[0].text)
}

// A resize event through the event loop must anchor the view, not just clamp it.
func TestResizeEventResumesFollowing(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, view := outputState(t, screen, 100)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyPgUp, 0, 0))
	require.False(t, view.follow)

	screen.SetSize(80, 220)
	assert.False(t, handleEvent(screen, state, tcell.NewEventResize(80, 220)))
	state.keepOutputAnchored(screen)
	assert.True(t, view.follow)
}

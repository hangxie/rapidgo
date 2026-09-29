package ui

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
)

func TestHelpScrollsInShortTerminal(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(56, 12)
	state := &shellState{helpVisible: true}
	readScreen := func() string {
		render(screen, *state)
		var content strings.Builder
		width, height := screen.Size()
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				value, _, _ := screen.Get(x, y)
				content.WriteString(value)
			}
			content.WriteByte('\n')
		}
		return content.String()
	}
	assert.Contains(t, readScreen(), "Shortcut")
	assert.NotContains(t, readScreen(), "Unsaved D / Esc")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnd, 0, 0)))
	assert.Contains(t, readScreen(), "Unsaved D / Esc")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyHome, 0, 0)))
	assert.Contains(t, readScreen(), "Shortcut")
	screen.SetSize(30, 12)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnd, 0, 0)))
	assert.Contains(t, readScreen(), "Unsaved D / Esc")
}

func TestHelpClosesWhenTooSmallToRender(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, size := range [][2]int{{15, 10}, {30, 4}} {
		screen.SetSize(80, 24)
		state := newJobState(&fakeRunner{})
		state.focus = focusTree
		state.helpVisible = true
		screen.SetSize(size[0], size[1])
		assert.False(t, handleEvent(screen, state, tcell.NewEventResize(size[0], size[1])))
		assert.False(t, state.helpVisible)
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF1, 0, 0)))
		assert.False(t, state.helpVisible)
		assert.Contains(t, state.message, "16x5")
		state.startJob(jobs.Build)
		view := state.activeView()
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "first"})
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "second"})
		state.focus = focusOutput
		view.selected = 0
		state.helpVisible = true // Also cover input before a resize event arrives.
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
		assert.False(t, state.helpVisible)
		assert.Equal(t, 1, view.selected)
	}
}

func TestEnvironmentHelpShowsFullPaths(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newJobState(&fakeRunner{})
	state.projectRoot = "/tmp/project"
	state.toolchain = jobs.Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModAlt)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.helpEnvironment)
	assert.True(t, state.helpVisible)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "RapidGo environment")
	rows := helpRows(*state, 52, 80, 24)
	var contents strings.Builder
	for _, row := range rows {
		for _, part := range row {
			contents.WriteString(part.text)
		}
		contents.WriteByte('\n')
	}
	assert.Contains(t, contents.String(), "/tmp/project")
	assert.Contains(t, contents.String(), "/usr/bin/go")
	assert.Contains(t, contents.String(), "go1.26.0")
	assert.Contains(t, contents.String(), "80 x 24")
}

func TestRunLoopQuits(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyCtrlQ, 0, 0)))
	require.NoError(t, runLoop(screen, "/tmp/project", nil))
}

func TestRunLoopSignalWithFullEventQueue(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)

	queueFull := false
	for i := 0; i < 100; i++ {
		err := screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'x', 0))
		if err == tcell.ErrEventQFull {
			queueFull = true
			break
		}
		require.NoError(t, err)
	}
	require.True(t, queueFull, "simulation event queue should be full")

	interrupts := make(chan os.Signal, 1)
	interrupts <- syscall.SIGTERM
	finished := make(chan error, 1)
	go func() { finished <- runLoop(screen, "/tmp/project", interrupts) }()
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("signal did not stop the UI loop with a full event queue")
	}
}

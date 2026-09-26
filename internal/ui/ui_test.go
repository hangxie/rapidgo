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

	"github.com/hangxie/rapidgo/internal/project"
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
	assert.Contains(t, readScreen(), "Tree: arrows")
	assert.NotContains(t, readScreen(), "Unsaved prompt")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnd, 0, 0)))
	assert.Contains(t, readScreen(), "Unsaved prompt")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyHome, 0, 0)))
	assert.Contains(t, readScreen(), "Tree: arrows")
	screen.SetSize(30, 12)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnd, 0, 0)))
	assert.Contains(t, readScreen(), "Go:")
}

func TestHandleEvent(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{projectRoot: "/tmp/project"}

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF1, 0, 0)))
	assert.True(t, state.helpVisible)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.False(t, state.helpVisible)
	assert.False(t, handleEvent(screen, state, tcell.NewEventResize(40, 12)))
	assert.True(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyCtrlQ, 0, 0)))
	assert.True(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyCtrlC, 0, 0)))
	assert.True(t, handleEvent(screen, state, tcell.NewEventInterrupt(nil)))
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

func TestMenuNavigation(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{}
	key := func(code tcell.Key) bool {
		return handleEvent(screen, state, tcell.NewEventKey(code, 0, 0))
	}

	assert.False(t, key(tcell.KeyF10))
	assert.True(t, state.menuOpen)
	assert.Equal(t, menuFile, state.menuIndex)
	assert.False(t, key(tcell.KeyRight))
	assert.Equal(t, menuSearch, state.menuIndex)
	assert.False(t, key(tcell.KeyRight))
	assert.Equal(t, menuBuild, state.menuIndex)
	assert.False(t, key(tcell.KeyRight))
	assert.Equal(t, menuHelp, state.menuIndex)
	assert.False(t, key(tcell.KeyEnter))
	assert.False(t, state.menuOpen)
	assert.True(t, state.helpVisible)

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModAlt)))
	assert.True(t, state.menuOpen)
	assert.False(t, state.helpVisible)
	assert.Equal(t, menuFile, state.menuIndex)
	assert.False(t, key(tcell.KeyEscape))
	assert.False(t, state.menuOpen)
	assert.False(t, key(tcell.KeyF10))
	assert.False(t, key(tcell.KeyDown))
	assert.True(t, key(tcell.KeyEnter), "File > Quit should exit")
}

func TestFileAndSearchMenuActions(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/project", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	setTestDocument(t, &state, "/tmp/project/main.go", "package main\n")
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModAlt)))
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	require.Len(t, requests, 2)
	assert.Equal(t, saveFile, requests[1].kind)
	state.applySaveResult(workResult{request: requests[1], saved: project.SaveResult{Content: "package main\n"}})
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt)))
	assert.Equal(t, menuSearch, state.menuIndex)
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.searching)
}

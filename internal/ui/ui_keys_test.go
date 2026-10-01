package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/project"
)

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

func TestHelpClosesOnlyWithEscape(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{helpVisible: true, helpEnvironment: true}
	for _, event := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyF1, 0, 0),
		tcell.NewEventKey(tcell.KeyF10, 0, 0),
		tcell.NewEventKey(tcell.KeyF3, 0, 0),
		tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModAlt),
	} {
		assert.False(t, handleEvent(screen, state, event))
		assert.True(t, state.helpVisible)
		assert.True(t, state.helpEnvironment)
		assert.False(t, state.menuOpen)
	}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.False(t, state.helpVisible)
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
	assert.Equal(t, menuWindow, state.menuIndex)
	assert.False(t, key(tcell.KeyRight))
	assert.Equal(t, menuHelp, state.menuIndex)
	assert.False(t, key(tcell.KeyEnter))
	assert.False(t, state.menuOpen)
	assert.True(t, state.helpVisible)

	assert.False(t, key(tcell.KeyEscape))
	assert.False(t, state.helpVisible)
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

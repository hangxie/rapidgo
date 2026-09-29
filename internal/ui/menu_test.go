package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMenuOpensArgumentPrompt(t *testing.T) {
	t.Parallel()

	state := shellState{menuOpen: true, helpVisible: true, runArgumentText: `"two words"`}
	state.runMenuAction(buildMenuSetup)
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	state.runSetupAction(1)
	assert.True(t, state.editingRunArgs)
	assert.False(t, state.runSetupOpen)
	assert.False(t, state.menuOpen)
	assert.False(t, state.helpVisible)
	assert.Equal(t, `"two words"`, state.runArgumentDraft)
}

func TestRunSetupMenuActions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Run Current File", menuActions[menuBuild][buildMenuCurrent].label)
	assert.Equal(t, "Run Setup", menuActions[menuBuild][buildMenuSetup].label)
	assert.Equal(t, ">", menuActions[menuBuild][buildMenuSetup].shortcut)
	assert.Equal(t, "Run in Terminal", menuActions[menuBuild][buildMenuTerminal].label)
	assert.Len(t, menuActions[menuBuild], 7)
	assert.Equal(t, []menuAction{{"Set Default Package", ""}, {"Run Arguments", ""}}, runSetupActions)
}

func TestRunSetupKeyboardNavigation(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{}
	state.runMenuAction(buildMenuSetup)
	assert.True(t, state.runSetupOpen)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.Equal(t, 1, state.runSetupItem)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.editingRunArgs)
	assert.False(t, state.runSetupOpen)

	state.editingRunArgs = false
	state.runMenuAction(buildMenuSetup)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyLeft, 0, 0)))
	assert.False(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.Equal(t, buildMenuSetup, state.menuItem)
}

func TestBuildMenuOpensRunSetupWithKeyboard(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.False(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.Equal(t, menuBuild, state.menuIndex)
	assert.Equal(t, buildMenuSetup, state.menuItem)
	assert.Empty(t, state.message)
}

func TestBuildMenuRightOpensRunSetup(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyRight, 0, 0)))
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
}

func TestRunSetupPreservesBackgroundMessageOnReturn(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, key := range []tcell.Key{tcell.KeyEscape, tcell.KeyLeft} {
		state := &shellState{}
		state.runMenuAction(buildMenuSetup)
		state.message = "Build finished"
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(key, 0, 0)))
		assert.Equal(t, "Build finished", state.message)
		assert.False(t, state.runSetupOpen)
		assert.True(t, state.menuOpen)
	}
}

func TestRunSetupGlobalShortcuts(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, key := range []tcell.Key{tcell.KeyCtrlQ, tcell.KeyCtrlC} {
		state := &shellState{}
		state.runMenuAction(buildMenuSetup)
		assert.True(t, handleEvent(screen, state, tcell.NewEventKey(key, 0, 0)))
	}
	state := &shellState{}
	state.runMenuAction(buildMenuSetup)
	state.message = "Build finished"
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF1, 0, 0)))
	assert.True(t, state.helpVisible)
	assert.False(t, state.runSetupOpen)
	assert.False(t, state.menuOpen)
	assert.Equal(t, "Build finished", state.message)
}

func TestBuildMenuPreparesTerminalRun(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/tool"))
	state.runMenuAction(buildMenuTerminal)
	require.NotNil(t, state.terminalRun)
	assert.Equal(t, "./cmd/tool", state.terminalRun.Target)
	assert.Empty(t, runner.started, "the command waits for terminal handoff")
}

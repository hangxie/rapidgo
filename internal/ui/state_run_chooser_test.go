package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
)

func TestTerminalRunChooserKeepsTerminalMode(t *testing.T) {
	t.Parallel()
	state, runner := runState(t, mainPackage("cmd/one"), mainPackage("cmd/two"))
	state.requestTerminalRun()
	require.NotNil(t, state.chooser)
	assert.True(t, state.chooser.terminal)
	state.handleChooserKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	require.NotNil(t, state.terminalRun)
	assert.Equal(t, "./cmd/one", state.terminalRun.Target)
	assert.Empty(t, runner.started)
}

func TestRunChooserRemembersTheChoice(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)
	assert.Empty(t, runner.started, "nothing runs until a package is chosen")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.Equal(t, 1, state.chooser.index)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.Equal(t, 1, state.chooser.index, "Up from the first entry wraps to the last")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Nil(t, state.chooser)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/worker", runner.started[0].Target)

	// The remembered target skips the chooser next time.
	state.startJob(jobs.Run)
	assert.Nil(t, state.chooser)
	require.Len(t, runner.started, 2)
	assert.Equal(t, "./cmd/worker", runner.started[1].Target)
}

func TestRunChooserCancels(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.Nil(t, state.chooser)
	assert.Empty(t, runner.started)
	assert.Equal(t, "Cancelled; no package was run", state.message)
	assert.Empty(t, state.runTarget, "cancelling must not remember a target")
}

func TestRunTargetCanBeChangedFromTheMenu(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	// Default Package... asks even though a target is already remembered, and it
	// starts on the one in effect.
	openRunDefaultFromMenu(state)
	require.NotNil(t, state.chooser)
	assert.Equal(t, 1, state.chooser.index)
	assert.False(t, state.chooser.run, "choosing a target must not launch it")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Equal(t, "./cmd/server", state.runTarget)
	assert.Equal(t, "Default run package: ./cmd/server", state.message)
	assert.Empty(t, runner.started, "setting the target does not run anything")

	// The new default is what Ctrl+F9 then runs.
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/server", runner.started[0].Target)
}

func TestRunTargetMenuCancelKeepsTheCurrentTarget(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	openRunDefaultFromMenu(state)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.Equal(t, "./cmd/worker", state.runTarget)
	assert.Equal(t, "Cancelled; the default run package is unchanged", state.message)
}

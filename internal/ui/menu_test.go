package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMenuOpensArgumentPrompt(t *testing.T) {
	t.Parallel()

	state := shellState{menuOpen: true, helpVisible: true, runArgumentText: `"two words"`}
	state.runMenuAction(buildMenuArguments)
	assert.True(t, state.editingRunArgs)
	assert.False(t, state.menuOpen)
	assert.False(t, state.helpVisible)
	assert.Equal(t, `"two words"`, state.runArgumentDraft)
}

func TestBuildMenuPreparesTerminalRun(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/tool"))
	state.runMenuAction(buildMenuTerminal)
	require.NotNil(t, state.terminalRun)
	assert.Equal(t, "./cmd/tool", state.terminalRun.Target)
	assert.Empty(t, runner.started, "the command waits for terminal handoff")
}

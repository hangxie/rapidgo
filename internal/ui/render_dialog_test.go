package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorDirtyTitleAndDiscardDialog(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "package main")
	require.NoError(t, state.buffer.Insert("x"))
	state.confirm = confirmQuit
	render(screen, state)
	var title strings.Builder
	for x := 23; x < 49; x++ {
		value, _, _ := screen.Get(x, 1)
		title.WriteString(value)
	}
	assert.Contains(t, title.String(), "main.go *")
	var prompt strings.Builder
	for x := 7; x < 73; x++ {
		value, _, _ := screen.Get(x, 11)
		prompt.WriteString(value)
	}
	assert.Contains(t, prompt.String(), "Discard edits and quit RapidGo?")
	_, _, visible := screen.GetCursor()
	assert.False(t, visible)
}

func TestCloseWindowDiscardDialog(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(80, 24)
	renderConfirmation(screen, 80, 24, shellState{confirm: confirmCloseWindow})
	var prompt strings.Builder
	for x := 7; x < 73; x++ {
		value, _, _ := screen.Get(x, 11)
		prompt.WriteString(value)
	}
	assert.Contains(t, prompt.String(), "Discard edits and close this window?")
}

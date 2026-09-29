package ui

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestRenderCompletionOnNormalAndNarrowScreens(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := shellState{
		focus: focusEditor, mainFocus: focusEditor, syntax: &syntaxCache{}, completionVisible: true,
		completionItems: []gopls.CompletionItem{{Label: "Println", Detail: "func(...any)"}, {Label: "Printf", Detail: "func(string, ...any)"}}, completionSelected: 1,
	}
	setTestDocument(t, &state, filepath.Join(t.TempDir(), "main.go"), "package main\n")
	screen.SetSize(80, 24)
	render(screen, state)
	assert.Contains(t, paneText(screen, 0, 23), "gopls completion")
	assert.Contains(t, paneText(screen, 0, 23), "Printf")
	screen.SetSize(24, 7)
	render(screen, state)
	assert.Contains(t, paneText(screen, 0, 6), "gopls completion")
}

func TestCompletionPickerClosesWhenTerminalCannotDisplayIt(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(20, 5)
	state := &shellState{completionVisible: true}
	state.ensureCompletionFits(screen)
	assert.False(t, state.completionVisible)
	assert.Contains(t, state.message, "larger terminal")
	renderCompletion(screen, 20, 5, shellState{completionItems: []gopls.CompletionItem{{Label: "hidden"}}})
	assert.NotContains(t, paneText(screen, 0, 4), "hidden")
}

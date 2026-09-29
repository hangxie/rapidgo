package ui

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestRenderLocationsUsesSharedPaneAndScroll(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 12)
	root := t.TempDir()
	items := make([]gopls.Location, 8)
	for index := range items {
		items[index] = gopls.Location{Path: filepath.Join(root, "main.go"), Position: gopls.Position{Line: index, Character: index}}
	}
	state := shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomLocations, locationKind: navigationReferences, locations: items, locationSelected: 7, locationScroll: 7}
	render(screen, state)
	view := paneText(screen, 0, 11)
	assert.Contains(t, view, "REFERENCES")
	assert.Contains(t, view, "main.go:8")
	assert.NotContains(t, view, "main.go:8:8", "columns are unknown for files outside the editor")
	assert.NotContains(t, view, "main.go:1:1")
	screen.SetSize(8, 4)
	assert.NotPanics(t, func() { render(screen, state) })
}

func TestRenderLocationsShowsEditorGraphemeColumn(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomLocations, locationKind: navigationReferences}
	setTestDocument(t, &state, path, "var 𐐀 = foo\n")
	state.locations = []gopls.Location{
		{Path: path, Position: gopls.Position{Character: 9}},
		{Path: path, Position: gopls.Position{Line: 5, Character: 9}},
	}
	render(screen, state)
	view := paneText(screen, 0, 23)
	assert.Contains(t, view, "main.go:1:9")
	assert.NotContains(t, view, "main.go:1:10")
	assert.Contains(t, view, "main.go:6")
	assert.NotContains(t, view, "main.go:6:10")
}

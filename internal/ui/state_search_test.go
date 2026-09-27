package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
)

func TestSearchInputHandlesGraphemesAndModifiers(t *testing.T) {
	t.Parallel()

	state := shellState{menuOpen: true, helpVisible: true, searchInput: "old"}
	state.startSearch()
	assert.Equal(t, "Open a file before searching", state.message)

	setTestDocument(t, &state, "main.go", "e\u0301 e\u0301")
	state.startSearch()
	assert.True(t, state.searching)
	assert.False(t, state.menuOpen)
	assert.False(t, state.helpVisible)
	assert.Empty(t, state.searchInput)

	for _, value := range []rune{'e', '\u0301'} {
		state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyRune, value, 0))
	}
	state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModCtrl))
	state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyRune, '\n', 0))
	assert.Equal(t, "e\u0301", state.searchInput)
	state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyBackspace2, 0, 0))
	assert.Empty(t, state.searchInput, "backspace removes the whole grapheme")
	state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.False(t, state.searching)
	assert.Equal(t, "Search text is empty", state.message)

	state.startSearch()
	state.handleSearchKey(nil, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	assert.False(t, state.searching)
	assert.Equal(t, "Search cancelled", state.message)
}

func TestSearchFindsNextAndWraps(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := shellState{focus: focusTree}
	setTestDocument(t, &state, "main.go", "界 x 界")
	state.startSearch()
	state.handleSearchKey(screen, tcell.NewEventKey(tcell.KeyRune, '界', 0))
	state.handleSearchKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, "Found: 界", state.message)
	assert.Equal(t, focusEditor, state.focus)
	start, end, selected := state.buffer.Selection()
	assert.True(t, selected)
	assert.Equal(t, editor.Position{}, start)
	assert.Equal(t, editor.Position{Column: 1}, end)

	state.findNext(screen)
	assert.Equal(t, "Found: 界", state.message)
	start, _, _ = state.buffer.Selection()
	assert.Equal(t, editor.Position{Column: 4}, start)
	state.findNext(screen)
	assert.Equal(t, "Found: 界 (wrapped)", state.message)
	start, _, _ = state.buffer.Selection()
	assert.Equal(t, editor.Position{}, start)

	state.searchQuery = "missing"
	state.findNext(screen)
	assert.Equal(t, "Not found: missing", state.message)
	state.searchQuery = ""
	state.findNext(screen)
	assert.Equal(t, "Use Ctrl+F to enter search text", state.message)
}

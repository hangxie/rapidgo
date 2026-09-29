package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrawTextDoesNotSplitWideCharacter(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(10, 2)
	drawText(screen, 0, 0, 1, "界X", baseStyle)
	value, _, _ := screen.Get(0, 0)
	assert.NotEqual(t, "界", value)
	drawText(screen, 0, 1, 2, "界X", baseStyle)
	value, _, width := screen.Get(0, 1)
	assert.Equal(t, "界", value)
	assert.Equal(t, 2, width)
	value, _, _ = screen.Get(2, 1)
	assert.NotEqual(t, "X", value)
}

func TestDrawStyledTextClipsWithoutSplittingWideCharacter(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(4, 1)
	drawStyledText(screen, 0, 0, 3, []textSegment{{"A", barStyle}, {"界B", shortcutStyle}, {"C", barStyle}})
	value, _, _ := screen.Get(0, 0)
	assert.Equal(t, "A", value)
	value, _, _ = screen.Get(1, 0)
	assert.Equal(t, "界", value)
	value, _, _ = screen.Get(3, 0)
	assert.NotEqual(t, "B", value)
	assert.NotEqual(t, "C", value)
}

func TestOnlyFocusedPaneHasDoubleBorder(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newShellState("/tmp/work", func(workRequest) bool { return true })
	setTestDocument(t, &state, "/tmp/work/main.go", "package main")
	render(screen, state)
	assertCellRune(t, screen, 0, 1, "╔")  // Focused tree.
	assertCellRune(t, screen, 21, 1, "┌") // Inactive editor.
	assertCellRune(t, screen, 0, 17, "┌") // Inactive output.

	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyTab, 0, 0)))
	assert.Equal(t, focusTree, state.focus)
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, 0)))
	render(screen, state)
	assertCellRune(t, screen, 0, 1, "┌")
	assertCellRune(t, screen, 21, 1, "╔")
	assertCellRune(t, screen, 0, 17, "┌")

	// F6 again reaches the output pane, which then takes a larger share.
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, 0)))
	assert.Equal(t, focusOutput, state.focus)
	render(screen, state)
	assertCellRune(t, screen, 21, 1, "┌") // Editor is inactive again.
	assertCellRune(t, screen, 0, 12, "╔") // Focused output, grown to half.
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, 0)))
	assert.Equal(t, focusTree, state.focus, "F6 cycles back to the tree")

	screen.SetSize(35, 12)
	render(screen, state)
	assertCellRune(t, screen, 0, 1, "╔") // Narrow terminal shows focused editor.
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF3, 0, 0)))
	render(screen, state)
	assertCellRune(t, screen, 0, 1, "╔") // F3 reveals focused tree.
	assert.True(t, state.treeAccessible(screen))
}

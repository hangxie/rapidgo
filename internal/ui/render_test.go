package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

func TestCalculateLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		width, height int
		wantProject   bool
	}{
		{width: 0, height: 0},
		{width: 1, height: 1},
		{width: 10, height: 2},
		{width: 30, height: 6},
		{width: 59, height: 24},
		{width: 60, height: 24, wantProject: true},
		{width: 80, height: 24, wantProject: true},
		{width: 200, height: 24, wantProject: true},
	}

	for _, test := range tests {
		view := calculateLayout(test.width, test.height, false)
		assert.Equal(t, test.wantProject, view.projectVisible)
		for _, pane := range []rectangle{view.menu, view.status, view.message, view.project, view.editor, view.output} {
			assert.GreaterOrEqual(t, pane.x, 0)
			assert.GreaterOrEqual(t, pane.y, 0)
			assert.LessOrEqual(t, pane.x+pane.width, test.width)
			assert.LessOrEqual(t, pane.y+pane.height, test.height)
		}
		if view.projectVisible {
			assert.Less(t, view.project.x+view.project.width, view.editor.x)
			if test.width == 200 {
				assert.Equal(t, 28, view.project.width)
			}
		}
		if test.height >= 3 {
			assert.Equal(t, 1, view.editor.y)
		}
		assert.LessOrEqual(t, view.editor.y+view.editor.height, view.output.y)
	}
}

func TestLongRunArgumentPromptShowsTail(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(30, 6)
	state := shellState{editingRunArgs: true, runArgumentDraft: strings.Repeat("界", 20) + "tail"}
	render(screen, state)
	status := rowText(screen, 5, 0, 30)
	assert.Contains(t, status, "Run args:")
	assert.Contains(t, status, "tail")
	assert.NotContains(t, status, strings.Repeat("界", 8))
	_, y, visible := screen.GetCursor()
	assert.True(t, visible)
	assert.Equal(t, 5, y)

	screen.SetSize(1, 5)
	render(screen, state)
}

func TestRenderAtNarrowSizes(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, size := range [][2]int{{1, 1}, {12, 3}, {30, 6}, {80, 24}} {
		screen.SetSize(size[0], size[1])
		render(screen, shellState{projectRoot: "/tmp/界-project", helpVisible: true})
		width, height := screen.Size()
		assert.Equal(t, size[0], width)
		assert.Equal(t, size[1], height)
	}
}

func TestEditorDisplaysTabsAsFourSpaces(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 5)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "a\tb")
	renderDocument(screen, rectangle{width: 40, height: 5}, state)

	var line strings.Builder
	for x := 6; x < 12; x++ {
		value, _, _ := screen.Get(x, 1)
		line.WriteString(value)
	}
	assert.Equal(t, "a    b", line.String())
	assert.Equal(t, "a\tb", state.buffer.Text(), "rendering must not change document text")
}

func TestGoSyntaxColorsAndSelection(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(50, 8)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "package main\n// 界 note\nvar s = \"hi\"\nvar n = 42\n")
	area := rectangle{width: 50, height: 8}
	renderDocument(screen, area, state)
	assertCellColors(t, screen, 6, 1, turboWhite, turboBlue)     // Keyword.
	assertCellColors(t, screen, 14, 1, turboYellow, turboBlue)   // Identifier.
	assertCellColors(t, screen, 6, 2, turboLightGray, turboBlue) // Comment.
	assertCellColors(t, screen, 14, 3, turboYellow, turboBlue)
	assertCellColors(t, screen, 14, 4, turboLightCyan, turboBlue) // Number.

	require.NoError(t, state.buffer.Select(editor.Position{}, editor.Position{Line: 0, Column: 7}))
	renderDocument(screen, area, state)
	assertCellColors(t, screen, 6, 1, turboBlack, turboLightCyan) // Selection wins.
}

func TestHighlightingRawStringAcrossLinesAndUnicodeCursor(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(50, 7)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/main.go", "var s = `one\n界 two`\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 1}, false))
	state.fileScroll = 1
	renderDocument(screen, rectangle{width: 50, height: 7}, state)
	assertCellColors(t, screen, 6, 1, turboYellow, turboBlue)
	x, y, visible := screen.GetCursor()
	assert.True(t, visible)
	assert.Equal(t, 8, x) // Wide 界 occupies two terminal cells.
	assert.Equal(t, 1, y)
}

func TestNonGoFileKeepsBaseTextColor(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 5)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/notes.txt", "package main")
	renderDocument(screen, rectangle{width: 40, height: 5}, state)
	assertCellColors(t, screen, 6, 1, turboYellow, turboBlue)
}

func TestEditorGutterFitsLargeLineNumbers(t *testing.T) {
	t.Parallel()
	for _, lineCount := range []int{10_000, 100_000} {
		t.Run(fmt.Sprint(lineCount), func(t *testing.T) {
			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(40, 8)
			state := shellState{focus: focusEditor}
			setTestDocument(t, &state, "/tmp/large.go", strings.Repeat("x\n", lineCount-1)+"x")
			state.fileScroll = lineCount - 1
			require.NoError(t, state.buffer.MoveTo(editor.Position{Line: lineCount - 1}, false))
			area := rectangle{width: 40, height: 8}
			gutter := editorGutterWidth(area, lineCount)
			renderDocument(screen, area, state)
			assertCellRune(t, screen, gutter, 1, " ")
			assertCellRune(t, screen, gutter+1, 1, "x")
			x, y, visible := screen.GetCursor()
			assert.True(t, visible)
			assert.Equal(t, gutter+1, x)
			assert.Equal(t, 1, y)
			var number strings.Builder
			for x := 1; x < gutter; x++ {
				value, _, _ := screen.Get(x, 1)
				number.WriteString(value)
			}
			assert.Equal(t, fmt.Sprint(lineCount), strings.TrimSpace(number.String()))
		})
	}
}

func TestEditorHidesGutterWhenLargeLineNumbersCrowdNarrowPane(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(12, 5)
	state := shellState{focus: focusEditor}
	setTestDocument(t, &state, "/tmp/large.go", strings.Repeat("x\n", 99_999)+"x")
	state.fileScroll = 99_999
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 99_999}, false))
	area := rectangle{width: 12, height: 5}
	assert.Zero(t, editorGutterWidth(area, 100_000))
	renderDocument(screen, area, state)
	assertCellRune(t, screen, 1, 1, "x")
	x, y, visible := screen.GetCursor()
	assert.True(t, visible)
	assert.Equal(t, 1, x)
	assert.Equal(t, 1, y)
}

func TestNarrowStatusKeepsShortcutsWithLongProjectName(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(35, 8)
	render(screen, shellState{projectRoot: "/tmp/" + strings.Repeat("long-name-", 8)})

	var status strings.Builder
	for x := 0; x < 35; x++ {
		value, _, _ := screen.Get(x, 7)
		status.WriteString(value)
	}
	assert.Contains(t, status.String(), "F1 Help")
	assert.Contains(t, status.String(), "^Q Quit")
	assert.Contains(t, status.String(), "F10 Menu")
	assert.Contains(t, status.String(), "F3 Tree")
}

func TestThemeUsesVGAColorsWithoutBoldAttribute(t *testing.T) {
	t.Parallel()

	for _, style := range []tcell.Style{titleStyle, shortcutStyle, menuActiveStyle, menuActiveMnemonicStyle} {
		_, _, attributes := style.Decompose()
		assert.Zero(t, attributes&tcell.AttrBold)
	}
}

func TestLargeTreeRendersOnlyVisibleRows(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newShellState("/tmp/work", func(workRequest) bool { return true })
	entries := make([]project.Entry, 5000)
	for index := range entries {
		entries[index] = project.Entry{Name: fmt.Sprintf("file-%04d.go", index)}
	}
	state.tree.Apply(state.tree.Root, entries, nil)
	state.selected = 5000
	state.keepSelectionVisible(screen)
	assert.Positive(t, state.treeScroll)
	render(screen, state)
	var row strings.Builder
	for x := 1; x < 19; x++ {
		value, _, _ := screen.Get(x, 15)
		row.WriteString(value)
	}
	assert.Contains(t, row.String(), "file-4999.go")
	assertCellColors(t, screen, 1, 15, turboBlack, turboGreen)
}

func assertCellRune(t *testing.T, screen tcell.Screen, x, y int, want string) {
	t.Helper()
	value, _, _ := screen.Get(x, y)
	assert.Equal(t, want, value)
}

func assertCellColors(t *testing.T, screen tcell.Screen, x, y int, wantForeground, wantBackground tcell.Color) {
	t.Helper()
	_, style, _ := screen.Get(x, y)
	foreground, background, _ := style.Decompose()
	assert.Equal(t, wantForeground, foreground)
	assert.Equal(t, wantBackground, background)
}

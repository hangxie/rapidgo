package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/jobs"
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
		}
		if test.height >= 3 {
			assert.Equal(t, 1, view.editor.y)
		}
		assert.LessOrEqual(t, view.editor.y+view.editor.height, view.output.y)
	}
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
	assert.Contains(t, title.String(), "EDITOR *")
	var prompt strings.Builder
	for x := 7; x < 73; x++ {
		value, _, _ := screen.Get(x, 11)
		prompt.WriteString(value)
	}
	assert.Contains(t, prompt.String(), "Discard edits and quit RapidGo?")
	_, _, visible := screen.GetCursor()
	assert.False(t, visible)
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

func TestRenderMenuBarAndDropdown(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuHelp})

	var menuBar, dropdown strings.Builder
	for x := 0; x < 40; x++ {
		value, _, _ := screen.Get(x, 0)
		menuBar.WriteString(value)
		value, _, _ = screen.Get(x, 2)
		dropdown.WriteString(value)
	}
	assert.Contains(t, menuBar.String(), "File")
	assert.Contains(t, menuBar.String(), "Search")
	assert.Contains(t, menuBar.String(), "Build")
	assert.Contains(t, menuBar.String(), "Help")
	assert.Contains(t, dropdown.String(), "Shortcuts")

	assertCellColors(t, screen, 60, 10, turboYellow, turboBlue)    // Desktop.
	assertCellColors(t, screen, 2, 0, turboRed, turboLightGray)    // Alt+F mnemonic.
	assertCellColors(t, screen, 3, 0, turboBlack, turboLightGray)  // Menu item.
	assertCellColors(t, screen, 17, 0, turboRed, turboLightGray)   // Alt+B mnemonic.
	assertCellColors(t, screen, 25, 0, turboRed, turboGreen)       // Active Alt+H mnemonic.
	assertCellColors(t, screen, 24, 1, turboWhite, turboLightGray) // Dropdown border.
	assertCellColors(t, screen, 26, 2, turboBlack, turboGreen)     // Selected dropdown item.
	assertCellColors(t, screen, 37, 2, turboRed, turboGreen)       // F1 shortcut.
	assertCellColors(t, screen, 27, 4, turboBlack, turboBlack)     // Dropdown shadow.
	assertCellColors(t, screen, 1, 23, turboRed, turboLightGray)   // Status shortcut.
	assertCellColors(t, screen, 4, 23, turboBlack, turboLightGray) // Status label.
	var status strings.Builder
	for x := 0; x < 80; x++ {
		value, _, _ := screen.Get(x, 23)
		status.WriteString(value)
	}
	assert.Contains(t, status.String(), "F6 Pane")
	assert.Contains(t, status.String(), "F2 Save")
	assert.Contains(t, status.String(), "Ctrl+F Find")
	assert.Contains(t, status.String(), "Ctrl+Q Quit")

	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuFile})
	assertCellColors(t, screen, 3, 2, turboBlack, turboGreen)     // File menu item.
	assertCellColors(t, screen, 14, 2, turboRed, turboGreen)      // F2 shortcut.
	assertCellColors(t, screen, 3, 3, turboBlack, turboLightGray) // Unselected Quit.
}

func TestMenuFitsShortTerminalsWithoutCoveringStatus(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, size := range [][2]int{{12, 3}, {30, 4}, {80, 5}} {
		screen.SetSize(size[0], size[1])
		render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuHelp})
		assertCellColors(t, screen, 0, size[1]-1, turboBlack, turboLightGray)
	}
}

func TestThemeUsesVGAColorsWithoutBoldAttribute(t *testing.T) {
	t.Parallel()

	for _, style := range []tcell.Style{titleStyle, shortcutStyle, menuActiveStyle, menuActiveMnemonicStyle} {
		_, _, attributes := style.Decompose()
		assert.Zero(t, attributes&tcell.AttrBold)
	}
}

func TestRenderFramedPanesAndHelpDialog(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project"})
	assertCellColors(t, screen, 0, 1, turboLightCyan, turboBlue)  // Project frame below menu.
	assertCellColors(t, screen, 3, 1, turboWhite, turboBlue)      // Project title.
	assertCellColors(t, screen, 21, 1, turboLightCyan, turboBlue) // Editor frame.
	var editorTitle strings.Builder
	for x := 23; x < 48; x++ {
		value, _, _ := screen.Get(x, 1)
		editorTitle.WriteString(value)
	}
	assert.Contains(t, editorTitle.String(), "/tmp/project")

	render(screen, shellState{
		projectRoot: "/tmp/project", helpVisible: true,
		toolchain: jobs.Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"},
	})
	helpRow := func(y int) string {
		var row strings.Builder
		for x := 13; x < 67; x++ {
			value, _, _ := screen.Get(x, y)
			row.WriteString(value)
		}
		return row.String()
	}
	assert.Contains(t, helpRow(3), "Shortcut")
	assert.Contains(t, helpRow(3), "Action")
	assert.Contains(t, helpRow(4), "F3")
	assert.Contains(t, helpRow(5), "F6 / Ctrl+F6")
	assert.Equal(t, strings.Index(helpRow(4), "Focus tree"), strings.Index(helpRow(5), "Next pane"))
	assert.Contains(t, helpRow(21), "Up/Down/PgUp/PgDn")
	assertCellColors(t, screen, 12, 1, turboWhite, turboLightGray) // Dialog border.
	assertCellColors(t, screen, 14, 2, turboBlack, turboLightGray) // Dialog text.
	assertCellColors(t, screen, 14, 4, turboRed, turboLightGray)   // Help shortcut.

	screen.SetSize(30, 6)
	render(screen, shellState{projectRoot: "/tmp/project", helpVisible: true})
	assertCellColors(t, screen, 24, 5, turboBlack, turboLightGray) // Shadow preserves status line.
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

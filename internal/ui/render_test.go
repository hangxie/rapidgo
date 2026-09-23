package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		view := calculateLayout(test.width, test.height)
		assert.Equal(t, test.wantProject, view.projectVisible)
		for _, pane := range []rectangle{view.menu, view.status, view.project, view.editor, view.output} {
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
	assert.Contains(t, status.String(), "Ctrl+Q Quit")
	assert.Contains(t, status.String(), "F10 Menu")
}

func TestRenderMenuBarAndDropdown(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuHelp})

	var menuBar, dropdown strings.Builder
	for x := 0; x < 30; x++ {
		value, _, _ := screen.Get(x, 0)
		menuBar.WriteString(value)
		value, _, _ = screen.Get(x, 2)
		dropdown.WriteString(value)
	}
	assert.Contains(t, menuBar.String(), "File")
	assert.Contains(t, menuBar.String(), "Help")
	assert.Contains(t, dropdown.String(), "Shortcuts")

	assertCellColors(t, screen, 40, 4, turboYellow, turboBlue)     // Desktop.
	assertCellColors(t, screen, 2, 0, turboRed, turboLightGray)    // Alt+F mnemonic.
	assertCellColors(t, screen, 3, 0, turboBlack, turboLightGray)  // Menu item.
	assertCellColors(t, screen, 8, 0, turboRed, turboGreen)        // Active Alt+H mnemonic.
	assertCellColors(t, screen, 7, 1, turboWhite, turboLightGray)  // Dropdown border.
	assertCellColors(t, screen, 9, 2, turboBlack, turboGreen)      // Selected dropdown item.
	assertCellColors(t, screen, 20, 2, turboRed, turboGreen)       // F1 shortcut.
	assertCellColors(t, screen, 10, 4, turboBlack, turboBlack)     // Dropdown shadow.
	assertCellColors(t, screen, 1, 23, turboRed, turboLightGray)   // Status shortcut.
	assertCellColors(t, screen, 4, 23, turboBlack, turboLightGray) // Status label.

	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuFile})
	assertCellColors(t, screen, 3, 2, turboBlack, turboGreen) // File menu item.
	assertCellColors(t, screen, 14, 2, turboRed, turboGreen)  // Ctrl+Q shortcut.
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

	render(screen, shellState{projectRoot: "/tmp/project", helpVisible: true})
	assertCellColors(t, screen, 16, 8, turboWhite, turboLightGray) // Dialog border.
	assertCellColors(t, screen, 18, 9, turboBlack, turboLightGray) // Dialog text.
	assertCellColors(t, screen, 18, 11, turboRed, turboLightGray)  // Help shortcut.
	assertCellColors(t, screen, 18, 15, turboBlack, turboBlack)    // Dialog shadow.

	screen.SetSize(30, 6)
	render(screen, shellState{projectRoot: "/tmp/project", helpVisible: true})
	assertCellColors(t, screen, 24, 5, turboBlack, turboLightGray) // Shadow preserves status line.
}

func assertCellColors(t *testing.T, screen tcell.Screen, x, y int, wantForeground, wantBackground tcell.Color) {
	t.Helper()
	_, style, _ := screen.Get(x, y)
	foreground, background, _ := style.Decompose()
	assert.Equal(t, wantForeground, foreground)
	assert.Equal(t, wantBackground, background)
}

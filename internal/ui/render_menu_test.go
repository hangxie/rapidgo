package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, rowText(screen, 3, 24, 45), "Environment")

	assertCellColors(t, screen, 60, 10, turboYellow, turboBlue)    // Desktop.
	assertCellColors(t, screen, 2, 0, turboRed, turboLightGray)    // Alt+F mnemonic.
	assertCellColors(t, screen, 3, 0, turboBlack, turboLightGray)  // Menu item.
	assertCellColors(t, screen, 17, 0, turboRed, turboLightGray)   // Alt+B mnemonic.
	assertCellColors(t, screen, 25, 0, turboRed, turboGreen)       // Active Alt+H mnemonic.
	assertCellColors(t, screen, 24, 1, turboWhite, turboLightGray) // Dropdown border.
	assertCellColors(t, screen, 26, 2, turboBlack, turboGreen)     // Selected dropdown item.
	assertCellColors(t, screen, 38, 2, turboRed, turboGreen)       // F1 shortcut.
	assertCellColors(t, screen, 27, 5, turboBlack, turboBlack)     // Dropdown shadow.
	assertCellColors(t, screen, 1, 23, turboRed, turboLightGray)   // Status shortcut.
	assertCellColors(t, screen, 4, 23, turboBlack, turboLightGray) // Status label.
	var status strings.Builder
	for x := 0; x < 80; x++ {
		value, _, _ := screen.Get(x, 23)
		status.WriteString(value)
	}
	assert.Contains(t, status.String(), "F6 Window")
	assert.Contains(t, status.String(), "F2 Save")
	assert.Contains(t, status.String(), "Ctrl+F Find")
	assert.Contains(t, status.String(), "Ctrl+Q Quit")

	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuFile})
	assertCellColors(t, screen, 3, 2, turboBlack, turboGreen)     // File menu item.
	assertCellColors(t, screen, 14, 2, turboRed, turboGreen)      // F2 shortcut.
	assertCellColors(t, screen, 3, 3, turboBlack, turboLightGray) // Unselected Quit.

	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuBuild})
	assert.Contains(t, rowText(screen, 5, 16, 50), "Run Current File")
	assert.Contains(t, rowText(screen, 6, 16, 50), "Run Setup")
	assert.Contains(t, rowText(screen, 6, 16, 50), ">")
	assert.Contains(t, rowText(screen, 7, 16, 50), "Run in Terminal")
	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup, runSetupOpen: true, runSetupItem: 1})
	assert.Contains(t, rowText(screen, 6, 16, 45), "Run Setup")
	assert.Contains(t, rowText(screen, 7, 45, 70), "Set Default Package")
	assert.Contains(t, rowText(screen, 8, 45, 70), "Run Arguments")
}

func TestBuildMenuShortcutsShareAColumn(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project", menuOpen: true, menuIndex: menuBuild})

	var column int
	for index, shortcut := range map[int]string{2: "F9", 3: "Ctrl+T", 4: "Ctrl+F9", 5: "Alt+F9", 8: "Ctrl+K"} {
		row := rowText(screen, index, 0, 80)
		position := strings.Index(row, shortcut)
		require.NotEqual(t, -1, position, "shortcut %s is visible", shortcut)
		if column == 0 {
			column = position
		}
		assert.Equal(t, column, position, "shortcut %s is aligned", shortcut)
	}
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

func TestRunSetupRemainsVisibleOnNarrowTerminal(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(12, 5)
	render(screen, shellState{menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup, runSetupOpen: true, runSetupItem: 1})
	assert.Contains(t, rowText(screen, 1, 0, 12), "Run Argume")
}

func TestMenuWidthIncludesShortcutlessLabels(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 33, menuWidth([]menuAction{{label: "A long shortcutless menu item"}}))
}

func TestWindowMenuGroupsAndSkipsSeparators(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(80, 24)
	state := newShellState("/project", func(workRequest) bool { return true })
	state.menuOpen, state.menuIndex = true, menuWindow
	render(screen, state)
	for item, label := range []string{"New View", "─", "Tile", "Cascade", "─", "Size / Move", "Zoom", "─", "Next", "Previous", "List...", "─", "Close"} {
		require.Contains(t, rowText(screen, item+2, 31, 80), label)
	}
	for _, item := range []int{2, 3, 5, 6, 8, 9, 10, 12, 0} {
		handleKey(screen, &state, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
		require.Equal(t, item, state.menuItem)
	}
	for _, item := range []int{12, 10, 9, 8, 6, 5, 3, 2, 0} {
		handleKey(screen, &state, tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
		require.Equal(t, item, state.menuItem)
	}
	state.menuItem = windowClose
	for _, size := range [][2]int{{12, 3}, {30, 4}, {80, 5}} {
		screen.SetSize(size[0], size[1])
		render(screen, state)
		require.Contains(t, rowText(screen, 1, 0, size[0]), "Close")
		assertCellColors(t, screen, 0, size[1]-1, turboBlack, turboLightGray)
	}
}

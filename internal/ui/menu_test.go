package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMenuOpensArgumentPrompt(t *testing.T) {
	t.Parallel()

	state := shellState{menuOpen: true, helpVisible: true, runArgumentText: `"two words"`}
	state.runMenuAction(buildMenuSetup)
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	state.runSetupAction(1)
	assert.True(t, state.editingRunArgs)
	assert.False(t, state.runSetupOpen)
	assert.False(t, state.menuOpen)
	assert.False(t, state.helpVisible)
	assert.Equal(t, `"two words"`, state.runArgumentDraft)
}

func TestRunSetupMenuActions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Run Current File", menuActions[menuBuild][buildMenuCurrent].label)
	assert.Equal(t, "Run Options", menuActions[menuBuild][buildMenuSetup].label)
	assert.Equal(t, ">", menuActions[menuBuild][buildMenuSetup].shortcut)
	assert.Equal(t, "Run in Terminal", menuActions[menuBuild][buildMenuTerminal].label)
	assert.Len(t, menuActions[menuBuild], 9)
	assert.Equal(t, []menuAction{{"Default Package...", ""}, {"Arguments...", ""}}, runSetupActions)
}

func TestRunSetupKeyboardNavigation(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{}
	state.runMenuAction(buildMenuSetup)
	assert.True(t, state.runSetupOpen)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.Equal(t, 1, state.runSetupItem)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.editingRunArgs)
	assert.False(t, state.runSetupOpen)

	state.editingRunArgs = false
	state.runMenuAction(buildMenuSetup)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyLeft, 0, 0)))
	assert.False(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.Equal(t, buildMenuSetup, state.menuItem)
}

func TestBuildMenuOpensRunSetupWithKeyboard(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.False(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
	assert.Equal(t, menuBuild, state.menuIndex)
	assert.Equal(t, buildMenuSetup, state.menuItem)
	assert.Empty(t, state.message)
}

func TestBuildMenuRightOpensRunSetup(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{menuOpen: true, menuIndex: menuBuild, menuItem: buildMenuSetup}
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyRight, 0, 0)))
	assert.True(t, state.runSetupOpen)
	assert.True(t, state.menuOpen)
}

func TestRunSetupPreservesBackgroundMessageOnReturn(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, key := range []tcell.Key{tcell.KeyEscape, tcell.KeyLeft} {
		state := &shellState{}
		state.runMenuAction(buildMenuSetup)
		state.message = "Build finished"
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(key, 0, 0)))
		assert.Equal(t, "Build finished", state.message)
		assert.False(t, state.runSetupOpen)
		assert.True(t, state.menuOpen)
	}
}

func TestRunSetupGlobalShortcuts(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, key := range []tcell.Key{tcell.KeyCtrlQ, tcell.KeyCtrlC} {
		state := &shellState{}
		state.runMenuAction(buildMenuSetup)
		assert.True(t, handleEvent(screen, state, tcell.NewEventKey(key, 0, 0)))
	}
	state := &shellState{}
	state.runMenuAction(buildMenuSetup)
	state.message = "Build finished"
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF1, 0, 0)))
	assert.True(t, state.helpVisible)
	assert.False(t, state.runSetupOpen)
	assert.False(t, state.menuOpen)
	assert.Equal(t, "Build finished", state.message)
}

func TestBuildMenuPreparesTerminalRun(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/tool"))
	state.runMenuAction(buildMenuTerminal)
	require.NotNil(t, state.terminalRun)
	assert.Equal(t, "./cmd/tool", state.terminalRun.Target)
	assert.Empty(t, runner.started, "the command waits for terminal handoff")
}

func TestMenuLayout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, [menuCount]string{"File", "Search", "Build", "Window", "Help"}, menuLabels)
	for _, test := range []struct {
		name    string
		index   int
		actions []menuAction
	}{
		{"File", menuFile, []menuAction{{"Save", "F2"}, {}, {"Quit", "Ctrl+Q"}}},
		{"Search", menuSearch, []menuAction{{"Find...", "Ctrl+F"}, {"Find Next", "Ctrl+G"}, {}, {"Errors", "Alt+E"}, {}, {"Go to Definition", "F12"}, {"Find References", "Alt+R"}, {"Inspect Symbol", "Alt+I"}, {"Complete Symbol", "Alt+C"}}},
		{"Build", menuBuild, []menuAction{{"Build", "F9"}, {"Test", "Ctrl+T"}, {}, {"Run", "Ctrl+F9"}, {"Run Current File", "Alt+F9"}, {"Run in Terminal", ""}, {"Stop", "Ctrl+K"}, {}, {"Run Options", ">"}}},
		{"Window", menuWindow, []menuAction{{"New View", ""}, {}, {"Tile", ""}, {"Cascade", ""}, {}, {"Size / Move", "Ctrl+F5"}, {"Zoom", "F5"}, {}, {"Next", "F6"}, {"Previous", "Shift+F6"}, {"List...", "Alt+0"}, {}, {"Close", "Alt+F3"}}},
		{"Help", menuHelp, []menuAction{{"Shortcuts", ""}, {"Environment Info", ""}}},
	} {
		t.Run(test.name, func(t *testing.T) { assert.Equal(t, test.actions, menuActions[test.index]) })
	}
}

func TestMenusSkipSeparators(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		index int
		items []int
	}{
		{"File", menuFile, []int{0, 2}},
		{"Search", menuSearch, []int{0, 1, 3, 5, 6, 7, 8}},
		{"Build", menuBuild, []int{0, 1, 3, 4, 5, 6, 8}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := shellState{menuIndex: test.index}
			for _, item := range append(test.items[1:], 0) {
				state.moveMenuItem(1)
				assert.Equal(t, item, state.menuItem)
			}
			for i := len(test.items) - 1; i >= 0; i-- {
				state.moveMenuItem(-1)
				assert.Equal(t, test.items[i], state.menuItem)
			}
		})
	}
}

func TestSearchMenuRoutesGroupedActions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		item   int
		action string
	}{
		{"Errors", 3, "errors"},
		{"Definition", 5, "definition"},
		{"References", 6, "references"},
		{"Inspect", 7, "hover"},
		{"Complete", 8, "completion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(80, 24)
			state := newShellState("/tmp/project", func(workRequest) bool { return true })
			setTestDocument(t, &state, "/tmp/project/main.go", "package main\n")
			state.focus = focusEditor
			state.languageStatus = "ready"
			state.enqueueLanguage = func(languageSnapshot) {}
			action := ""
			state.enqueueNavigation = func(request languageNavigationRequest) { action = request.kind.name() }
			state.enqueueHover = func(languageHoverRequest) { action = "hover" }
			state.enqueueCompletion = func(languageCompletionRequest) { action = "completion" }
			state.menuOpen, state.menuIndex, state.menuItem = true, menuSearch, test.item
			assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
			if state.bottomMode == bottomErrors {
				action = "errors"
			}
			assert.Equal(t, test.action, action)
			assert.False(t, state.menuOpen)
		})
	}
}

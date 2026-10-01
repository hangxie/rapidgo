package ui

import "github.com/hangxie/rapidgo/internal/jobs"

const (
	menuFile = iota
	menuSearch
	menuBuild
	menuWindow
	menuHelp
	menuCount
)

var (
	menuLabels = [menuCount]string{"File", "Search", "Build", "Window", "Help"}
	menuX      = [menuCount]int{1, 7, 16, 24, 33}
)

type menuAction struct{ label, shortcut string }

const (
	windowNewView = iota
	_
	windowTile
	windowCascade
	_
	windowSizeMove
	windowZoom
	_
	windowNext
	windowPrevious
	windowList
	_
	windowClose
)

var menuActions = [menuCount][]menuAction{
	menuFile:   {{"Save", "F2"}, {}, {"Quit", "Ctrl+Q"}},
	menuSearch: {{"Find...", "Ctrl+F"}, {"Find Next", "Ctrl+G"}, {}, {"Errors", "Alt+E"}, {}, {"Go to Definition", "F12"}, {"Find References", "Alt+R"}, {"Inspect Symbol", "Alt+I"}, {"Complete Symbol", "Alt+C"}},
	menuBuild:  {{"Build", "F9"}, {"Test", "Ctrl+T"}, {}, {"Run", "Ctrl+F9"}, {"Run Current File", "Alt+F9"}, {"Run in Terminal", ""}, {"Stop", "Ctrl+K"}, {}, {"Run Options", ">"}},
	menuHelp:   {{"Shortcuts", ""}, {"Environment Info", ""}},
	menuWindow: {{"New View", ""}, {}, {"Tile", ""}, {"Cascade", ""}, {}, {"Size / Move", "Ctrl+F5"}, {"Zoom", "F5"}, {}, {"Next", "F6"}, {"Previous", "Shift+F6"}, {"List...", "Alt+0"}, {}, {"Close", "Alt+F3"}},
}

func (state *shellState) moveMenuItem(delta int) {
	actions := menuActions[state.menuIndex]
	for range len(actions) {
		state.menuItem = (state.menuItem + delta + len(actions)) % len(actions)
		if actions[state.menuItem].label != "" {
			return
		}
	}
}

const (
	buildMenuBuild = iota
	buildMenuTest
	_
	buildMenuRun
	buildMenuCurrent
	buildMenuTerminal
	buildMenuStop
	_
	buildMenuSetup
)

// buildMenuKinds maps Build menu actions to job kinds.
var buildMenuKinds = map[int]jobs.Kind{buildMenuBuild: jobs.Build, buildMenuTest: jobs.Test, buildMenuRun: jobs.Run}

// runSetupActions lists the settings available under Run Options.
var runSetupActions = []menuAction{{"Default Package...", ""}, {"Arguments...", ""}}

const runSetupHint = "Run options: Up/Down and Enter to select, Left/Esc to return"

func (state *shellState) runMenuAction(item int) {
	if kind, ok := buildMenuKinds[item]; ok {
		state.startJob(kind)
		return
	}
	switch item {
	case buildMenuCurrent:
		state.requestCurrentEntry()
	case buildMenuSetup:
		state.menuOpen = true
		state.menuIndex = menuBuild
		state.menuItem = buildMenuSetup
		state.helpVisible = false
		state.runSetupOpen = true
		state.runSetupItem = 0
		state.message = runSetupHint
	case buildMenuTerminal:
		state.requestTerminalRun()
	case buildMenuStop:
		state.stopJob()
	}
}

// runSetupAction opens a run setting selected from the submenu.
func (state *shellState) runSetupAction(item int) {
	state.runSetupOpen = false
	state.menuOpen = false
	switch item {
	case 0:
		state.chooseRunTarget()
	case 1:
		state.editRunArguments()
	}
}

// closeRunSetup keeps messages that arrived after the submenu opened.
func (state *shellState) closeRunSetup() {
	state.runSetupOpen = false
	if state.message == runSetupHint {
		state.message = ""
	}
}

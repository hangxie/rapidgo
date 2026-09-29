package ui

import "github.com/hangxie/rapidgo/internal/jobs"

const (
	menuFile = iota
	menuSearch
	menuBuild
	menuHelp
	menuCount
)

var (
	menuLabels = [menuCount]string{"File", "Search", "Build", "Help"}
	menuX      = [menuCount]int{1, 7, 16, 24}
)

type menuAction struct{ label, shortcut string }

var menuActions = [menuCount][]menuAction{
	menuFile:   {{"Save", "F2"}, {"Quit", "Ctrl+Q"}},
	menuSearch: {{"Find", "Ctrl+F"}, {"Find Next", "Ctrl+G"}, {"Inspect Symbol", "Alt+I"}, {"Errors", "Alt+E"}},
	menuBuild:  {{"Build", "F9"}, {"Test", "Ctrl+T"}, {"Run", "Ctrl+F9"}, {"Run Current File", "Alt+F9"}, {"Run Setup", ">"}, {"Run in Terminal", ""}, {"Stop", "Ctrl+K"}},
	menuHelp:   {{"Shortcuts", "F1"}, {"Environment", ""}},
}

// buildMenuKinds maps the Build menu's leading actions to job kinds.
var buildMenuKinds = []jobs.Kind{jobs.Build, jobs.Test, jobs.Run}

// runSetupActions lists the settings available under Run Setup.
var runSetupActions = []menuAction{{"Set Default Package", ""}, {"Run Arguments", ""}}

const runSetupHint = "Run setup: Up/Down and Enter to select, Left/Esc to return"

// buildMenuSetup is the index of the run settings action.
var (
	buildMenuCurrent  = len(buildMenuKinds)
	buildMenuSetup    = buildMenuCurrent + 1
	buildMenuTerminal = buildMenuSetup + 1
)

func (state *shellState) runMenuAction(item int) {
	switch {
	case item >= 0 && item < len(buildMenuKinds):
		state.startJob(buildMenuKinds[item])
	case item == buildMenuCurrent:
		state.requestCurrentEntry()
	case item == buildMenuSetup:
		state.menuOpen = true
		state.menuIndex = menuBuild
		state.menuItem = buildMenuSetup
		state.helpVisible = false
		state.runSetupOpen = true
		state.runSetupItem = 0
		state.message = runSetupHint
	case item == buildMenuTerminal:
		state.requestTerminalRun()
	default:
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

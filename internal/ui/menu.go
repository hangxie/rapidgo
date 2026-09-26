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
	menuSearch: {{"Find", "Ctrl+F"}, {"Find Next", "Ctrl+G"}},
	menuBuild:  {{"Build", "F9"}, {"Test", "Ctrl+T"}, {"Run", "Ctrl+F9"}, {"Run Target", ""}, {"Stop", "Ctrl+K"}},
	menuHelp:   {{"Shortcuts", "F1"}},
}

// buildMenuKinds maps the Build menu's leading actions to job kinds. The two
// that follow choose the default run target and stop running commands.
var buildMenuKinds = []jobs.Kind{jobs.Build, jobs.Test, jobs.Run}

// buildMenuTarget is the index of the Run Target action.
var buildMenuTarget = len(buildMenuKinds)

func (state *shellState) runMenuAction(item int) {
	switch {
	case item >= 0 && item < len(buildMenuKinds):
		state.startJob(buildMenuKinds[item])
	case item == buildMenuTarget:
		state.chooseRunTarget()
	default:
		state.stopJob()
	}
}

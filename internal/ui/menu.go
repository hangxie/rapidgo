package ui

const (
	menuFile = iota
	menuSearch
	menuHelp
	menuCount
)

var (
	menuLabels = [menuCount]string{"File", "Search", "Help"}
	menuX      = [menuCount]int{1, 7, 16}
)

type menuAction struct{ label, shortcut string }

var menuActions = [menuCount][]menuAction{
	menuFile:   {{"Save", "F2"}, {"Quit", "Ctrl+Q"}},
	menuSearch: {{"Find", "Ctrl+F"}, {"Find Next", "Ctrl+G"}},
	menuHelp:   {{"Shortcuts", "F1"}},
}

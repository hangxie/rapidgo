package ui

const (
	menuFile = iota
	menuHelp
	menuCount
)

var (
	menuLabels = [menuCount]string{"File", "Help"}
	menuX      = [menuCount]int{1, 7}
)

var (
	menuActionLabels    = [menuCount]string{"Quit", "Shortcuts"}
	menuActionShortcuts = [menuCount]string{"Ctrl+Q", "F1"}
)

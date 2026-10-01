package ui

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

const (
	menuFile = iota
	menuSearch
	menuBuild
	menuWindow
	menuHelp
	menuCount
)

type menuDefinition struct {
	labelKey, mnemonicKey string
	mnemonic              rune
}

var menuDefinitions = [menuCount]menuDefinition{
	{"msg_file", "menu_file_mnemonic_index", 'F'},
	{"msg_search", "menu_search_mnemonic_index", 'S'},
	{"msg_build", "menu_build_mnemonic_index", 'B'},
	{"msg_window", "menu_window_mnemonic_index", 'W'},
	{"msg_help", "menu_help_mnemonic_index", 'H'},
}

type menuBarEntry struct {
	label, mnemonic string
	cell            int
}

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

var menuActionKeys = [menuCount][]menuAction{
	menuFile:   {{"msg_save", "msg_f2"}, {}, {"msg_quit", "msg_ctrl_q"}},
	menuSearch: {{"msg_find", "msg_ctrl_f"}, {"msg_find_next", "msg_ctrl_g"}, {}, {"msg_errors", "msg_alt_e"}, {}, {"msg_go_to_definition", "msg_f12"}, {"msg_find_references", "msg_alt_r"}, {"msg_inspect_symbol", "msg_alt_i"}, {"msg_complete_symbol", "msg_alt_c"}},
	menuBuild:  {{"msg_build", "msg_f9"}, {"msg_test", "msg_ctrl_t"}, {}, {"msg_run", "msg_ctrl_f9"}, {"msg_run_current_file", "msg_alt_f9"}, {"msg_run_in_terminal", ""}, {"msg_stop", "msg_ctrl_k"}, {}, {"msg_run_options", ">"}},
	menuHelp:   {{"msg_shortcuts", ""}, {"msg_environment_info", ""}},
	menuWindow: {{"msg_new_view", ""}, {}, {"msg_tile", ""}, {"msg_cascade", ""}, {}, {"msg_size_move", "msg_ctrl_f5"}, {"msg_zoom", "msg_f5"}, {}, {"msg_next", "msg_f6"}, {"msg_previous", "msg_shift_f6"}, {"msg_list", "msg_alt_0"}, {}, {"msg_close", "msg_alt_f3"}},
}

func (state *shellState) moveMenuItem(delta int) {
	actions := menuActionKeys[state.menuIndex]
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

// runSetupActionKeys lists the settings available under Run Options.
var runSetupActionKeys = []menuAction{{"msg_default_package", ""}, {"msg_arguments", ""}}

const runSetupHint = "msg_run_options_up_down_and_enter_to_select_left_esc_to_return"

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
		state.message = i18n.Text(runSetupHint)
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
	if state.message == i18n.Text(runSetupHint) {
		state.message = ""
	}
}

// menuActions resolves labels when displayed, after startup locale selection.
func menuActions(index int) []menuAction { return translateActions(menuActionKeys[index]) }

func runSetupActions() []menuAction { return translateActions(runSetupActionKeys) }

func translateActions(keys []menuAction) []menuAction {
	actions := make([]menuAction, len(keys))
	for index, action := range keys {
		actions[index] = menuAction{i18n.Text(action.label), action.shortcut}
		if action.shortcut != "" && action.shortcut != ">" {
			actions[index].shortcut = i18n.Text(action.shortcut)
		}
	}
	return actions
}

func menuLabels() [menuCount]string {
	var labels [menuCount]string
	for index, entry := range menuBarEntries() {
		labels[index] = entry.label
	}
	return labels
}

func menuBarEntries() [menuCount]menuBarEntry {
	var entries [menuCount]menuBarEntry
	for index, definition := range menuDefinitions {
		label := i18n.Text(definition.labelKey)
		position, err := strconv.Atoi(i18n.Text(definition.mnemonicKey))
		text, cell := "", 0
		if err == nil {
			text, cell = declaredMnemonic(label, position, definition.mnemonic)
		}
		if text == "" {
			text = string(definition.mnemonic)
			cell = uniseg.StringWidth(label) + 2
			label += " (" + text + ")"
		}
		entries[index] = menuBarEntry{label: label, mnemonic: text, cell: cell}
	}
	return entries
}

func declaredMnemonic(label string, position int, mnemonic rune) (string, int) {
	clusters := uniseg.NewGraphemes(label)
	cell := 0
	for index := 0; clusters.Next(); index++ {
		if index == position {
			char, _ := utf8.DecodeRuneInString(clusters.Str())
			if unicode.ToUpper(char) == mnemonic {
				return clusters.Str(), cell
			}
			break
		}
		cell += clusters.Width()
	}
	return "", 0
}

func menuX(index int) int { return menuPositions(menuLabels())[index] }

func menuPositions(labels [menuCount]string) [menuCount]int {
	positions := [menuCount]int{1}
	for index := 1; index < menuCount; index++ {
		positions[index] = positions[index-1] + uniseg.StringWidth(labels[index-1]) + 2
		if index > 1 {
			positions[index]++
		}
	}
	return positions
}

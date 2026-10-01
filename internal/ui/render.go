package ui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

var (
	baseStyle               = tcell.StyleDefault.Foreground(turboYellow).Background(turboBlue)
	titleStyle              = tcell.StyleDefault.Foreground(turboWhite).Background(turboBlue)
	frameStyle              = tcell.StyleDefault.Foreground(turboLightCyan).Background(turboBlue)
	barStyle                = tcell.StyleDefault.Foreground(turboBlack).Background(turboLightGray)
	shortcutStyle           = tcell.StyleDefault.Foreground(turboRed).Background(turboLightGray)
	menuActiveStyle         = tcell.StyleDefault.Foreground(turboBlack).Background(turboGreen)
	menuActiveMnemonicStyle = tcell.StyleDefault.Foreground(turboRed).Background(turboGreen)
	helpStyle               = barStyle
	helpBorderStyle         = tcell.StyleDefault.Foreground(turboWhite).Background(turboLightGray)
	shadowStyle             = tcell.StyleDefault.Foreground(turboBlack).Background(turboBlack)
	treeSelectedStyle       = menuActiveStyle
	editorSelectionStyle    = tcell.StyleDefault.Foreground(turboBlack).Background(turboLightCyan)
	keywordStyle            = tcell.StyleDefault.Foreground(turboWhite).Background(turboBlue)
	stringStyle             = tcell.StyleDefault.Foreground(turboYellow).Background(turboBlue)
	numberStyle             = tcell.StyleDefault.Foreground(turboLightCyan).Background(turboBlue)
	commentStyle            = tcell.StyleDefault.Foreground(turboLightGray).Background(turboBlue)
	outputErrorStyle        = tcell.StyleDefault.Foreground(turboLightRed).Background(turboBlue)
	messageStyle            = tcell.StyleDefault.Foreground(turboWhite).Background(turboBlue)
)

type textSegment struct {
	text  string
	style tcell.Style
}

type rectangle struct {
	x, y, width, height int
}

type layout struct {
	menu, message, status   rectangle
	project, editor, output rectangle
	projectVisible          bool
}

// calculateLayout divides the terminal into its bars, work area, and message line.
func calculateLayout(width, height int, outputFocused bool) layout {
	if width <= 0 || height <= 0 {
		return layout{}
	}
	result := layout{menu: rectangle{width: width, height: 1}}
	if height == 1 {
		return result
	}
	result.status = rectangle{y: height - 1, width: width, height: 1}
	if height == 2 {
		return result
	}
	contentHeight := height - 2
	// The message line is the first thing a cramped terminal gives up.
	if height >= 5 {
		result.message = rectangle{y: height - 2, width: width, height: 1}
		contentHeight--
	}
	if contentHeight <= 0 {
		return result
	}

	outputHeight := 0
	if contentHeight >= 2 {
		share := 4
		if outputFocused {
			share = 2
		}
		outputHeight = contentHeight / share
		if outputHeight < 2 && contentHeight >= 4 {
			outputHeight = 2
		}
		if outputHeight < 1 {
			outputHeight = 1
		}
	}
	editorHeight := contentHeight - outputHeight
	result.editor = rectangle{y: 1, width: width, height: editorHeight}
	result.output = rectangle{y: 1 + editorHeight, width: width, height: outputHeight}
	if width >= 60 && editorHeight > 0 {
		projectWidth := width / 4
		if projectWidth > 28 {
			projectWidth = 28
		}
		result.project = rectangle{y: 1, width: projectWidth, height: editorHeight}
		result.editor.x = projectWidth + 1
		result.editor.width = width - result.editor.x
		result.projectVisible = true
	}
	return result
}

func render(screen tcell.Screen, state shellState) {
	width, height := screen.Size()
	view := calculateLayout(width, height, state.focus == focusOutput)
	screen.SetStyle(baseStyle)
	screen.Fill(' ', baseStyle)
	screen.HideCursor()
	if width <= 0 || height <= 0 {
		return
	}

	renderMenuBar(screen, width, view.menu.y, state)
	renderPanes(screen, view, state)
	message := state.message
	if problem := state.languageLineMessage(); problem != "" && state.confirm == confirmNone && !state.saving && !state.completionVisible && (state.saveErrorMessage == "" || state.message != state.saveErrorMessage) && state.message != i18n.Text("msg_inspecting_symbol_with_gopls") && state.message != i18n.Text("msg_completing_with_gopls") {
		message = problem
	}
	renderMessageLine(screen, view.message, message)
	renderStatusBar(screen, view, state)
	if state.menuOpen && height > 2 {
		renderMenu(screen, width, height, state.menuIndex, state.menuItem)
	}
	if state.runSetupOpen && height > 2 {
		x := menuX(menuBuild) + menuWidth(menuActions(menuBuild)) - 1
		y := buildMenuSetup + 2
		renderMenuActionsAt(screen, width, height, x, y, runSetupActions(), state.runSetupItem)
	}
	if state.helpVisible {
		renderHelp(screen, width, height, state)
	}
	if state.confirm != confirmNone {
		renderConfirmation(screen, width, height, state)
	}
	if state.chooser != nil {
		renderRunChooser(screen, width, height, state.chooser)
	}
	if state.windowChooser != nil {
		renderWindowList(screen, width, height, state)
	}
	if state.hoverVisible {
		renderHover(screen, width, height, state)
	}
	if state.completionVisible {
		renderCompletion(screen, width, height, state)
	}
	if (state.helpVisible && helpFits(width, height)) || state.menuOpen || state.runSetupOpen || state.confirm != confirmNone || state.chooser != nil || state.windowChooser != nil || state.hoverVisible || state.completionVisible {
		screen.HideCursor()
	}
	screen.Show()
}

func renderMenuBar(screen tcell.Screen, width, y int, state shellState) {
	fillRow(screen, y, width, barStyle)
	entries := menuBarEntries()
	var labels [menuCount]string
	for index, entry := range entries {
		labels[index] = entry.label
	}
	positions := menuPositions(labels)
	for index, label := range labels {
		style := barStyle
		mnemonicStyle := shortcutStyle
		if (state.menuOpen && state.menuIndex == index) || (state.runSetupOpen && index == menuBuild) {
			style = menuActiveStyle
			mnemonicStyle = menuActiveMnemonicStyle
		}
		drawText(screen, positions[index], y, width-positions[index], " "+label+" ", style)
		mnemonicX := positions[index] + 1 + entries[index].cell
		drawText(screen, mnemonicX, y, width-mnemonicX, entries[index].mnemonic, mnemonicStyle)
	}
}

func renderPanes(screen tcell.Screen, view layout, state shellState) {
	// A narrow terminal shows one pane in the work area. While the output
	// pane has focus that is the tree or editor the user came from.
	main := state.focus
	if main == focusOutput {
		main = state.mainFocus
	}
	if view.projectVisible || main == focusTree {
		area := view.project
		if !view.projectVisible {
			area = view.editor
		}
		if state.tree == nil {
			drawPane(screen, area, i18n.Text("msg_project"), i18n.Text("msg_files_coming_next"))
		} else {
			renderTree(screen, area, state)
		}
	}
	if view.editor.height > 0 && (view.projectVisible || main == focusEditor) {
		renderWorkspace(screen, view.editor, state)
	}
	if view.output.height > 0 {
		renderOutput(screen, view.output, state)
	}
}

// renderMessageLine shows the latest transient message on its own row.
func renderMessageLine(screen tcell.Screen, area rectangle, message string) {
	if area.height == 0 {
		return
	}
	fillRow(screen, area.y, area.width, messageStyle)
	if message == "" {
		message = i18n.Text("msg_ready")
	}
	drawText(screen, 1, area.y, area.width-1, message, messageStyle)
}

// renderOutput draws the visible job's output, tailing until scrolled away.
func renderOutput(screen tcell.Screen, area rectangle, state shellState) {
	if state.bottomMode == bottomErrors {
		renderErrors(screen, area, state)
		return
	}
	if state.bottomMode == bottomLocations {
		renderLocations(screen, area, state)
		return
	}
	active := state.focus == focusOutput
	job := state.activeView()
	if job == nil {
		drawFrame(screen, area, i18n.Text("msg_output"), active)
		if area.width >= 4 && area.height > 2 {
			drawText(screen, area.x+2, area.y+1, area.width-4, i18n.Text("msg_no_output_yet_press_f9_to_build"), baseStyle)
		}
		return
	}
	rows := max(1, area.height-2)
	drawFrame(screen, area, job.title()+job.position(rows), active)
	if area.width < 4 || area.height < 3 {
		return
	}
	top := job.top(rows)
	for row, line := range job.visibleLines(rows) {
		style := baseStyle
		switch {
		case active && top+row == job.selected:
			style = treeSelectedStyle
			for col := area.x + 1; col < area.x+area.width-1; col++ {
				screen.SetContent(col, area.y+1+row, ' ', nil, style)
			}
		case line.stream == jobs.Stderr:
			style = outputErrorStyle
		}
		drawText(screen, area.x+1, area.y+1+row, area.width-2, line.text, style)
	}
}

func renderStatusBar(screen tcell.Screen, view layout, state shellState) {
	if view.status.height == 0 {
		return
	}
	width := view.status.width
	fillRow(screen, view.status.y, width, barStyle)
	if state.searching {
		renderSearchStatus(screen, view.status, state.searchInput)
		return
	}
	if state.editingRunArgs {
		renderPromptStatus(screen, view.status, i18n.Text("msg_run_args"), state.runArgumentDraft)
		return
	}
	if state.helpVisible {
		drawStyledText(screen, 1, view.status.y, width-1, []textSegment{{i18n.Text("msg_esc"), shortcutStyle}, {i18n.Text("msg_close_help"), barStyle}, {i18n.Text("msg_ctrl_q"), shortcutStyle}, {i18n.Text("msg_quit_2"), barStyle}})
		return
	}
	status := []textSegment{{i18n.Text("msg_f2"), shortcutStyle}, {i18n.Text("msg_save_2"), barStyle}, {i18n.Text("msg_ctrl_f"), shortcutStyle}, {i18n.Text("msg_find_2"), barStyle}, {i18n.Text("msg_f3"), shortcutStyle}, {i18n.Text("msg_tree"), barStyle}, {i18n.Text("msg_f6"), shortcutStyle}, {i18n.Text("msg_window_2"), barStyle}, {i18n.Text("msg_f10"), shortcutStyle}, {i18n.Text("msg_menu"), barStyle}, {i18n.Text("msg_f1"), shortcutStyle}, {i18n.Text("msg_help_3"), barStyle}, {i18n.Text("msg_ctrl_q"), shortcutStyle}, {i18n.Text("msg_quit_2"), barStyle}}
	if width < 44 {
		status = []textSegment{{i18n.Text("msg_f3"), shortcutStyle}, {i18n.Text("msg_tree_2"), barStyle}, {i18n.Text("msg_f10"), shortcutStyle}, {i18n.Text("msg_menu_2"), barStyle}, {i18n.Text("msg_f1"), shortcutStyle}, {i18n.Text("msg_help_2"), barStyle}, {i18n.Text("msg_q"), shortcutStyle}, {i18n.Text("msg_quit_2"), barStyle}}
	}
	if width < 34 {
		status = []textSegment{{i18n.Text("msg_f3"), shortcutStyle}, {i18n.Text("msg_tree_2"), barStyle}, {i18n.Text("msg_f10"), shortcutStyle}, {" ", barStyle}, {i18n.Text("msg_f1"), shortcutStyle}, {" ", barStyle}, {i18n.Text("msg_q"), shortcutStyle}, {i18n.Text("msg_quit_2"), barStyle}}
	}
	if width < 24 {
		status = []textSegment{{i18n.Text("msg_f3"), shortcutStyle}, {" ", barStyle}, {i18n.Text("msg_f1"), shortcutStyle}, {" ", barStyle}, {i18n.Text("msg_q"), shortcutStyle}}
	}
	if !view.projectVisible {
		status = append(status, textSegment{"  " + filepath.Base(state.projectRoot), barStyle})
	}
	drawStyledText(screen, 1, view.status.y, width-1, status)
}

func renderSearchStatus(screen tcell.Screen, area rectangle, input string) {
	renderPromptStatus(screen, area, i18n.Text("msg_search_2"), input)
}

func renderPromptStatus(screen tcell.Screen, area rectangle, label, input string) {
	if area.width < 2 {
		return
	}
	drawText(screen, 1, area.y, area.width-1, label, shortcutStyle)
	start := 1 + uniseg.StringWidth(label)
	available := max(0, area.width-start-1)
	// Keep the newest graphemes visible as the query grows beyond the bar.
	remaining := uniseg.StringWidth(input)
	for remaining > available && input != "" {
		clusters := uniseg.NewGraphemes(input)
		clusters.Next()
		_, end := clusters.Positions()
		remaining -= uniseg.StringWidth(input[:end])
		input = input[end:]
	}
	drawText(screen, start, area.y, available, input, barStyle)
	if start < area.width {
		screen.ShowCursor(min(area.width-1, start+remaining), area.y)
	}
}

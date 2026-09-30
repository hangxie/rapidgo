package ui

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

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
	if problem := state.languageLineMessage(); problem != "" && state.confirm == confirmNone && !state.saving && !state.completionVisible && !strings.HasPrefix(state.message, "Save failed") && !strings.HasPrefix(state.message, "Inspecting ") && !strings.HasPrefix(state.message, "Completing ") {
		message = problem
	}
	renderMessageLine(screen, view.message, message)
	renderStatusBar(screen, view, state)
	if state.menuOpen && height > 2 {
		renderMenu(screen, width, height, state.menuIndex, state.menuItem)
	}
	if state.runSetupOpen && height > 2 {
		x := menuX[menuBuild] + menuWidth(menuActions[menuBuild]) - 1
		y := buildMenuSetup + 2
		renderMenuActionsAt(screen, width, height, x, y, runSetupActions, state.runSetupItem)
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
	if state.hoverVisible {
		renderHover(screen, width, height, state)
	}
	if state.completionVisible {
		renderCompletion(screen, width, height, state)
	}
	if (state.helpVisible && helpFits(width, height)) || state.menuOpen || state.runSetupOpen || state.confirm != confirmNone || state.chooser != nil || state.hoverVisible || state.completionVisible {
		screen.HideCursor()
	}
	screen.Show()
}

func renderMenuBar(screen tcell.Screen, width, y int, state shellState) {
	fillRow(screen, y, width, barStyle)
	for index, label := range menuLabels {
		style := barStyle
		mnemonicStyle := shortcutStyle
		if (state.menuOpen && state.menuIndex == index) || (state.runSetupOpen && index == menuBuild) {
			style = menuActiveStyle
			mnemonicStyle = menuActiveMnemonicStyle
		}
		drawText(screen, menuX[index], y, width-menuX[index], " "+label+" ", style)
		drawText(screen, menuX[index]+1, y, width-menuX[index]-1, label[:1], mnemonicStyle)
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
			drawPane(screen, area, "PROJECT", "Files coming next")
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
		message = "Ready"
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
		drawFrame(screen, area, "OUTPUT", active)
		if area.width >= 4 && area.height > 2 {
			drawText(screen, area.x+2, area.y+1, area.width-4, "No output yet; press F9 to build", baseStyle)
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
		renderPromptStatus(screen, view.status, "Run args: ", state.runArgumentDraft)
		return
	}
	if state.helpVisible {
		drawStyledText(screen, 1, view.status.y, width-1, []textSegment{{"Esc", shortcutStyle}, {" Close help  ", barStyle}, {"Ctrl+Q", shortcutStyle}, {" Quit", barStyle}})
		return
	}
	status := []textSegment{{"F2", shortcutStyle}, {" Save  ", barStyle}, {"Ctrl+F", shortcutStyle}, {" Find  ", barStyle}, {"F3", shortcutStyle}, {" Tree  ", barStyle}, {"F6", shortcutStyle}, {" Window  ", barStyle}, {"F10", shortcutStyle}, {" Menu  ", barStyle}, {"F1", shortcutStyle}, {" Help  ", barStyle}, {"Ctrl+Q", shortcutStyle}, {" Quit", barStyle}}
	if width < 44 {
		status = []textSegment{{"F3", shortcutStyle}, {" Tree ", barStyle}, {"F10", shortcutStyle}, {" Menu ", barStyle}, {"F1", shortcutStyle}, {" Help ", barStyle}, {"^Q", shortcutStyle}, {" Quit", barStyle}}
	}
	if width < 34 {
		status = []textSegment{{"F3", shortcutStyle}, {" Tree ", barStyle}, {"F10", shortcutStyle}, {" ", barStyle}, {"F1", shortcutStyle}, {" ", barStyle}, {"^Q", shortcutStyle}, {" Quit", barStyle}}
	}
	if width < 24 {
		status = []textSegment{{"F3", shortcutStyle}, {" ", barStyle}, {"F1", shortcutStyle}, {" ", barStyle}, {"^Q", shortcutStyle}}
	}
	if !view.projectVisible {
		status = append(status, textSegment{"  " + filepath.Base(state.projectRoot), barStyle})
	}
	drawStyledText(screen, 1, view.status.y, width-1, status)
}

func renderSearchStatus(screen tcell.Screen, area rectangle, input string) {
	renderPromptStatus(screen, area, "Search: ", input)
}

func renderPromptStatus(screen tcell.Screen, area rectangle, label, input string) {
	if area.width < 2 {
		return
	}
	drawText(screen, 1, area.y, area.width-1, label, shortcutStyle)
	start := 1 + len(label)
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

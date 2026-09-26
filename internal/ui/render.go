package ui

import (
	"fmt"
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
	renderMessageLine(screen, view.message, state.message)
	renderStatusBar(screen, view, state)
	if state.menuOpen && height > 2 {
		renderMenu(screen, width, height, state.menuIndex, state.menuItem)
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
	if state.helpVisible || state.menuOpen || state.confirm != confirmNone || state.chooser != nil {
		screen.HideCursor()
	}
	screen.Show()
}

func renderMenuBar(screen tcell.Screen, width, y int, state shellState) {
	fillRow(screen, y, width, barStyle)
	for index, label := range menuLabels {
		style := barStyle
		mnemonicStyle := shortcutStyle
		if state.menuOpen && state.menuIndex == index {
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
		renderDocument(screen, view.editor, state)
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
	status := []textSegment{{"F2", shortcutStyle}, {" Save  ", barStyle}, {"Ctrl+F", shortcutStyle}, {" Find  ", barStyle}, {"F3", shortcutStyle}, {" Tree  ", barStyle}, {"F6", shortcutStyle}, {" Pane  ", barStyle}, {"F10", shortcutStyle}, {" Menu  ", barStyle}, {"F1", shortcutStyle}, {" Help  ", barStyle}, {"Ctrl+Q", shortcutStyle}, {" Quit", barStyle}}
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
	if area.width < 2 {
		return
	}
	label := "Search: "
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

func renderMenu(screen tcell.Screen, width, height, index, selected int) {
	x := menuX[index]
	if x >= width {
		return
	}
	actions := menuActions[index]
	boxWidth := 21
	if boxWidth > width-x {
		boxWidth = width - x
	}
	boxHeight := len(actions) + 2
	if height < boxHeight+2 || boxWidth < 4 {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, 1, ' ', nil, barStyle)
		}
		drawText(screen, x+1, 1, boxWidth-1, actions[selected].label, barStyle)
		return
	}
	bottom := boxHeight
	if bottom+1 < height-1 {
		shadowRow(screen, x+2, bottom+1, boxWidth-1, width)
	}
	for row := 1; row <= bottom; row++ {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, row, ' ', nil, barStyle)
		}
		if row > 1 && x+boxWidth < width {
			screen.SetContent(x+boxWidth, row, ' ', nil, shadowStyle)
		}
	}
	right := x + boxWidth - 1
	for col := x + 1; col < right; col++ {
		screen.SetContent(col, 1, '─', nil, helpBorderStyle)
		screen.SetContent(col, bottom, '─', nil, helpBorderStyle)
	}
	screen.SetContent(x, 1, '┌', nil, helpBorderStyle)
	screen.SetContent(right, 1, '┐', nil, helpBorderStyle)
	for item, action := range actions {
		row := item + 2
		style, shortcut := barStyle, shortcutStyle
		if item == selected {
			style, shortcut = menuActiveStyle, menuActiveMnemonicStyle
		}
		for col := x + 1; col < right; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
		screen.SetContent(x, row, '│', nil, helpBorderStyle)
		screen.SetContent(right, row, '│', nil, helpBorderStyle)
		drawText(screen, x+2, row, boxWidth-3, action.label, style)
		drawText(screen, x+13, row, boxWidth-14, action.shortcut, shortcut)
	}
	screen.SetContent(x, bottom, '└', nil, helpBorderStyle)
	screen.SetContent(right, bottom, '┘', nil, helpBorderStyle)
}

// helpEntry pairs a shortcut with its action.
type helpEntry struct{ shortcut, action string }

// helpEntries lists keyboard actions and the detected toolchain.
func helpEntries(state shellState) []helpEntry {
	return []helpEntry{
		{"F3", "Focus tree"},
		{"F6 / Ctrl+F6", "Next pane"},
		{"Tree Up/Down", "Select item"},
		{"Tree Left/Right", "Fold / expand"},
		{"Tree Enter", "Open item"},
		{"Tree Home/End", "First / last item"},
		{"Tree PgUp/PgDn", "Move by page"},
		{"Editor arrows", "Move cursor"},
		{"Editor Home/End", "Line ends"},
		{"Editor PgUp/PgDn", "Move by page"},
		{"Ctrl+Home/End", "File ends"},
		{"Shift+movement", "Extend selection"},
		{"Tab / Shift+Tab", "Indent / unindent"},
		{"Enter", "New line"},
		{"Backspace / Delete", "Erase"},
		{"Ctrl+A", "Select all"},
		{"Ctrl+Z / Ctrl+Y", "Undo / redo"},
		{"F2", "Save"},
		{"Ctrl+F / Ctrl+G", "Find / next"},
		{"Search Enter / Esc", "Find / cancel"},
		{"F9", "Build"},
		{"Ctrl+T", "Test"},
		{"Ctrl+F9", "Run"},
		{"Ctrl+K", "Stop all Go jobs"},
		{"Output Up/Down", "Select line"},
		{"Output PgUp/PgDn", "Move by page"},
		{"Output Home/End", "First / latest line"},
		{"Output Left/Right", "Switch command"},
		{"Output Enter", "Jump to problem"},
		{"F10", "Open menu"},
		{"Alt+F/S/B/H", "Choose menu"},
		{"Menu arrows/Enter/Esc", "Navigate / act / close"},
		{"F1 / Esc", "Help / close"},
		{"Ctrl+Q / Ctrl+C", "Quit"},
		{"Unsaved D / Esc", "Discard / cancel"},
		{"Go toolchain", state.toolchainStatus()},
	}
}

// helpRows wraps help entries to the available dialog width.
func helpRows(state shellState, width int) [][]textSegment {
	if width < 1 {
		return nil
	}
	const keyWidth = 23
	wide := width >= 44
	rows := [][]textSegment{}
	if wide {
		rows = append(rows, []textSegment{{"Shortcut" + strings.Repeat(" ", keyWidth-len("Shortcut")), helpStyle}, {"Action", helpStyle}})
	}
	for _, entry := range helpEntries(state) {
		gap := "  "
		indent := 0
		if wide {
			gap = strings.Repeat(" ", max(2, keyWidth-uniseg.StringWidth(entry.shortcut)))
			indent = keyWidth
		}
		line := []textSegment{{entry.shortcut, shortcutStyle}, {gap + entry.action, helpStyle}}
		row := []textSegment{}
		used := 0
		for _, segment := range line {
			clusters := uniseg.NewGraphemes(segment.text)
			for clusters.Next() {
				cluster := clusters.Str()
				cells := uniseg.StringWidth(cluster)
				if used > 0 && used+cells > width {
					rows = append(rows, row)
					row = []textSegment{{strings.Repeat(" ", indent), helpStyle}}
					used = indent
				}
				if cells > width {
					continue
				}
				if len(row) > 0 && row[len(row)-1].style == segment.style {
					row[len(row)-1].text += cluster
				} else {
					row = append(row, textSegment{cluster, segment.style})
				}
				used += cells
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// helpPageSize returns the number of help entries that fit above the footer.
func helpPageSize(height, lineCount int) int {
	return max(0, min(height-2, lineCount+5)-4)
}

func renderHelp(screen tcell.Screen, width, height int, state shellState) {
	if width < 16 || height < 5 {
		return
	}
	boxWidth := min(width-2, 56)
	lines := helpRows(state, boxWidth-4)
	boxHeight := min(height-2, len(lines)+5)
	x := (width - boxWidth) / 2
	y := (height - boxHeight) / 2
	for row := y + 1; row < y+boxHeight+1 && row < height-1; row++ {
		for col := x + 2; col < x+boxWidth+2 && col < width; col++ {
			screen.SetContent(col, row, ' ', nil, shadowStyle)
		}
	}
	for row := y; row < y+boxHeight; row++ {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, row, ' ', nil, helpStyle)
		}
	}
	for col := x + 1; col < x+boxWidth-1; col++ {
		screen.SetContent(col, y, '─', nil, helpBorderStyle)
		screen.SetContent(col, y+boxHeight-1, '─', nil, helpBorderStyle)
	}
	for row := y + 1; row < y+boxHeight-1; row++ {
		screen.SetContent(x, row, '│', nil, helpBorderStyle)
		screen.SetContent(x+boxWidth-1, row, '│', nil, helpBorderStyle)
	}
	screen.SetContent(x, y, '┌', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y, '┐', nil, helpBorderStyle)
	screen.SetContent(x, y+boxHeight-1, '└', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y+boxHeight-1, '┘', nil, helpBorderStyle)
	drawText(screen, x+2, y+1, boxWidth-3, "RapidGo help", helpStyle)
	page := helpPageSize(height, len(lines))
	start := min(state.helpScroll, max(0, len(lines)-page))
	for index, line := range lines[start:min(len(lines), start+page)] {
		row := y + 2 + index
		drawStyledText(screen, x+2, row, boxWidth-3, line)
	}
	if boxHeight >= 4 {
		footer := "Up/Down/PgUp/PgDn scroll  F1/Esc close"
		if page < len(lines) {
			footer = fmt.Sprintf("%d-%d/%d  Up/Down/PgUp/PgDn", start+1, min(len(lines), start+page), len(lines))
		}
		drawText(screen, x+2, y+boxHeight-2, boxWidth-4, footer, helpStyle)
	}
}

func renderConfirmation(screen tcell.Screen, width, height int, state shellState) {
	if width < 12 || height < 5 {
		return
	}
	boxWidth := min(width-2, 68)
	x, y := (width-boxWidth)/2, (height-5)/2
	drawDialogFrame(screen, x, y, boxWidth, 5)
	drawText(screen, x+2, y+1, boxWidth-4, "Unsaved changes", helpStyle)
	action := "open another file"
	if state.confirm == confirmQuit {
		action = "quit RapidGo"
	}
	drawText(screen, x+2, y+2, boxWidth-4, "Discard edits and "+action+"?", helpStyle)
	drawStyledText(screen, x+2, y+3, boxWidth-4, []textSegment{{"D", shortcutStyle}, {" Discard   ", helpStyle}, {"Esc", shortcutStyle}, {" Cancel", helpStyle}})
}

// renderRunChooser lists the runnable packages, naming what Enter will do.
func renderRunChooser(screen tcell.Screen, width, height int, chooser *runChooser) {
	if width < 20 || height < 7 {
		return
	}
	boxWidth := min(width-2, 56)
	boxHeight := min(height-2, len(chooser.targets)+5)
	x, y := (width-boxWidth)/2, (height-boxHeight)/2
	drawDialogFrame(screen, x, y, boxWidth, boxHeight)
	title, action := "Select run target", " Select  "
	if chooser.run {
		title, action = "Run which package?", " Run  "
	}
	drawText(screen, x+2, y+1, boxWidth-4, title, helpStyle)
	rows := boxHeight - 4
	first := max(0, min(chooser.index-rows+1, len(chooser.targets)-rows))
	for offset := range rows {
		index := first + offset
		if index >= len(chooser.targets) {
			break
		}
		style := helpStyle
		if index == chooser.index {
			style = menuActiveStyle
			for col := x + 1; col < x+boxWidth-1; col++ {
				screen.SetContent(col, y+2+offset, ' ', nil, style)
			}
		}
		drawText(screen, x+2, y+2+offset, boxWidth-4, chooser.targets[index], style)
	}
	drawStyledText(screen, x+2, y+boxHeight-2, boxWidth-4, []textSegment{
		{"Up/Down", shortcutStyle},
		{" Move  ", helpStyle},
		{"Enter", shortcutStyle},
		{action, helpStyle},
		{"Esc", shortcutStyle},
		{" Cancel", helpStyle},
	})
}

// drawDialogFrame fills a light-gray dialog box and draws its border.
func drawDialogFrame(screen tcell.Screen, x, y, boxWidth, boxHeight int) {
	for row := y; row < y+boxHeight; row++ {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, row, ' ', nil, helpStyle)
		}
	}
	for col := x + 1; col < x+boxWidth-1; col++ {
		screen.SetContent(col, y, '─', nil, helpBorderStyle)
		screen.SetContent(col, y+boxHeight-1, '─', nil, helpBorderStyle)
	}
	for row := y + 1; row < y+boxHeight-1; row++ {
		screen.SetContent(x, row, '│', nil, helpBorderStyle)
		screen.SetContent(x+boxWidth-1, row, '│', nil, helpBorderStyle)
	}
	screen.SetContent(x, y, '┌', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y, '┐', nil, helpBorderStyle)
	screen.SetContent(x, y+boxHeight-1, '└', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y+boxHeight-1, '┘', nil, helpBorderStyle)
}

func drawPane(screen tcell.Screen, area rectangle, title, placeholder string) {
	drawFrame(screen, area, title, false)
	if area.width >= 4 && area.height > 2 {
		drawText(screen, area.x+2, area.y+1, area.width-4, placeholder, baseStyle)
	}
}

func drawFrame(screen tcell.Screen, area rectangle, title string, active bool) {
	if area.width < 4 || area.height < 2 {
		drawText(screen, area.x, area.y, area.width, title, titleStyle)
		return
	}
	left, right := area.x, area.x+area.width-1
	top, bottom := area.y, area.y+area.height-1
	horizontal, vertical := '─', '│'
	topLeft, topRight, bottomLeft, bottomRight := '┌', '┐', '└', '┘'
	if active {
		horizontal, vertical = '═', '║'
		topLeft, topRight, bottomLeft, bottomRight = '╔', '╗', '╚', '╝'
	}
	for col := left + 1; col < right; col++ {
		screen.SetContent(col, top, horizontal, nil, frameStyle)
		screen.SetContent(col, bottom, horizontal, nil, frameStyle)
	}
	for row := top + 1; row < bottom; row++ {
		screen.SetContent(left, row, vertical, nil, frameStyle)
		screen.SetContent(right, row, vertical, nil, frameStyle)
	}
	screen.SetContent(left, top, topLeft, nil, frameStyle)
	screen.SetContent(right, top, topRight, nil, frameStyle)
	screen.SetContent(left, bottom, bottomLeft, nil, frameStyle)
	screen.SetContent(right, bottom, bottomRight, nil, frameStyle)
	drawText(screen, left+2, top, area.width-4, " "+title+" ", titleStyle)
}

func shadowRow(screen tcell.Screen, x, y, length, width int) {
	for col := x; col < x+length && col < width; col++ {
		screen.SetContent(col, y, ' ', nil, shadowStyle)
	}
}

func fillRow(screen tcell.Screen, y, width int, style tcell.Style) {
	for x := 0; x < width; x++ {
		screen.SetContent(x, y, ' ', nil, style)
	}
}

func drawStyledText(screen tcell.Screen, x, y, available int, segments []textSegment) {
	for _, segment := range segments {
		if available <= 0 {
			return
		}
		drawText(screen, x, y, available, segment.text, segment.style)
		width := uniseg.StringWidth(segment.text)
		if width > available {
			return
		}
		x += width
		available -= width
	}
}

// drawText clips at grapheme boundaries so a wide character never spills.
func drawText(screen tcell.Screen, x, y, available int, value string, style tcell.Style) {
	if available <= 0 {
		return
	}
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		cluster := graphemes.Str()
		cellWidth := uniseg.StringWidth(cluster)
		if cellWidth > available {
			return
		}
		if cellWidth <= 0 {
			continue
		}
		_, _ = screen.Put(x, y, cluster, style)
		x += cellWidth
		available -= cellWidth
	}
}

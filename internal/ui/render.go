package ui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
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
)

type textSegment struct {
	text  string
	style tcell.Style
}

type rectangle struct {
	x, y, width, height int
}

type layout struct {
	menu, status            rectangle
	project, editor, output rectangle
	projectVisible          bool
}

func calculateLayout(width, height int) layout {
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
	if contentHeight <= 0 {
		return result
	}

	outputHeight := 0
	if contentHeight >= 2 {
		outputHeight = contentHeight / 4
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
	view := calculateLayout(width, height)
	screen.SetStyle(baseStyle)
	screen.Fill(' ', baseStyle)
	screen.HideCursor()
	if width <= 0 || height <= 0 {
		return
	}

	renderMenuBar(screen, width, view.menu.y, state)
	renderPanes(screen, view, state)
	renderStatusBar(screen, view, state)
	if state.menuOpen && height > 2 {
		renderMenu(screen, width, height, state.menuIndex, state.menuItem)
	}
	if state.helpVisible {
		renderHelp(screen, width, height)
	}
	if state.confirm != confirmNone {
		renderConfirmation(screen, width, height, state)
	}
	if state.helpVisible || state.menuOpen || state.confirm != confirmNone {
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
	if view.projectVisible || state.focus == focusTree {
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
	if view.editor.height > 0 && (view.projectVisible || state.focus == focusEditor) {
		renderDocument(screen, view.editor, state)
	}
	if view.output.height > 0 {
		message := state.message
		if message == "" {
			message = "No output yet"
		}
		drawPane(screen, view.output, "OUTPUT", message)
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

func renderHelp(screen tcell.Screen, width, height int) {
	if width < 16 || height < 5 {
		return
	}
	boxWidth := width - 2
	if boxWidth > 48 {
		boxWidth = 48
	}
	boxHeight := 12
	if boxHeight > height-2 {
		boxHeight = height - 2
	}
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
	if boxHeight >= 4 {
		drawStyledText(screen, x+2, y+2, boxWidth-3, []textSegment{{"F3", shortcutStyle}, {" Tree  ", helpStyle}, {"F6 / Ctrl+F6", shortcutStyle}, {" Next pane  ", helpStyle}, {"Enter", shortcutStyle}, {" Open", helpStyle}})
	}
	if boxHeight >= 5 {
		drawStyledText(screen, x+2, y+3, boxWidth-3, []textSegment{{"Tree: arrows", shortcutStyle}, {" Navigate/fold  ", helpStyle}, {"Enter", shortcutStyle}, {" Open", helpStyle}})
	}
	if boxHeight >= 6 {
		drawStyledText(screen, x+2, y+4, boxWidth-3, []textSegment{{"Editor: arrows/Home/End/PgUp/PgDn", shortcutStyle}, {" Move", helpStyle}})
	}
	if boxHeight >= 7 {
		drawStyledText(screen, x+2, y+5, boxWidth-3, []textSegment{{"Shift+move", shortcutStyle}, {" Select  ", helpStyle}, {"Tab/Shift+Tab", shortcutStyle}, {" Indent", helpStyle}})
	}
	if boxHeight >= 8 {
		drawStyledText(screen, x+2, y+6, boxWidth-3, []textSegment{{"Ctrl+A/Z/Y", shortcutStyle}, {" Select all / Undo / Redo", helpStyle}})
	}
	if boxHeight >= 9 {
		drawStyledText(screen, x+2, y+7, boxWidth-3, []textSegment{{"F2", shortcutStyle}, {" Save  ", helpStyle}, {"Ctrl+F", shortcutStyle}, {" Find  ", helpStyle}, {"Ctrl+G", shortcutStyle}, {" Next", helpStyle}})
	}
	if boxHeight >= 10 {
		drawStyledText(screen, x+2, y+8, boxWidth-3, []textSegment{{"F10 / Alt+F / Alt+S / Alt+H", shortcutStyle}, {" Menu", helpStyle}})
	}
	if boxHeight >= 11 {
		drawStyledText(screen, x+2, y+9, boxWidth-3, []textSegment{{"F1 / Esc", shortcutStyle}, {" Close help  ", helpStyle}, {"Ctrl+Q", shortcutStyle}, {" Quit", helpStyle}})
	}
}

func renderConfirmation(screen tcell.Screen, width, height int, state shellState) {
	if width < 12 || height < 5 {
		return
	}
	boxWidth := min(width-2, 68)
	x, y := (width-boxWidth)/2, (height-5)/2
	for row := y; row < y+5; row++ {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, row, ' ', nil, helpStyle)
		}
	}
	for col := x + 1; col < x+boxWidth-1; col++ {
		screen.SetContent(col, y, '─', nil, helpBorderStyle)
		screen.SetContent(col, y+4, '─', nil, helpBorderStyle)
	}
	for row := y + 1; row < y+4; row++ {
		screen.SetContent(x, row, '│', nil, helpBorderStyle)
		screen.SetContent(x+boxWidth-1, row, '│', nil, helpBorderStyle)
	}
	screen.SetContent(x, y, '┌', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y, '┐', nil, helpBorderStyle)
	screen.SetContent(x, y+4, '└', nil, helpBorderStyle)
	screen.SetContent(x+boxWidth-1, y+4, '┘', nil, helpBorderStyle)
	drawText(screen, x+2, y+1, boxWidth-4, "Unsaved changes", helpStyle)
	action := "open another file"
	if state.confirm == confirmQuit {
		action = "quit RapidGo"
	}
	drawText(screen, x+2, y+2, boxWidth-4, "Discard edits and "+action+"?", helpStyle)
	drawStyledText(screen, x+2, y+3, boxWidth-4, []textSegment{{"D", shortcutStyle}, {" Discard   ", helpStyle}, {"Esc", shortcutStyle}, {" Cancel", helpStyle}})
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

// drawText clips at grapheme boundaries so wide characters never spill into
// the next pane or past the terminal edge.
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

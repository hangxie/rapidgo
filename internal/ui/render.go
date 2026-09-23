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

	fillRow(screen, view.menu.y, width, barStyle)
	for index, label := range menuLabels {
		style := barStyle
		mnemonicStyle := shortcutStyle
		if state.menuOpen && state.menuIndex == index {
			style = menuActiveStyle
			mnemonicStyle = menuActiveMnemonicStyle
		}
		drawText(screen, menuX[index], view.menu.y, width-menuX[index], " "+label+" ", style)
		drawText(screen, menuX[index]+1, view.menu.y, width-menuX[index]-1, label[:1], mnemonicStyle)
	}
	if view.projectVisible {
		drawPane(screen, view.project, "PROJECT", "Files coming next")
	}
	if view.editor.height > 0 {
		drawPane(screen, view.editor, "EDITOR  "+state.projectRoot, "Editor coming next")
	}
	if view.output.height > 0 {
		drawPane(screen, view.output, "OUTPUT", "No output yet")
	}
	if view.status.height > 0 {
		fillRow(screen, view.status.y, width, barStyle)
		status := []textSegment{{"F10", shortcutStyle}, {" Menu  ", barStyle}, {"F1", shortcutStyle}, {" Help  ", barStyle}, {"Ctrl+Q", shortcutStyle}, {" Quit", barStyle}}
		if width < 34 {
			status = []textSegment{{"F10", shortcutStyle}, {" Menu  ", barStyle}, {"F1", shortcutStyle}, {" Help  ", barStyle}, {"^Q", shortcutStyle}, {" Quit", barStyle}}
		}
		if width < 27 {
			status = []textSegment{{"F10", shortcutStyle}, {"  ", barStyle}, {"F1", shortcutStyle}, {"  ", barStyle}, {"^Q", shortcutStyle}, {" Quit", barStyle}}
		}
		if !view.projectVisible {
			status = append(status, textSegment{"  " + filepath.Base(state.projectRoot), barStyle})
		}
		drawStyledText(screen, 1, view.status.y, width-1, status)
	}
	if state.menuOpen && height > 2 {
		renderMenu(screen, width, height, state.menuIndex)
	}
	if state.helpVisible {
		renderHelp(screen, width, height)
	}
	screen.Show()
}

func renderMenu(screen tcell.Screen, width, height, index int) {
	x := menuX[index]
	if x >= width {
		return
	}
	boxWidth := 21
	if boxWidth > width-x {
		boxWidth = width - x
	}
	if height < 5 || boxWidth < 4 {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, 1, ' ', nil, barStyle)
		}
		drawText(screen, x+1, 1, boxWidth-1, menuActionLabels[index], barStyle)
		return
	}
	if height > 5 {
		shadowRow(screen, x+2, 4, boxWidth-1, width)
	}
	for row := 1; row <= 3; row++ {
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
		screen.SetContent(col, 2, ' ', nil, menuActiveStyle)
		screen.SetContent(col, 3, '─', nil, helpBorderStyle)
	}
	screen.SetContent(x, 1, '┌', nil, helpBorderStyle)
	screen.SetContent(right, 1, '┐', nil, helpBorderStyle)
	screen.SetContent(x, 2, '│', nil, helpBorderStyle)
	screen.SetContent(right, 2, '│', nil, helpBorderStyle)
	screen.SetContent(x, 3, '└', nil, helpBorderStyle)
	screen.SetContent(right, 3, '┘', nil, helpBorderStyle)
	drawText(screen, x+2, 2, boxWidth-3, menuActionLabels[index], menuActiveStyle)
	drawText(screen, x+13, 2, boxWidth-14, menuActionShortcuts[index], menuActiveMnemonicStyle)
}

func renderHelp(screen tcell.Screen, width, height int) {
	if width < 16 || height < 5 {
		return
	}
	boxWidth := width - 2
	if boxWidth > 48 {
		boxWidth = 48
	}
	boxHeight := 7
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
	if boxHeight >= 5 {
		drawStyledText(screen, x+2, y+3, boxWidth-3, []textSegment{{"F1 / Esc", shortcutStyle}, {"  Close help", helpStyle}})
	}
	if boxHeight >= 6 {
		drawStyledText(screen, x+2, y+4, boxWidth-3, []textSegment{{"Ctrl+Q", shortcutStyle}, {"    Quit RapidGo", helpStyle}})
	}
	if boxHeight >= 7 {
		drawStyledText(screen, x+2, y+5, boxWidth-3, []textSegment{{"F10 / Alt+F / Alt+H", shortcutStyle}, {"  Menu", helpStyle}})
	}
}

// All panes use double borders until pane focus exists. Once focus is modeled,
// reserve the double border for the active pane and draw inactive panes singly.
func drawPane(screen tcell.Screen, area rectangle, title, placeholder string) {
	if area.width < 4 || area.height < 2 {
		drawText(screen, area.x, area.y, area.width, title, titleStyle)
		return
	}
	left, right := area.x, area.x+area.width-1
	top, bottom := area.y, area.y+area.height-1
	for col := left + 1; col < right; col++ {
		screen.SetContent(col, top, '═', nil, frameStyle)
		screen.SetContent(col, bottom, '═', nil, frameStyle)
	}
	for row := top + 1; row < bottom; row++ {
		screen.SetContent(left, row, '║', nil, frameStyle)
		screen.SetContent(right, row, '║', nil, frameStyle)
	}
	screen.SetContent(left, top, '╔', nil, frameStyle)
	screen.SetContent(right, top, '╗', nil, frameStyle)
	screen.SetContent(left, bottom, '╚', nil, frameStyle)
	screen.SetContent(right, bottom, '╝', nil, frameStyle)
	drawText(screen, left+2, top, area.width-4, " "+title+" ", titleStyle)
	if area.height > 2 {
		drawText(screen, left+2, top+1, area.width-4, placeholder, baseStyle)
	}
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

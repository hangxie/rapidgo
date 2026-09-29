package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

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

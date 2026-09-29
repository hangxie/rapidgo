package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

func hoverRows(value string, width int) []string {
	var rows []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\t", "    "), "\n") {
		row, cells := "", 0
		clusters := uniseg.NewGraphemes(line)
		for clusters.Next() {
			cluster := clusters.Str()
			size := uniseg.StringWidth(cluster)
			if size > width {
				cluster, size = "�", 1
			}
			if cells > 0 && cells+size > width {
				rows = append(rows, row)
				row, cells = "", 0
			}
			row += cluster
			cells += size
		}
		rows = append(rows, row)
	}
	return rows
}

func renderHover(screen tcell.Screen, width, height int, state shellState) {
	if width < 16 || height < 5 {
		return
	}
	boxWidth := min(width-2, 72)
	rows := hoverRows(state.hoverText, boxWidth-4)
	boxHeight := min(height, 20, max(5, len(rows)+4))
	x, y := (width-boxWidth)/2, (height-boxHeight)/2
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
	drawText(screen, x+2, y+1, boxWidth-4, "gopls hover", helpStyle)
	page := boxHeight - 4
	start := min(state.hoverScroll, max(0, len(rows)-page))
	for index, line := range rows[start:min(len(rows), start+page)] {
		drawText(screen, x+2, y+2+index, boxWidth-4, line, helpStyle)
	}
	footer := "Esc close  Up/Down scroll"
	if len(rows) > page {
		footer = fmt.Sprintf("Esc close  %d-%d/%d  PgUp/PgDn scroll", start+1, min(start+page, len(rows)), len(rows))
	}
	drawText(screen, x+2, y+boxHeight-2, boxWidth-4, footer, helpStyle)
}

package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func completionFits(width, height int) bool { return width >= 24 && height >= 7 }

func completionPage(screen tcell.Screen) int {
	_, height := screen.Size()
	return max(1, min(height-2, 16)-4)
}

func renderCompletion(screen tcell.Screen, width, height int, state shellState) {
	if !completionFits(width, height) {
		return
	}
	boxWidth := min(width-2, 72)
	boxHeight := min(height-2, 16, len(state.completionItems)+4)
	x, y := (width-boxWidth)/2, (height-boxHeight)/2
	drawDialogFrame(screen, x, y, boxWidth, boxHeight)
	drawText(screen, x+2, y+1, boxWidth-4, i18n.Text("msg_gopls_completion"), helpStyle)
	page := boxHeight - 4
	start := min(state.completionScroll, max(0, len(state.completionItems)-page))
	for index, item := range state.completionItems[start:min(len(state.completionItems), start+page)] {
		style := helpStyle
		if start+index == state.completionSelected {
			style = menuActiveStyle
			for col := x + 1; col < x+boxWidth-1; col++ {
				screen.SetContent(col, y+2+index, ' ', nil, style)
			}
		}
		label := item.Label
		if item.Detail != "" {
			label += "  " + item.Detail
		}
		drawText(screen, x+2, y+2+index, boxWidth-4, label, style)
	}
	footer := i18n.Format("msg_d_d_enter_insert_esc_cancel", state.completionSelected+1, len(state.completionItems))
	drawText(screen, x+2, y+boxHeight-2, boxWidth-4, footer, helpStyle)
}

package ui

import "github.com/gdamore/tcell/v2"

func renderConfirmation(screen tcell.Screen, width, height int, state shellState) {
	if width < 12 || height < 5 {
		return
	}
	boxWidth := min(width-2, 68)
	x, y := (width-boxWidth)/2, (height-5)/2
	drawDialogFrame(screen, x, y, boxWidth, 5)
	drawText(screen, x+2, y+1, boxWidth-4, "Unsaved changes", helpStyle)
	action := "open another file"
	switch state.confirm {
	case confirmQuit:
		action = "quit RapidGo"
	case confirmCloseWindow:
		action = "close this window"
	}
	drawText(screen, x+2, y+2, boxWidth-4, "Discard edits and "+action+"?", helpStyle)
	drawStyledText(screen, x+2, y+3, boxWidth-4, []textSegment{{"D", shortcutStyle}, {" Discard   ", helpStyle}, {"Esc", shortcutStyle}, {" Cancel", helpStyle}})
}

// renderRunChooser lists runnable targets, naming what Enter will do.
func renderRunChooser(screen tcell.Screen, width, height int, chooser *runChooser) {
	if width < 20 || height < 7 {
		return
	}
	boxWidth := min(width-2, 56)
	boxHeight := min(height-2, len(chooser.targets)+5)
	x, y := (width-boxWidth)/2, (height-boxHeight)/2
	drawDialogFrame(screen, x, y, boxWidth, boxHeight)
	title, action := "Set default run package", " Select  "
	if chooser.run {
		title, action = "Run which package?", " Run  "
		if chooser.terminal {
			title, action = "Run in terminal: which package?", " Open  "
		}
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

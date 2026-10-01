package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func renderConfirmation(screen tcell.Screen, width, height int, state shellState) {
	if width < 12 || height < 5 {
		return
	}
	boxWidth := min(width-2, 68)
	x, y := (width-boxWidth)/2, (height-5)/2
	drawDialogFrame(screen, x, y, boxWidth, 5)
	drawText(screen, x+2, y+1, boxWidth-4, i18n.Text("msg_unsaved_changes"), helpStyle)
	action := i18n.Text("msg_open_another_file")
	switch state.confirm {
	case confirmQuit:
		action = i18n.Text("msg_quit_rapidgo")
	case confirmCloseWindow:
		action = i18n.Text("msg_close_this_window")
	}
	drawText(screen, x+2, y+2, boxWidth-4, i18n.Format("msg_discard_edits_and_s", action), helpStyle)
	drawStyledText(screen, x+2, y+3, boxWidth-4, []textSegment{{i18n.Text("msg_d"), shortcutStyle}, {i18n.Text("msg_discard"), helpStyle}, {i18n.Text("msg_esc"), shortcutStyle}, {i18n.Text("msg_cancel"), helpStyle}})
}

// renderRunChooser lists runnable targets, naming what Enter will do.
func renderRunChooser(screen tcell.Screen, width, height int, chooser *runChooser) {
	if width < 20 || height < 7 {
		return
	}
	title, action := i18n.Text("msg_set_default_run_package"), i18n.Text("msg_select")
	if chooser.run {
		title, action = i18n.Text("msg_run_which_package"), i18n.Text("msg_run_2")
		if chooser.terminal {
			title, action = i18n.Text("msg_run_in_terminal_which_package"), i18n.Text("msg_open")
		}
	}
	renderChoiceDialog(screen, width, height, title, action, chooser.targets, chooser.index)
}

func renderChoiceDialog(screen tcell.Screen, width, height int, title, action string, labels []string, selected int) {
	if width < 12 || height < 5 {
		return
	}
	boxWidth := min(width-2, 56)
	boxHeight := min(height-2, len(labels)+5)
	boxHeight = max(5, boxHeight)
	x, y := (width-boxWidth)/2, (height-boxHeight)/2
	drawDialogFrame(screen, x, y, boxWidth, boxHeight)
	drawText(screen, x+2, y+1, boxWidth-4, title, helpStyle)
	rows := boxHeight - 4
	first := max(0, min(selected-rows+1, len(labels)-rows))
	for offset := range rows {
		index := first + offset
		if index >= len(labels) {
			break
		}
		style := helpStyle
		if index == selected {
			style = menuActiveStyle
			for col := x + 1; col < x+boxWidth-1; col++ {
				screen.SetContent(col, y+2+offset, ' ', nil, style)
			}
		}
		drawText(screen, x+2, y+2+offset, boxWidth-4, labels[index], style)
	}
	drawStyledText(screen, x+2, y+boxHeight-2, boxWidth-4, []textSegment{
		{i18n.Text("msg_up_down"), shortcutStyle},
		{i18n.Text("msg_move"), helpStyle},
		{i18n.Text("msg_enter"), shortcutStyle},
		{action, helpStyle},
		{i18n.Text("msg_esc"), shortcutStyle},
		{i18n.Text("msg_cancel"), helpStyle},
	})
}

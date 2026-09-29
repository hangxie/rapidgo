package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

func renderMenu(screen tcell.Screen, width, height, index, selected int) {
	renderMenuActions(screen, width, height, menuX[index], menuActions[index], selected)
}

func renderMenuActions(screen tcell.Screen, width, height, x int, actions []menuAction, selected int) {
	renderMenuActionsAt(screen, width, height, x, 1, actions, selected)
}

func renderMenuActionsAt(screen tcell.Screen, width, height, x, y int, actions []menuAction, selected int) {
	boxWidth := menuWidth(actions)
	x = min(x, max(0, width-boxWidth))
	if x >= width {
		return
	}
	if boxWidth > width-x {
		boxWidth = width - x
	}
	boxHeight := len(actions) + 2
	y = min(y, max(1, height-boxHeight-2))
	if height < y+boxHeight+1 || boxWidth < 4 {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, y, ' ', nil, barStyle)
		}
		drawText(screen, x+1, y, boxWidth-1, actions[selected].label, barStyle)
		return
	}
	bottom := y + boxHeight - 1
	if bottom+1 < height-1 {
		shadowRow(screen, x+2, bottom+1, boxWidth-1, width)
	}
	for row := y; row <= bottom; row++ {
		for col := x; col < x+boxWidth; col++ {
			screen.SetContent(col, row, ' ', nil, barStyle)
		}
		if row > y && x+boxWidth < width {
			screen.SetContent(x+boxWidth, row, ' ', nil, shadowStyle)
		}
	}
	right := x + boxWidth - 1
	for col := x + 1; col < right; col++ {
		screen.SetContent(col, y, '─', nil, helpBorderStyle)
		screen.SetContent(col, bottom, '─', nil, helpBorderStyle)
	}
	screen.SetContent(x, y, '┌', nil, helpBorderStyle)
	screen.SetContent(right, y, '┐', nil, helpBorderStyle)
	shortcutX := x + menuShortcutOffset(actions)
	for item, action := range actions {
		row := y + item + 1
		style, shortcut := barStyle, shortcutStyle
		if item == selected {
			style, shortcut = menuActiveStyle, menuActiveMnemonicStyle
		}
		for col := x + 1; col < right; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
		screen.SetContent(x, row, '│', nil, helpBorderStyle)
		screen.SetContent(right, row, '│', nil, helpBorderStyle)
		labelEnd := shortcutX - 1
		if action.shortcut == "" {
			labelEnd = right
		}
		drawText(screen, x+2, row, max(0, min(right, labelEnd)-(x+2)), action.label, style)
		if action.shortcut != "" && shortcutX+uniseg.StringWidth(action.shortcut) < right {
			drawText(screen, shortcutX, row, right-shortcutX, action.shortcut, shortcut)
		}
	}
	screen.SetContent(x, bottom, '└', nil, helpBorderStyle)
	screen.SetContent(right, bottom, '┘', nil, helpBorderStyle)
}

// menuWidth fits labels and shortcuts inside the dropdown borders.
func menuWidth(actions []menuAction) int {
	labelWidth, shortcutWidth := 0, 0
	for _, action := range actions {
		labelWidth = max(labelWidth, uniseg.StringWidth(action.label))
		shortcutWidth = max(shortcutWidth, uniseg.StringWidth(action.shortcut))
	}
	width := max(21, labelWidth+4)
	if shortcutWidth > 0 {
		width = max(width, max(13, labelWidth+3)+shortcutWidth+2)
	}
	return width
}

// menuShortcutOffset places every shortcut after the widest label.
func menuShortcutOffset(actions []menuAction) int {
	labelWidth := 0
	for _, action := range actions {
		labelWidth = max(labelWidth, uniseg.StringWidth(action.label))
	}
	return max(13, labelWidth+3)
}

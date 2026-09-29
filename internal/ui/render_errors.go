package ui

import (
	"fmt"
	"path/filepath"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/diagnostic"
)

func renderErrors(screen tcell.Screen, area rectangle, state shellState) {
	items := state.errorItems()
	rows := max(1, area.height-2)
	selected := min(state.errorSelected, max(0, len(items)-1))
	top := min(state.errorScroll, selected)
	top = max(0, min(top, max(0, len(items)-rows)))
	if selected >= top+rows {
		top = selected - rows + 1
	}
	title := fmt.Sprintf("ERRORS  %d diagnostic(s)", len(items))
	if len(items) > rows {
		title += fmt.Sprintf("  %d-%d/%d", top+1, min(len(items), top+rows), len(items))
	}
	drawFrame(screen, area, title, state.focus == focusOutput)
	if area.width < 4 || area.height < 3 {
		return
	}
	if len(items) == 0 {
		drawText(screen, area.x+2, area.y+1, area.width-4, "No diagnostics", baseStyle)
		return
	}
	for index := top; index < min(len(items), top+rows); index++ {
		item := items[index]
		style := baseStyle
		if state.focus == focusOutput && index == selected {
			style = treeSelectedStyle
			for col := area.x + 1; col < area.x+area.width-1; col++ {
				screen.SetContent(col, area.y+1+index-top, ' ', nil, style)
			}
		} else if item.Severity == diagnostic.Error {
			style = outputErrorStyle
		}
		path := item.Path
		if relative, err := filepath.Rel(state.projectRoot, path); err == nil {
			path = relative
		}
		column := item.Column
		if item.Source == "gopls" {
			column = item.utf16Column + 1
		}
		location := fmt.Sprintf("%s:%d", path, item.Line)
		if column > 0 {
			location += fmt.Sprintf(":%d", column)
		}
		label := fmt.Sprintf("%s [%s] %s", location, item.Severity, item.Message)
		drawText(screen, area.x+1, area.y+1+index-top, area.width-2, label, style)
	}
}

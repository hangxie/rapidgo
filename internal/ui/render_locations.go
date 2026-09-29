package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/gopls"
)

func renderLocations(screen tcell.Screen, area rectangle, state shellState) {
	rows := max(1, area.height-2)
	selected := min(state.locationSelected, max(0, len(state.locations)-1))
	top := max(0, min(state.locationScroll, selected, max(0, len(state.locations)-rows)))
	if selected >= top+rows {
		top = selected - rows + 1
	}
	title := fmt.Sprintf("%s  %d location(s)", strings.ToUpper(state.locationKind.name()), len(state.locations))
	if len(state.locations) > rows {
		title += fmt.Sprintf("  %d-%d/%d", top+1, min(len(state.locations), top+rows), len(state.locations))
	}
	drawFrame(screen, area, title, state.focus == focusOutput)
	if area.width < 4 || area.height < 3 {
		return
	}
	var openPath string
	var lines []string
	if state.document != nil && state.buffer != nil {
		openPath = state.document.Path
		lines = state.buffer.Lines()
	}
	for index := top; index < min(len(state.locations), top+rows); index++ {
		location := state.locations[index]
		style := baseStyle
		if state.focus == focusOutput && index == selected {
			style = treeSelectedStyle
			for col := area.x + 1; col < area.x+area.width-1; col++ {
				screen.SetContent(col, area.y+1+index-top, ' ', nil, style)
			}
		}
		drawText(screen, area.x+1, area.y+1+index-top, area.width-2, locationLabel(state.projectRoot, openPath, lines, location), style)
	}
}

func locationLabel(root, openPath string, lines []string, location gopls.Location) string {
	path := location.Path
	if relative, err := filepath.Rel(root, path); err == nil {
		path = relative
	}
	label := fmt.Sprintf("%s:%d", path, location.Position.Line+1)
	if openPath != location.Path {
		return label
	}
	if location.Position.Line < 0 || location.Position.Line >= len(lines) {
		return label
	}
	line := lines[location.Position.Line]
	column, _ := graphemeColumn(line, utf16ByteColumn(line, location.Position.Character), false)
	return fmt.Sprintf("%s:%d", label, column+1)
}

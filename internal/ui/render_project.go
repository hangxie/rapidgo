package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/project"
)

func renderTree(screen tcell.Screen, area rectangle, state shellState) {
	title := "PROJECT"
	if state.tree.Root.Module {
		title += " [module]"
	}
	drawFrame(screen, area, title, state.focus == focusTree)
	if area.width < 4 || area.height < 3 {
		return
	}
	items := state.tree.Visible()
	if state.treeScroll >= len(items) {
		return
	}
	innerWidth := area.width - 2
	for row := 0; row < area.height-2 && state.treeScroll+row < len(items); row++ {
		index := state.treeScroll + row
		y := area.y + 1 + row
		style := baseStyle
		if index == state.selected {
			style = treeSelectedStyle
			for x := area.x + 1; x < area.x+area.width-1; x++ {
				screen.SetContent(x, y, ' ', nil, style)
			}
		}
		item := items[index]
		indent := min(item.Depth*2, max(0, innerWidth-8))
		label := strings.Repeat(" ", indent) + treeLabel(item.Node)
		drawText(screen, area.x+1, y, innerWidth, label, style)
	}
}

func treeLabel(node *project.Node) string {
	prefix := "    "
	if node.Symlink {
		prefix = "[@] "
	} else if node.IsDir {
		switch {
		case node.Loading:
			prefix = "[~] "
		case node.Error != nil:
			prefix = "[!] "
		case node.Expanded:
			prefix = "[-] "
		default:
			prefix = "[+] "
		}
	}
	label := prefix + node.Name
	if node.Module {
		label += " [mod]"
	}
	return label
}

func renderDocument(screen tcell.Screen, area rectangle, state shellState) {
	title := "EDITOR  " + state.projectRoot
	if state.document != nil {
		title = "PREVIEW  " + state.document.Path
	}
	drawFrame(screen, area, title, state.focus == focusPreview)
	if area.width < 4 || area.height < 3 {
		return
	}
	if state.document == nil {
		message := "Editor coming next"
		if state.tree != nil {
			message = "Select a file and press Enter (read-only preview)"
		}
		drawText(screen, area.x+2, area.y+1, area.width-4, message, baseStyle)
		return
	}
	innerWidth := area.width - 2
	for row := 0; row < area.height-2 && state.fileScroll+row < len(state.document.Lines); row++ {
		index := state.fileScroll + row
		line := strings.ReplaceAll(state.document.Lines[index], "\t", "    ")
		if innerWidth >= 12 {
			line = fmt.Sprintf("%4d ", index+1) + line
		}
		drawText(screen, area.x+1, area.y+1+row, innerWidth, line, baseStyle)
	}
}

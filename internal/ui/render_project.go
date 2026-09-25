package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/editor"
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
		title = "EDITOR  " + state.document.Path
		if state.buffer != nil && state.buffer.Dirty() {
			title = "EDITOR * " + state.document.Path
		}
	}
	drawFrame(screen, area, title, state.focus == focusEditor)
	if area.width < 4 || area.height < 3 {
		return
	}
	if state.document == nil || state.buffer == nil {
		message := "Editor coming next"
		if state.tree != nil {
			message = "Select a file and press Enter"
		}
		drawText(screen, area.x+2, area.y+1, area.width-4, message, baseStyle)
		return
	}
	lines := state.buffer.Lines()
	gutter := editorGutterWidth(area, len(lines))
	textWidth := editorTextWidth(area, len(lines))
	start, end, selected := state.buffer.Selection()
	for row := 0; row < area.height-2 && state.fileScroll+row < len(lines); row++ {
		index := state.fileScroll + row
		y := area.y + 1 + row
		if gutter > 0 {
			drawText(screen, area.x+1, y, gutter, fmt.Sprintf("%*d ", gutter-1, index+1), baseStyle)
		}
		drawEditorLine(screen, area.x+1+gutter, y, textWidth, lines[index], state.fileColumn, index, start, end, selected)
	}
	if state.focus == focusEditor && !state.menuOpen && !state.helpVisible && state.confirm == confirmNone {
		cursor := state.buffer.Cursor()
		if cursor.Line >= state.fileScroll && cursor.Line < state.fileScroll+area.height-2 {
			column := visualColumn(lines[cursor.Line], cursor.Column) - state.fileColumn
			if column >= 0 && column < textWidth {
				screen.ShowCursor(area.x+1+gutter+column, area.y+1+cursor.Line-state.fileScroll)
			}
		}
	}
}

func editorGutterWidth(area rectangle, lineCount int) int {
	innerWidth := area.width - 2
	gutter := max(5, len(strconv.Itoa(max(1, lineCount)))+1)
	if innerWidth >= gutter+7 {
		return gutter
	}
	return 0
}

func editorTextWidth(area rectangle, lineCount int) int {
	return max(0, area.width-2-editorGutterWidth(area, lineCount))
}

func visualColumn(line string, column int) int {
	width := 0
	clusters := uniseg.NewGraphemes(line)
	for index := 0; index < column && clusters.Next(); index++ {
		_, cells := editorCluster(clusters.Str())
		width += cells
	}
	return width
}

func editorCluster(cluster string) (string, int) {
	if cluster == "\t" {
		return "    ", 4
	}
	if strings.ContainsFunc(cluster, unicode.IsControl) {
		return "�", 1
	}
	width := uniseg.StringWidth(cluster)
	if width <= 0 {
		return "�", 1
	}
	return cluster, width
}

func drawEditorLine(screen tcell.Screen, x, y, width int, line string, scroll, lineIndex int, start, end editor.Position, selected bool) {
	if width <= 0 {
		return
	}
	column, cells := 0, 0
	clusters := uniseg.NewGraphemes(line)
	for clusters.Next() {
		display, size := editorCluster(clusters.Str())
		if cells >= scroll+width {
			break
		}
		if cells >= scroll && cells+size <= scroll+width {
			style := baseStyle
			position := editor.Position{Line: lineIndex, Column: column}
			if selected && !positionBefore(position, start) && positionBefore(position, end) {
				style = editorSelectionStyle
			}
			drawText(screen, x+cells-scroll, y, size, display, style)
		}
		cells += size
		column++
	}
}

func positionBefore(left, right editor.Position) bool {
	return left.Line < right.Line || (left.Line == right.Line && left.Column < right.Column)
}

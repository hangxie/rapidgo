package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// helpEntry pairs a shortcut with its action.
type helpEntry struct{ shortcut, action string }

// helpEntries lists keyboard actions.
func helpEntries() []helpEntry {
	return []helpEntry{
		{"F3", "Focus tree"},
		{"Alt+F3", "Close active editor window"},
		{"F6 / Shift+F6", "Next / previous editor window"},
		{"Alt+0", "List open editor windows"},
		{"Ctrl+F6", "Next pane"},
		{"Window menu", "New view, tile, cascade, zoom"},
		{"F5", "Zoom / restore active window"},
		{"Ctrl+F5", "Size / move active window"},
		{"Window arrows", "Move; Shift+arrows resize"},
		{"Window Enter / Esc", "Finish move / resize"},
		{"Tree Up/Down", "Select item"},
		{"Tree Left/Right", "Fold / expand"},
		{"Tree Enter", "Open item"},
		{"Tree Home/End", "First / last item"},
		{"Tree PgUp/PgDn", "Move by page"},
		{"Editor arrows", "Move cursor"},
		{"Editor Home/End", "Line ends"},
		{"Editor PgUp/PgDn", "Move by page"},
		{"Ctrl+Home/End", "File ends"},
		{"Shift+movement", "Extend selection"},
		{"Tab / Shift+Tab", "Indent / unindent"},
		{"Enter", "New line"},
		{"Backspace / Delete", "Erase"},
		{"Ctrl+A", "Select all"},
		{"Ctrl+Z / Ctrl+Y", "Undo / redo"},
		{"F2", "Save"},
		{"Ctrl+F / Ctrl+G", "Find / next"},
		{"Alt+I", "Inspect symbol with gopls"},
		{"Ctrl+Space / Alt+C", "Complete symbol with gopls"},
		{"F12 / Alt+D", "Jump to Go definition"},
		{"Shift+F12 / Alt+R", "List Go references"},
		{"Locations arrows/Enter", "Select / jump to source"},
		{"Locations Esc", "Return to Output and editor"},
		{"Completion arrows", "Select a suggestion"},
		{"Completion Enter / Esc", "Insert / cancel suggestion"},
		{"Alt+E", "Toggle Errors / Output view"},
		{"Errors arrows/Enter", "Select / jump to diagnostic"},
		{"Errors Left/Right", "Return to Output"},
		{"Errors Esc", "Return to Output and editor"},
		{"Hover Esc / arrows", "Close / scroll symbol info"},
		{"Search Enter / Esc", "Find / cancel"},
		{"F9", "Build"},
		{"Ctrl+T", "Test"},
		{"Ctrl+F9", "Run in output pane (noninteractive)"},
		{"Alt+F9", "Run current file with helpers"},
		{"Build → Run in Terminal", "Run TUI in terminal"},
		{"Build → Run Options", "Set default package / arguments"},
		{"Ctrl+K", "Stop all Go jobs"},
		{"Output Up/Down", "Select line"},
		{"Output PgUp/PgDn", "Move by page"},
		{"Output Home/End", "First / latest line"},
		{"Output Left/Right", "Switch command"},
		{"Output Enter", "Jump to problem"},
		{"F10", "Open menu"},
		{"Alt+F/S/B/H/W", "Choose menu"},
		{"Menu arrows/Enter/Esc", "Navigate / act / close"},
		{"F1", "Open help"},
		{"Esc", "Close help"},
		{"Help Up/Down", "Scroll one row"},
		{"Help PgUp/PgDn", "Scroll by page"},
		{"Help Home/End", "First / last row"},
		{"Ctrl+Q / Ctrl+C", "Quit"},
		{"Unsaved D / Esc", "Discard / cancel"},
		{"Window list arrows/Enter/Esc", "Select / activate / cancel"},
	}
}

// environmentEntries lists the current project, Go, and terminal settings.
func environmentEntries(state shellState, width, height int) []helpEntry {
	value := func(text string) string {
		if text == "" {
			return "unset"
		}
		return text
	}
	openFile := "none"
	if state.document != nil {
		openFile = state.document.Path
	}
	version := "detecting..."
	if state.toolchain.Version != "" {
		version = state.toolchain.Version
	}
	if state.toolchainErr != nil {
		version = "unavailable"
	}
	runDefault := "automatic"
	if state.runTarget != "" {
		runDefault = state.runTarget
	}
	entries := []helpEntry{
		{"Project root", state.projectRoot},
		{"Open file", openFile},
		{"Go version", version},
		{"Go executable", value(state.toolchain.Path)},
		{"gopls status", value(state.languageStatus)},
	}
	if state.languageErr != nil {
		entries = append(entries, helpEntry{"gopls error", state.languageErr.Error()})
	}
	if state.toolchainErr != nil {
		entries = append(entries, helpEntry{"Go detection", state.toolchainErr.Error()})
	}
	return append(entries, []helpEntry{
		{"GOTOOLCHAIN env", value(os.Getenv("GOTOOLCHAIN"))},
		{"Run default", runDefault},
		{"Run arguments", value(state.runArgumentText)},
		{"Run priority", "Open main package, then default"},
		{"TERM", value(os.Getenv("TERM"))},
		{"Platform", runtime.GOOS + "/" + runtime.GOARCH},
		{"Terminal size", fmt.Sprintf("%d x %d", width, height)},
	}...)
}

// helpRows wraps help entries to the available dialog width.
func helpRows(state shellState, width, terminalWidth, terminalHeight int) [][]textSegment {
	if width < 1 {
		return nil
	}
	const keyWidth = 23
	wide := width >= 44
	rows := [][]textSegment{}
	if wide {
		left, right := "Shortcut", "Action"
		if state.helpEnvironment {
			left, right = "Setting", "Value"
		}
		rows = append(rows, []textSegment{{left + strings.Repeat(" ", keyWidth-len(left)), helpStyle}, {right, helpStyle}})
	}
	entries := helpEntries()
	if state.helpEnvironment {
		entries = environmentEntries(state, terminalWidth, terminalHeight)
	}
	for _, entry := range entries {
		gap := "  "
		indent := 0
		if wide {
			gap = strings.Repeat(" ", max(2, keyWidth-uniseg.StringWidth(entry.shortcut)))
			indent = keyWidth
		}
		line := []textSegment{{entry.shortcut, shortcutStyle}, {gap + entry.action, helpStyle}}
		row := []textSegment{}
		used := 0
		for _, segment := range line {
			clusters := uniseg.NewGraphemes(segment.text)
			for clusters.Next() {
				cluster := clusters.Str()
				cells := uniseg.StringWidth(cluster)
				if used > 0 && used+cells > width {
					rows = append(rows, row)
					row = []textSegment{{strings.Repeat(" ", indent), helpStyle}}
					used = indent
				}
				if cells > width {
					continue
				}
				if len(row) > 0 && row[len(row)-1].style == segment.style {
					row[len(row)-1].text += cluster
				} else {
					row = append(row, textSegment{cluster, segment.style})
				}
				used += cells
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// helpPageSize returns the number of help entries that fit above the footer.
func helpPageSize(height, lineCount int) int {
	return max(0, min(height-2, lineCount+5)-4)
}

// helpFits reports whether a help dialog can display a title and content.
func helpFits(width, height int) bool { return width >= 16 && height >= 5 }

func renderHelp(screen tcell.Screen, width, height int, state shellState) {
	if !helpFits(width, height) {
		return
	}
	boxWidth := min(width-2, 56)
	lines := helpRows(state, boxWidth-4, width, height)
	boxHeight := min(height-2, len(lines)+5)
	x := (width - boxWidth) / 2
	y := (height - boxHeight) / 2
	for row := y + 1; row < y+boxHeight+1 && row < height-1; row++ {
		for col := x + 2; col < x+boxWidth+2 && col < width; col++ {
			screen.SetContent(col, row, ' ', nil, shadowStyle)
		}
	}
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
	title := "RapidGo shortcuts"
	if state.helpEnvironment {
		title = "RapidGo environment"
	}
	drawText(screen, x+2, y+1, boxWidth-3, title, helpStyle)
	page := helpPageSize(height, len(lines))
	start := min(state.helpScroll, max(0, len(lines)-page))
	for index, line := range lines[start:min(len(lines), start+page)] {
		row := y + 2 + index
		drawStyledText(screen, x+2, row, boxWidth-3, line)
	}
	if boxHeight >= 4 {
		footer := "Esc close  Up/Down PgUp/PgDn Home/End scroll"
		if page < len(lines) {
			footer = fmt.Sprintf("Esc close  %d-%d/%d  Up/Down PgUp/PgDn Home/End", start+1, min(len(lines), start+page), len(lines))
		}
		drawText(screen, x+2, y+boxHeight-2, boxWidth-4, footer, helpStyle)
	}
}

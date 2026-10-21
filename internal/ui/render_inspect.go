package ui

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func renderStack(screen tcell.Screen, area rectangle, state shellState) {
	run := state.debugger
	if !inspectable(run) {
		renderDebugList(screen, area, state, i18n.Format("msg_call_stack_d", 0), nil, 0, 0)
		return
	}
	labels := make([]string, 0, len(run.frames))
	for index, frame := range run.frames {
		marker := "  "
		if index == run.frame {
			marker = "► "
		}
		labels = append(labels, marker+frameLabel(state.projectRoot, frame.Name, frame.Source.Path, frame.Line))
	}
	renderDebugList(screen, area, state, i18n.Format("msg_call_stack_d", len(labels)), labels, run.stackCursor, run.stackScroll)
}

func renderVariables(screen tcell.Screen, area rectangle, state shellState) {
	run := state.debugger
	frame, ok := run.selectedFrame()
	if !inspectable(run) || !ok {
		renderDebugList(screen, area, state, i18n.Text("msg_variables_title"), nil, 0, 0)
		return
	}
	var labels []string
	if run.tree != nil {
		for _, row := range run.tree.Rows() {
			labels = append(labels, variableLabel(row.Depth, row.Name, row.Type, row.Value, row.Expandable, row.Expanded, row.Loading))
		}
	}
	renderDebugList(screen, area, state, i18n.Format("msg_variables_s", frame.Name), labels, run.variable, run.varScroll)
}

// inspectable reports whether the paused program has a stack to show.
func inspectable(run *debugRun) bool {
	return run.active() && run.paused && len(run.frames) > 0
}

// renderDebugList draws labels with the selection, or why there is nothing to show.
func renderDebugList(screen tcell.Screen, area rectangle, state shellState, title string, labels []string, selected, scroll int) {
	rows := max(1, area.height-2)
	selected = min(selected, max(0, len(labels)-1))
	top := max(0, min(scrollTo(scroll, selected, rows), max(0, len(labels)-rows)))
	if len(labels) > rows {
		title += i18n.Format("msg_d_d_d", top+1, min(len(labels), top+rows), len(labels))
	}
	drawFrame(screen, area, title, state.focus == focusOutput)
	if area.width < 4 || area.height < 3 {
		return
	}
	if len(labels) == 0 {
		drawText(screen, area.x+2, area.y+1, area.width-4, debugPlaceholder(state), baseStyle)
		return
	}
	for index := top; index < min(len(labels), top+rows); index++ {
		style := baseStyle
		if state.focus == focusOutput && index == selected {
			style = treeSelectedStyle
			for col := area.x + 1; col < area.x+area.width-1; col++ {
				screen.SetContent(col, area.y+1+index-top, ' ', nil, style)
			}
		}
		drawText(screen, area.x+1, area.y+1+index-top, area.width-2, labels[index], style)
	}
}

func debugPlaceholder(state shellState) string {
	run := state.debugger
	switch {
	case !run.active():
		return i18n.Text("msg_no_debug_session_is_running")
	case !run.paused:
		return i18n.Text("msg_the_program_is_running")
	case len(run.frames) == 0 || state.bottomMode == bottomVariables && run.tree == nil:
		return i18n.Text("msg_loading")
	default:
		return i18n.Text("msg_no_variables")
	}
}

func frameLabel(root, name, path string, line int) string {
	if path == "" {
		return name
	}
	if relative, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(relative, "..") {
		path = relative
	}
	return i18n.Format("msg_s_s_d", name, path, line)
}

// variableLabel reads like a Go declaration, such as "n int = 3", behind a fold marker.
func variableLabel(depth int, name, kind, value string, expandable, expanded, loading bool) string {
	marker := "  "
	switch {
	case expanded:
		marker = "▾ "
	case expandable:
		marker = "▸ "
	}
	label := strings.Repeat("  ", depth) + marker + name
	if kind != "" {
		label += " " + kind
	}
	if value != "" {
		label += " = " + value
	}
	if loading {
		label += " " + i18n.Text("msg_loading")
	}
	return label
}

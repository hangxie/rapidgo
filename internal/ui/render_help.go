package ui

import (
	"os"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// helpEntry pairs a shortcut with its action.
type helpEntry struct{ shortcut, action string }

// helpEntries lists keyboard actions.
func helpEntries() []helpEntry {
	return []helpEntry{
		{i18n.Text("msg_f3"), i18n.Text("msg_focus_tree")},
		{i18n.Text("msg_alt_f3"), i18n.Text("msg_close_active_editor_window")},
		{i18n.Text("msg_f6_shift_f6"), i18n.Text("msg_next_previous_editor_window")},
		{i18n.Text("msg_alt_0"), i18n.Text("msg_list_open_editor_windows")},
		{i18n.Text("msg_ctrl_f6"), i18n.Text("msg_next_pane")},
		{i18n.Text("msg_window_menu"), i18n.Text("msg_new_view_tile_cascade_zoom")},
		{i18n.Text("msg_f5"), i18n.Text("msg_zoom_restore_active_window")},
		{i18n.Text("msg_ctrl_f5"), i18n.Text("msg_size_move_active_window")},
		{i18n.Text("msg_window_arrows"), i18n.Text("msg_move_shift_arrows_resize")},
		{i18n.Text("msg_window_enter_esc"), i18n.Text("msg_finish_move_resize")},
		{i18n.Text("msg_tree_up_down"), i18n.Text("msg_select_item")},
		{i18n.Text("msg_tree_left_right"), i18n.Text("msg_fold_expand")},
		{i18n.Text("msg_tree_enter"), i18n.Text("msg_open_item")},
		{i18n.Text("msg_tree_home_end"), i18n.Text("msg_first_last_item")},
		{i18n.Text("msg_tree_pgup_pgdn"), i18n.Text("msg_move_by_page")},
		{i18n.Text("msg_editor_arrows"), i18n.Text("msg_move_cursor")},
		{i18n.Text("msg_editor_home_end"), i18n.Text("msg_line_ends")},
		{i18n.Text("msg_editor_pgup_pgdn"), i18n.Text("msg_move_by_page")},
		{i18n.Text("msg_ctrl_home_end"), i18n.Text("msg_file_ends")},
		{i18n.Text("msg_shift_movement"), i18n.Text("msg_extend_selection")},
		{i18n.Text("msg_tab_shift_tab"), i18n.Text("msg_indent_unindent")},
		{i18n.Text("msg_enter"), i18n.Text("msg_new_line")},
		{i18n.Text("msg_backspace_delete"), i18n.Text("msg_erase")},
		{i18n.Text("msg_ctrl_a"), i18n.Text("msg_select_all")},
		{i18n.Text("msg_ctrl_z_ctrl_y"), i18n.Text("msg_undo_redo")},
		{i18n.Text("msg_f2"), i18n.Text("msg_save")},
		{i18n.Text("msg_ctrl_f_ctrl_g"), i18n.Text("msg_find_next_2")},
		{i18n.Text("msg_alt_i"), i18n.Text("msg_inspect_symbol_with_gopls")},
		{i18n.Text("msg_ctrl_space_alt_c"), i18n.Text("msg_complete_symbol_with_gopls")},
		{i18n.Text("msg_f12_alt_d"), i18n.Text("msg_jump_to_go_definition")},
		{i18n.Text("msg_shift_f12_alt_r"), i18n.Text("msg_list_go_references")},
		{i18n.Text("msg_locations_arrows_enter"), i18n.Text("msg_select_jump_to_source")},
		{i18n.Text("msg_locations_esc"), i18n.Text("msg_return_to_output_and_editor")},
		{i18n.Text("msg_completion_arrows"), i18n.Text("msg_select_a_suggestion")},
		{i18n.Text("msg_completion_enter_esc"), i18n.Text("msg_insert_cancel_suggestion")},
		{i18n.Text("msg_alt_e"), i18n.Text("msg_toggle_errors_output_view")},
		{i18n.Text("msg_errors_arrows_enter"), i18n.Text("msg_select_jump_to_diagnostic")},
		{i18n.Text("msg_errors_left_right"), i18n.Text("msg_return_to_output")},
		{i18n.Text("msg_errors_esc"), i18n.Text("msg_return_to_output_and_editor")},
		{i18n.Text("msg_hover_esc_arrows"), i18n.Text("msg_close_scroll_symbol_info")},
		{i18n.Text("msg_search_enter_esc"), i18n.Text("msg_find_cancel")},
		{i18n.Text("msg_f9"), i18n.Text("msg_build")},
		{i18n.Text("msg_ctrl_t"), i18n.Text("msg_test")},
		{i18n.Text("msg_ctrl_f9"), i18n.Text("msg_run_in_output_pane_noninteractive")},
		{i18n.Text("msg_alt_f9"), i18n.Text("msg_run_current_file_with_helpers")},
		{i18n.Text("msg_build_run_in_terminal"), i18n.Text("msg_run_tui_in_terminal")},
		{i18n.Text("msg_build_run_options"), i18n.Text("msg_set_default_package_arguments")},
		{i18n.Text("msg_ctrl_k"), i18n.Text("msg_stop_all_go_jobs")},
		{i18n.Text("msg_output_up_down"), i18n.Text("msg_select_line")},
		{i18n.Text("msg_output_pgup_pgdn"), i18n.Text("msg_move_by_page")},
		{i18n.Text("msg_output_home_end"), i18n.Text("msg_first_latest_line")},
		{i18n.Text("msg_output_left_right"), i18n.Text("msg_switch_command")},
		{i18n.Text("msg_output_enter"), i18n.Text("msg_jump_to_problem")},
		{i18n.Text("msg_f10"), i18n.Text("msg_open_menu")},
		{i18n.Text("msg_alt_f_s_b_h_w"), i18n.Text("msg_choose_menu")},
		{i18n.Text("msg_menu_arrows_enter_esc"), i18n.Text("msg_navigate_act_close")},
		{i18n.Text("msg_f1"), i18n.Text("msg_open_help")},
		{i18n.Text("msg_esc"), i18n.Text("msg_close_help_2")},
		{i18n.Text("msg_help_up_down"), i18n.Text("msg_scroll_one_row")},
		{i18n.Text("msg_help_pgup_pgdn"), i18n.Text("msg_scroll_by_page")},
		{i18n.Text("msg_help_home_end"), i18n.Text("msg_first_last_row")},
		{i18n.Text("msg_ctrl_q_ctrl_c"), i18n.Text("msg_quit")},
		{i18n.Text("msg_unsaved_d_esc"), i18n.Text("msg_discard_cancel")},
		{i18n.Text("msg_window_list_arrows_enter_esc"), i18n.Text("msg_select_activate_cancel")},
	}
}

// environmentEntries lists the current project, Go, and terminal settings.
func environmentEntries(state shellState, width, height int) []helpEntry {
	value := func(text string) string {
		if text == "" {
			return i18n.Text("msg_unset")
		}
		return text
	}
	openFile := i18n.Text("msg_none")
	if state.document != nil {
		openFile = state.document.Path
	}
	version := i18n.Text("msg_detecting")
	if state.toolchain.Version != "" {
		version = state.toolchain.Version
	}
	if state.toolchainErr != nil {
		version = i18n.Text("status_unavailable")
	}
	runDefault := i18n.Text("msg_automatic")
	if state.runTarget != "" {
		runDefault = state.runTarget
	}
	entries := []helpEntry{
		{i18n.Text("msg_project_root"), state.projectRoot},
		{i18n.Text("msg_open_file"), openFile},
		{i18n.Text("msg_go_version"), version},
		{i18n.Text("msg_go_executable"), value(state.toolchain.Path)},
		{i18n.Text("msg_gopls_status"), languageStatusText(state.languageStatus)},
	}
	if state.languageErr != nil {
		entries = append(entries, helpEntry{i18n.Text("msg_gopls_error"), state.languageErr.Error()})
	}
	if state.toolchainErr != nil {
		entries = append(entries, helpEntry{i18n.Text("msg_go_detection"), state.toolchainErr.Error()})
	}
	return append(entries, []helpEntry{
		{i18n.Text("msg_gotoolchain_env"), value(os.Getenv("GOTOOLCHAIN"))},
		{i18n.Text("msg_run_default"), runDefault},
		{i18n.Text("msg_run_arguments"), value(state.runArgumentText)},
		{i18n.Text("msg_run_priority"), i18n.Text("msg_open_main_package_then_default")},
		{i18n.Text("msg_term"), value(os.Getenv("TERM"))},
		{i18n.Text("msg_platform"), runtime.GOOS + "/" + runtime.GOARCH},
		{i18n.Text("msg_terminal_size"), i18n.Format("msg_d_x_d", width, height)},
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
	entries := helpEntries()
	if state.helpEnvironment {
		entries = environmentEntries(state, terminalWidth, terminalHeight)
	}
	if wide {
		left, right := i18n.Text("msg_shortcut"), i18n.Text("msg_action")
		if state.helpEnvironment {
			left, right = i18n.Text("msg_setting"), i18n.Text("msg_value")
		}
		entries = append([]helpEntry{{left, right}}, entries...)
	}
	for index, entry := range entries {
		gap := "  "
		indent := 0
		if wide {
			gap = strings.Repeat(" ", max(2, keyWidth-uniseg.StringWidth(entry.shortcut)))
			indent = keyWidth
		}
		keyStyle := shortcutStyle
		if wide && index == 0 {
			keyStyle = helpStyle
		}
		line := []textSegment{{entry.shortcut, keyStyle}, {gap + entry.action, helpStyle}}
		rows = append(rows, wrapHelpLine(line, width, indent)...)
	}
	return rows
}

// wrapHelpLine wraps styled text without splitting grapheme clusters.
func wrapHelpLine(line []textSegment, width, indent int) [][]textSegment {
	rows := [][]textSegment{}
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
	title := i18n.Text("msg_rapidgo_shortcuts")
	if state.helpEnvironment {
		title = i18n.Text("msg_rapidgo_environment")
	}
	drawText(screen, x+2, y+1, boxWidth-3, title, helpStyle)
	page := helpPageSize(height, len(lines))
	start := min(state.helpScroll, max(0, len(lines)-page))
	for index, line := range lines[start:min(len(lines), start+page)] {
		row := y + 2 + index
		drawStyledText(screen, x+2, row, boxWidth-3, line)
	}
	if boxHeight >= 4 {
		footer := i18n.Text("msg_esc_close_up_down_pgup_pgdn_home_end_scroll")
		if page < len(lines) {
			footer = i18n.Format("msg_esc_close_d_d_d_up_down_pgup_pgdn_home_end", start+1, min(len(lines), start+page), len(lines))
		}
		drawText(screen, x+2, y+boxHeight-2, boxWidth-4, footer, helpStyle)
	}
}

func languageStatusText(status string) string {
	if status == "" {
		return i18n.Text("msg_unset")
	}
	return i18n.Text("status_" + status)
}

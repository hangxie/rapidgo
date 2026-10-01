package ui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/workspace"
)

type windowChooser struct {
	windows []*workspace.Window
	index   int
}

func (state *shellState) openWindowList() {
	state.menuOpen = false
	if state.workspace == nil || len(state.workspace.Windows) == 0 {
		state.message = i18n.Text("msg_no_editor_windows_to_list")
		return
	}
	state.helpVisible = false
	state.hoverVisible = false
	state.completionVisible = false
	state.cancelPendingCompletion()
	state.hoverSeq++
	state.navigationSeq++
	state.focusSeq++
	state.windowChooser = &windowChooser{
		windows: append([]*workspace.Window(nil), state.workspace.Windows...),
		index:   state.workspace.Active,
	}
	state.windowListMessage()
}

func (state *shellState) windowListMessage() {
	chooser := state.windowChooser
	state.message = i18n.Format("msg_window_d_s_arrows_select_enter_activates_esc_cancels", chooser.index+1, chooser.windows[chooser.index].Document.Path)
}

func (state *shellState) handleWindowListKey(screen tcell.Screen, event *tcell.EventKey) bool {
	chooser := state.windowChooser
	last := len(chooser.windows) - 1
	_, height := screen.Size()
	page := max(1, height-6)
	switch event.Key() {
	case tcell.KeyCtrlQ, tcell.KeyCtrlC:
		state.windowChooser = nil
		return state.requestQuit()
	case tcell.KeyEscape:
		state.windowChooser = nil
		state.message = i18n.Text("msg_window_selection_cancelled")
		return false
	case tcell.KeyUp:
		chooser.index = (chooser.index + last) % len(chooser.windows)
	case tcell.KeyDown:
		chooser.index = (chooser.index + 1) % len(chooser.windows)
	case tcell.KeyHome:
		chooser.index = 0
	case tcell.KeyEnd:
		chooser.index = last
	case tcell.KeyPgUp:
		chooser.index = max(0, chooser.index-page)
	case tcell.KeyPgDn:
		chooser.index = min(last, chooser.index+page)
	case tcell.KeyEnter:
		selected := chooser.windows[chooser.index]
		state.windowChooser = nil
		state.storeWindow()
		for index, win := range state.workspace.Windows {
			if win == selected {
				state.workspace.Select(index)
				state.openSeq++
				state.opening = false
				state.pendingPosition = nil
				state.loadWindow()
				state.setFocus(focusEditor)
				state.ensureCursorVisible(screen)
				state.message = i18n.Format("msg_activated_s", selected.Document.Path)
				return false
			}
		}
		state.message = i18n.Text("msg_selected_window_is_no_longer_open")
		return false
	}
	state.windowListMessage()
	return false
}

func renderWindowList(screen tcell.Screen, width, height int, state shellState) {
	chooser := state.windowChooser
	labels := make([]string, len(chooser.windows))
	for index, win := range chooser.windows {
		active, dirty := " ", ""
		if win == state.workspace.Current() {
			active = ">"
		}
		if win.Buffer.Dirty() {
			dirty = " *"
		}
		path := win.Document.Path
		if relative, err := filepath.Rel(state.projectRoot, path); err == nil {
			path = relative
		}
		labels[index] = i18n.Format("msg_s_d_s_s", active, index+1, path, dirty)
	}
	renderChoiceDialog(screen, width, height, i18n.Text("msg_windows"), i18n.Text("msg_activate"), labels, chooser.index)
}

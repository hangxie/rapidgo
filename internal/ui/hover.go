package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
	"github.com/hangxie/rapidgo/internal/i18n"
)

type languageHoverRequest struct {
	snapshot languageSnapshot
	position gopls.Position
	cursor   editor.Position
	ticket   uint64
}

type languageHoverResult struct {
	request languageHoverRequest
	content string
	err     error
}

func (connection *languageConnection) hover(request languageHoverRequest) {
	if err := connection.apply(request.snapshot); err != nil {
		sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageHovered, hover: languageHoverResult{request: request, err: err}})
		return
	}
	query, cancel := context.WithTimeout(connection.ctx, 5*time.Second)
	defer cancel()
	content, err := connection.session.Hover(query, request.snapshot.path, request.position)
	sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageHovered, hover: languageHoverResult{request: request, content: content, err: err}})
}

func (state *shellState) requestHover() {
	if state.document == nil || state.buffer == nil || filepath.Ext(state.document.Path) != ".go" {
		state.message = i18n.Text("msg_open_a_go_file_to_inspect_a_symbol")
		return
	}
	if state.languageStatus == "unavailable" || state.enqueueHover == nil || state.enqueueLanguage == nil {
		state.message = i18n.Text("msg_gopls_is_unavailable_see_help_environment_info")
		return
	}
	state.syncLanguage()
	state.menuOpen = false
	state.cancelPendingCompletion()
	state.hoverSeq++
	cursor := state.buffer.Cursor()
	line := state.buffer.Lines()[cursor.Line]
	request := languageHoverRequest{
		snapshot: state.languageQueued,
		position: gopls.Position{Line: cursor.Line, Character: graphemeUTF16Column(line, cursor.Column)},
		cursor:   cursor,
		ticket:   state.hoverSeq,
	}
	state.hoverVisible = false
	state.message = i18n.Text("msg_inspecting_symbol_with_gopls")
	state.enqueueHover(request)
}

func (state *shellState) applyHoverResult(result languageHoverResult) {
	request := result.request
	if request.ticket != state.hoverSeq {
		return
	}
	if state.helpVisible || (state.bottomMode == bottomErrors && state.focus == focusOutput) || state.confirm != confirmNone || state.chooser != nil || state.menuOpen {
		if state.message == i18n.Text("msg_inspecting_symbol_with_gopls") {
			state.message = i18n.Text("msg_hover_cancelled_another_dialog_is_open")
		}
		return
	}
	if state.document == nil || state.buffer == nil || state.document.Path != request.snapshot.path || state.buffer.Text() != request.snapshot.text || state.buffer.Cursor() != request.cursor {
		if state.message == i18n.Text("msg_inspecting_symbol_with_gopls") {
			state.message = i18n.Text("msg_hover_cancelled_file_or_caret_changed")
		}
		return
	}
	if result.err != nil {
		state.message = i18n.Format("msg_gopls_hover_failed_v", result.err)
		return
	}
	content := strings.TrimSpace(result.content)
	if content == "" {
		state.message = i18n.Text("msg_no_symbol_information_at_the_caret")
		return
	}
	state.hoverText = content
	state.hoverScroll = 0
	state.hoverVisible = true
	state.message = i18n.Text("msg_hover_esc_closes_arrows_and_pgup_pgdn_scroll")
}

func graphemeUTF16Column(line string, column int) int {
	units := 0
	clusters := uniseg.NewGraphemes(line)
	for index := 0; index < column && clusters.Next(); index++ {
		for _, value := range clusters.Str() {
			units += utf16.RuneLen(value)
		}
	}
	return units
}

func (state *shellState) ensureHoverFits(screen tcell.Screen) {
	width, height := screen.Size()
	if state.hoverVisible && (width < 16 || height < 5) {
		state.hoverVisible = false
		state.message = i18n.Text("msg_hover_needs_a_terminal_of_at_least_16x5")
	}
}

func (state *shellState) handleHoverKey(screen tcell.Screen, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyCtrlQ || event.Key() == tcell.KeyCtrlC {
		state.hoverVisible = false
		return state.requestQuit()
	}
	if event.Key() == tcell.KeyEscape {
		state.hoverVisible = false
		if state.message == i18n.Text("msg_hover_esc_closes_arrows_and_pgup_pgdn_scroll") || strings.HasPrefix(state.message, i18n.Text("msg_hover")) {
			state.message = ""
		}
		return false
	}
	width, height := screen.Size()
	boxWidth := min(width-2, 72)
	rows := hoverRows(state.hoverText, max(1, boxWidth-4))
	boxHeight := min(height, 20, max(5, len(rows)+4))
	page := max(1, boxHeight-4)
	last := max(0, len(rows)-page)
	switch event.Key() {
	case tcell.KeyUp:
		state.hoverScroll = max(0, state.hoverScroll-1)
	case tcell.KeyDown:
		state.hoverScroll = min(last, state.hoverScroll+1)
	case tcell.KeyPgUp:
		state.hoverScroll = max(0, state.hoverScroll-page)
	case tcell.KeyPgDn:
		state.hoverScroll = min(last, state.hoverScroll+page)
	case tcell.KeyHome:
		state.hoverScroll = 0
	case tcell.KeyEnd:
		state.hoverScroll = last
	}
	return false
}

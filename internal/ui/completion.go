package ui

import (
	"context"
	"path/filepath"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
	"github.com/hangxie/rapidgo/internal/i18n"
)

type languageCompletionRequest struct {
	snapshot languageSnapshot
	position gopls.Position
	cursor   editor.Position
	ticket   uint64
}

type languageCompletionResult struct {
	request languageCompletionRequest
	items   []gopls.CompletionItem
	err     error
}

func (connection *languageConnection) complete(request languageCompletionRequest) {
	if err := connection.apply(request.snapshot); err != nil {
		sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageCompleted, completion: languageCompletionResult{request: request, err: err}})
		return
	}
	query, cancel := context.WithTimeout(connection.ctx, 5*time.Second)
	defer cancel()
	items, err := connection.session.Complete(query, request.snapshot.path, request.position)
	sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageCompleted, completion: languageCompletionResult{request: request, items: items, err: err}})
}

func (state *shellState) requestCompletion() {
	if state.document == nil || state.buffer == nil || filepath.Ext(state.document.Path) != ".go" {
		state.message = i18n.Text("msg_open_a_go_file_to_complete_a_symbol")
		return
	}
	if state.focus != focusEditor {
		state.message = i18n.Text("msg_focus_the_editor_to_complete_a_symbol")
		return
	}
	if state.languageStatus == "unavailable" || state.enqueueCompletion == nil || state.enqueueLanguage == nil {
		state.message = i18n.Text("msg_gopls_is_unavailable_see_help_environment_info")
		return
	}
	state.syncLanguage()
	state.menuOpen = false
	state.hoverSeq++
	state.completionSeq++
	cursor := state.buffer.Cursor()
	request := languageCompletionRequest{
		snapshot: state.languageQueued,
		position: gopls.Position{Line: cursor.Line, Character: graphemeUTF16Column(state.buffer.Lines()[cursor.Line], cursor.Column)},
		cursor:   cursor,
		ticket:   state.completionSeq,
	}
	state.completionVisible = false
	state.completionPending = true
	state.message = i18n.Text("msg_completing_with_gopls")
	state.enqueueCompletion(request)
}

func (state *shellState) applyCompletionResult(result languageCompletionResult) {
	request := result.request
	if request.ticket != state.completionSeq || !state.completionPending {
		return
	}
	state.completionPending = false
	if state.focus != focusEditor {
		state.message = i18n.Text("msg_completion_cancelled_editor_lost_focus")
		return
	}
	if state.document == nil || state.buffer == nil || state.document.Path != request.snapshot.path || state.buffer.Text() != request.snapshot.text || state.buffer.Cursor() != request.cursor {
		state.message = i18n.Text("msg_completion_cancelled_file_or_caret_changed")
		return
	}
	if state.helpVisible || state.hoverVisible || state.confirm != confirmNone || state.chooser != nil || state.menuOpen || state.searching || state.editingRunArgs || state.runSetupOpen {
		state.message = i18n.Text("msg_completion_cancelled_another_dialog_is_open")
		return
	}
	if result.err != nil {
		state.message = i18n.Format("msg_gopls_completion_failed_v", result.err)
		return
	}
	if len(result.items) == 0 {
		state.message = i18n.Text("msg_no_completions_at_the_caret")
		return
	}
	state.completionRequest = request
	state.completionItems = result.items
	state.completionSelected = 0
	state.completionScroll = 0
	state.completionVisible = true
	state.message = i18n.Text("msg_completion_enter_inserts_esc_cancels")
}

func (state *shellState) handleCompletionKey(screen tcell.Screen, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyCtrlQ || event.Key() == tcell.KeyCtrlC {
		state.completionVisible = false
		return state.requestQuit()
	}
	last := len(state.completionItems) - 1
	page := completionPage(screen)
	switch event.Key() {
	case tcell.KeyEscape:
		state.completionVisible = false
		state.message = i18n.Text("msg_completion_cancelled")
	case tcell.KeyUp:
		state.completionSelected = max(0, state.completionSelected-1)
	case tcell.KeyDown:
		state.completionSelected = min(last, state.completionSelected+1)
	case tcell.KeyPgUp:
		state.completionSelected = max(0, state.completionSelected-page)
	case tcell.KeyPgDn:
		state.completionSelected = min(last, state.completionSelected+page)
	case tcell.KeyHome:
		state.completionSelected = 0
	case tcell.KeyEnd:
		state.completionSelected = last
	case tcell.KeyEnter:
		state.completionVisible = false
		if state.buffer == nil || state.document == nil || state.document.Path != state.completionRequest.snapshot.path || state.buffer.Text() != state.completionRequest.snapshot.text {
			state.message = i18n.Text("msg_completion_cancelled_file_changed")
			return false
		}
		if err := applyCompletionItem(state.buffer, state.completionItems[state.completionSelected], state.completionRequest.cursor); err != nil {
			state.message = i18n.Format("msg_cannot_apply_completion_v", err)
			return false
		}
		state.message = i18n.Format("msg_inserted_s", state.completionItems[state.completionSelected].Label)
		state.ensureCursorVisible(screen)
	}
	if state.completionSelected < state.completionScroll {
		state.completionScroll = state.completionSelected
	}
	if state.completionSelected >= state.completionScroll+page {
		state.completionScroll = state.completionSelected - page + 1
	}
	return false
}

func (state *shellState) cancelPendingCompletion() {
	if state.completionPending {
		state.completionPending = false
		state.completionSeq++
		if state.message == i18n.Text("msg_completing_with_gopls") {
			state.message = i18n.Text("msg_completion_cancelled")
		}
	}
}

func (state *shellState) ensureCompletionFits(screen tcell.Screen) {
	width, height := screen.Size()
	if state.completionVisible && !completionFits(width, height) {
		state.completionVisible = false
		state.message = i18n.Text("msg_completion_needs_a_larger_terminal")
	}
}

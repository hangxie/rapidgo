package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
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
		state.message = "Open a Go file to complete a symbol"
		return
	}
	if state.focus != focusEditor {
		state.message = "Focus the editor to complete a symbol"
		return
	}
	if state.languageStatus == "unavailable" || state.enqueueCompletion == nil || state.enqueueLanguage == nil {
		state.message = "gopls is unavailable; see Help → Environment Info"
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
	state.message = "Completing with gopls..."
	state.enqueueCompletion(request)
}

func (state *shellState) applyCompletionResult(result languageCompletionResult) {
	request := result.request
	if request.ticket != state.completionSeq || !state.completionPending {
		return
	}
	state.completionPending = false
	if state.focus != focusEditor {
		state.message = "Completion cancelled: editor lost focus"
		return
	}
	if state.document == nil || state.buffer == nil || state.document.Path != request.snapshot.path || state.buffer.Text() != request.snapshot.text || state.buffer.Cursor() != request.cursor {
		state.message = "Completion cancelled: file or caret changed"
		return
	}
	if state.helpVisible || state.hoverVisible || state.confirm != confirmNone || state.chooser != nil || state.menuOpen || state.searching || state.editingRunArgs || state.runSetupOpen {
		state.message = "Completion cancelled: another dialog is open"
		return
	}
	if result.err != nil {
		state.message = fmt.Sprintf("gopls completion failed: %v", result.err)
		return
	}
	if len(result.items) == 0 {
		state.message = "No completions at the caret"
		return
	}
	state.completionRequest = request
	state.completionItems = result.items
	state.completionSelected = 0
	state.completionScroll = 0
	state.completionVisible = true
	state.message = "Completion: Enter inserts; Esc cancels"
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
		state.message = "Completion cancelled"
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
			state.message = "Completion cancelled: file changed"
			return false
		}
		if err := applyCompletionItem(state.buffer, state.completionItems[state.completionSelected], state.completionRequest.cursor); err != nil {
			state.message = fmt.Sprintf("Cannot apply completion: %v", err)
			return false
		}
		state.message = "Inserted " + state.completionItems[state.completionSelected].Label
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
		if strings.HasPrefix(state.message, "Completing ") {
			state.message = "Completion cancelled"
		}
	}
}

func (state *shellState) ensureCompletionFits(screen tcell.Screen) {
	width, height := screen.Size()
	if state.completionVisible && !completionFits(width, height) {
		state.completionVisible = false
		state.message = "Completion needs a larger terminal"
	}
}

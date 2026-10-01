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

type navigationKind uint8

const (
	navigationDefinition navigationKind = iota
	navigationReferences
)

func (kind navigationKind) name() string {
	if kind == navigationReferences {
		return i18n.Text("msg_references")
	}
	return i18n.Text("msg_definition")
}

type languageNavigationRequest struct {
	snapshot languageSnapshot
	position gopls.Position
	cursor   editor.Position
	kind     navigationKind
	ticket   uint64
}

type languageNavigationResult struct {
	request   languageNavigationRequest
	locations []gopls.Location
	err       error
}

func (state *shellState) requestF12Navigation(event *tcell.EventKey) {
	if event.Modifiers()&tcell.ModShift != 0 {
		state.requestNavigation(navigationReferences)
		return
	}
	state.requestNavigation(navigationDefinition)
}

func (connection *languageConnection) navigate(request languageNavigationRequest) {
	result := languageNavigationResult{request: request}
	if err := connection.apply(request.snapshot); err != nil {
		result.err = err
	} else {
		query, cancel := context.WithTimeout(connection.ctx, 5*time.Second)
		defer cancel()
		if request.kind == navigationReferences {
			result.locations, result.err = connection.session.References(query, request.snapshot.path, request.position)
		} else {
			result.locations, result.err = connection.session.Definition(query, request.snapshot.path, request.position)
		}
	}
	sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageNavigated, navigation: result})
}

func (state *shellState) requestNavigation(kind navigationKind) {
	if state.document == nil || state.buffer == nil || filepath.Ext(state.document.Path) != ".go" {
		state.message = i18n.Format("msg_open_a_go_file_to_find_s", kind.name())
		return
	}
	if state.focus != focusEditor {
		state.message = i18n.Format("msg_focus_the_editor_to_find_s", kind.name())
		return
	}
	if state.languageStatus == "unavailable" || state.enqueueNavigation == nil || state.enqueueLanguage == nil {
		state.message = i18n.Text("msg_gopls_is_unavailable_see_help_environment_info")
		return
	}
	state.syncLanguage()
	state.menuOpen = false
	state.cancelPendingCompletion()
	state.hoverSeq++
	state.navigationSeq++
	cursor := state.buffer.Cursor()
	request := languageNavigationRequest{
		snapshot: state.languageQueued,
		position: gopls.Position{Line: cursor.Line, Character: graphemeUTF16Column(state.buffer.Lines()[cursor.Line], cursor.Column)},
		cursor:   cursor, kind: kind, ticket: state.navigationSeq,
	}
	state.message = i18n.Format("msg_finding_s_with_gopls", kind.name())
	state.enqueueNavigation(request)
}

func (state *shellState) applyNavigationResult(result languageNavigationResult) {
	request := result.request
	if request.ticket != state.navigationSeq {
		return
	}
	if state.focus != focusEditor || state.helpVisible || state.hoverVisible || state.completionVisible || state.confirm != confirmNone || state.chooser != nil || state.menuOpen || state.searching {
		state.message = i18n.Text("msg_navigation_cancelled_editor_lost_focus")
		return
	}
	if state.document == nil || state.buffer == nil || state.document.Path != request.snapshot.path || state.buffer.Text() != request.snapshot.text || state.buffer.Cursor() != request.cursor {
		state.message = i18n.Text("msg_navigation_cancelled_file_or_caret_changed")
		return
	}
	if result.err != nil {
		state.message = i18n.Format("msg_gopls_s_failed_v", request.kind.name(), result.err)
		return
	}
	if len(result.locations) == 0 {
		state.message = i18n.Format("msg_no_s_found_at_the_caret", request.kind.name())
		return
	}
	if request.kind == navigationDefinition && len(result.locations) == 1 {
		state.jumpToLocation(result.locations[0])
		return
	}
	state.locations = result.locations
	state.locationKind = request.kind
	state.locationSelected = 0
	state.locationScroll = 0
	state.bottomMode = bottomLocations
	state.focusSeq++
	state.setFocus(focusOutput)
	state.message = i18n.Format("msg_d_s_arrows_select_enter_jumps_esc_closes", len(result.locations), request.kind.name())
}

func (state *shellState) jumpToLocation(location gopls.Location) {
	if location.Path == "" || (state.document == nil || state.document.Path != location.Path) && !exists(location.Path) {
		state.message = i18n.Format("msg_cannot_locate_s", location.Path)
		return
	}
	state.openAtTarget(location.Path, jumpTarget{line: location.Position.Line, utf16Column: location.Position.Character, fromUTF16: true})
}

func (state *shellState) handleLocationsKey(screen tcell.Screen, event *tcell.EventKey) {
	rows := state.outputRows(screen)
	last := max(0, len(state.locations)-1)
	switch event.Key() {
	case tcell.KeyUp:
		state.locationSelected = max(0, state.locationSelected-1)
	case tcell.KeyDown:
		state.locationSelected = min(last, state.locationSelected+1)
	case tcell.KeyPgUp:
		state.locationSelected = max(0, state.locationSelected-rows)
	case tcell.KeyPgDn:
		state.locationSelected = min(last, state.locationSelected+rows)
	case tcell.KeyHome:
		state.locationSelected = 0
	case tcell.KeyEnd:
		state.locationSelected = last
	case tcell.KeyEnter:
		if len(state.locations) > 0 {
			state.jumpToLocation(state.locations[state.locationSelected])
		}
		return
	case tcell.KeyLeft, tcell.KeyRight:
		state.bottomMode = bottomOutput
		return
	}
	state.locationScroll = min(state.locationScroll, state.locationSelected)
	if state.locationSelected >= state.locationScroll+rows {
		state.locationScroll = state.locationSelected - rows + 1
	}
}

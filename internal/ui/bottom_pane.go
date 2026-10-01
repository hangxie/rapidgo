package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/i18n"
)

type bottomPaneMode uint8

const (
	bottomOutput bottomPaneMode = iota
	bottomErrors
	bottomLocations
)

func (view *jobView) hasActionableDiagnostics() bool {
	for _, line := range view.lines {
		if line.problem != nil && line.problem.Severity != diagnostic.Info {
			return true
		}
	}
	return false
}

func (state *shellState) openProblems(screen tcell.Screen) {
	state.menuOpen = false
	width, height := screen.Size()
	area := calculateLayout(width, height, true).output
	if area.width < 4 || area.height < 3 {
		state.message = i18n.Text("msg_errors_needs_a_larger_terminal")
		return
	}
	state.bottomMode = bottomErrors
	state.errorSelected = 0
	state.errorScroll = 0
	state.focusSeq++
	state.setFocus(focusOutput)
}

func (state *shellState) toggleBottomMode(screen tcell.Screen) {
	state.menuOpen = false
	if state.bottomMode == bottomErrors {
		state.bottomMode = bottomOutput
		return
	}
	state.openProblems(screen)
}

func (state *shellState) closeBottomView() {
	if state.bottomMode != bottomOutput && state.focus == focusOutput && !state.menuOpen {
		state.bottomMode = bottomOutput
		state.setFocus(state.mainFocus)
	}
}

// errorItems combines the selected job's located output with live gopls reports.
func (state *shellState) errorItems() []languageProblem {
	items := make([]languageProblem, 0, len(state.problems))
	if view := state.activeView(); view != nil {
		for _, line := range view.lines {
			if line.problem != nil && line.problem.Severity != diagnostic.Info {
				items = append(items, languageProblem{Diagnostic: *line.problem, utf16Column: -1})
			}
		}
	}
	return append(items, state.problems...)
}

func (state *shellState) handleErrorsKey(screen tcell.Screen, event *tcell.EventKey) {
	items := state.currentErrorItems()
	rows := state.outputRows(screen)
	last := max(0, len(items)-1)
	switch event.Key() {
	case tcell.KeyUp:
		state.errorSelected = max(0, state.errorSelected-1)
	case tcell.KeyDown:
		state.errorSelected = min(last, state.errorSelected+1)
	case tcell.KeyPgUp:
		state.errorSelected = max(0, state.errorSelected-rows)
	case tcell.KeyPgDn:
		state.errorSelected = min(last, state.errorSelected+rows)
	case tcell.KeyHome:
		state.errorSelected = 0
	case tcell.KeyEnd:
		state.errorSelected = last
	case tcell.KeyEnter:
		if len(items) == 0 {
			state.message = i18n.Text("msg_no_error_selected")
			return
		}
		problem := items[state.errorSelected]
		if problem.Source == "gopls" {
			state.jumpToLanguage(problem)
		} else {
			state.jumpTo(problem.Diagnostic)
		}
		return
	case tcell.KeyLeft, tcell.KeyRight:
		state.bottomMode = bottomOutput
		return
	}
	state.errorScroll = min(state.errorScroll, state.errorSelected)
	if state.errorSelected >= state.errorScroll+rows {
		state.errorScroll = state.errorSelected - rows + 1
	}
}

func (state *shellState) currentErrorItems() []languageProblem {
	items := state.errorItems()
	state.errorSelected = min(state.errorSelected, max(0, len(items)-1))
	state.errorScroll = min(state.errorScroll, state.errorSelected)
	return items
}

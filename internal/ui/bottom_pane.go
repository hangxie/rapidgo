package ui

import (
	"path/filepath"
	"slices"
	"strings"

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

// errorItems combines the selected job's located output with live gopls reports, merging repeats.
func (state *shellState) errorItems() []languageProblem {
	items := make([]languageProblem, 0, len(state.problems))
	seen := make(map[problemKey]int)
	add := func(problem languageProblem) {
		key := state.problemKey(problem)
		index, ok := seen[key]
		if !ok {
			index = len(items)
			seen[key] = index
			problem.sources = nil
			items = append(items, problem)
		}
		if problem.Source != "" && !slices.Contains(items[index].sources, problem.Source) {
			items[index].sources = append(items[index].sources, problem.Source)
		}
	}
	if view := state.activeView(); view != nil {
		for _, line := range view.lines {
			if line.problem != nil && line.problem.Severity != diagnostic.Info {
				add(languageProblem{Diagnostic: *line.problem, utf16Column: -1})
			}
		}
	}
	for _, problem := range state.problems {
		add(problem)
	}
	return items
}

// problemKey identifies a problem across tools by file, position, severity, and text.
type problemKey struct {
	path     string
	line     int
	column   int  // one-based
	utf16    bool // column counts UTF-16 units rather than bytes
	severity diagnostic.Severity
	message  string
}

func (state *shellState) problemKey(problem languageProblem) problemKey {
	path := problem.Path
	if !filepath.IsAbs(path) {
		base := state.projectRoot
		if directory, ok := state.packageDir(problem.Package); ok && problem.Source == diagnostic.SourceTest {
			base = directory
		}
		path = filepath.Join(base, filepath.FromSlash(path))
	}
	key := problemKey{path: filepath.Clean(path), line: problem.Line, column: problem.Column, severity: problem.Severity, message: strings.TrimSpace(problem.Message)}
	if problem.Source == "gopls" && problem.Column == 0 {
		// Only open files have the text to convert UTF-16 units to bytes.
		key.column, key.utf16 = problem.utf16Column+1, true
	}
	return key
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

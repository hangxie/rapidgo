package ui

import (
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

func (state *shellState) startSearch() {
	if state.buffer == nil {
		state.message = "Open a file before searching"
		return
	}
	state.menuOpen = false
	state.helpVisible = false
	state.searchInput = ""
	state.searching = true
}

func (state *shellState) handleSearchKey(screen tcell.Screen, event *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyEscape:
		state.searching = false
		state.message = "Search cancelled"
	case tcell.KeyEnter:
		state.searching = false
		if state.searchInput == "" {
			state.message = "Search text is empty"
			return
		}
		state.searchQuery = state.searchInput
		state.findNext(screen)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		clusters := uniseg.NewGraphemes(state.searchInput)
		last := 0
		for clusters.Next() {
			last, _ = clusters.Positions()
		}
		state.searchInput = state.searchInput[:last]
	case tcell.KeyRune:
		if event.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 && !unicode.IsControl(event.Rune()) {
			state.searchInput += string(event.Rune())
		}
	}
}

func (state *shellState) findNext(screen tcell.Screen) {
	if state.buffer == nil || state.searchQuery == "" {
		state.message = "Use Ctrl+F to enter search text"
		return
	}
	match, found := state.buffer.FindNext(state.searchQuery)
	if !found {
		state.message = "Not found: " + state.searchQuery
		return
	}
	_ = state.buffer.Select(match.Start, match.End)
	state.focusSeq++
	state.setFocus(focusEditor)
	state.ensureCursorVisible(screen)
	state.message = "Found: " + state.searchQuery
	if match.Wrapped {
		state.message += " (wrapped)"
	}
}

package ui

import (
	"unicode"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

func handleEvent(screen tcell.Screen, state *shellState, event tcell.Event) bool {
	switch event := event.(type) {
	case *tcell.EventKey:
		return handleKey(screen, state, event)
	case *tcell.EventResize:
		screen.Sync()
		width, height := screen.Size()
		if !helpFits(width, height) {
			state.helpVisible = false
		}
		if width < 16 || height < 5 {
			state.hoverVisible = false
		}
		state.ensureCompletionFits(screen)
		state.ensureCursorVisible(screen)
	case *tcell.EventInterrupt:
		return state.requestQuit()
	}
	return false
}

func handleKey(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	width, height := screen.Size()
	if state.helpVisible && !helpFits(width, height) {
		state.helpVisible = false
	}
	if state.chooser != nil {
		state.handleChooserKey(event)
		return false
	}
	if state.confirm != confirmNone {
		return state.handleConfirmation(event)
	}
	if state.windowChooser != nil {
		return state.handleWindowListKey(screen, event)
	}
	if state.hoverVisible {
		return state.handleHoverKey(screen, event)
	}
	if state.completionVisible {
		return state.handleCompletionKey(screen, event)
	}
	if state.searching {
		state.handleSearchKey(screen, event)
		return false
	}
	if state.editingRunArgs {
		state.handleRunArgumentsKey(event)
		return false
	}
	if state.windowSizing {
		return state.handleWindowSizing(screen, event)
	}
	if state.runSetupOpen {
		switch event.Key() {
		case tcell.KeyCtrlQ, tcell.KeyCtrlC:
			return state.requestQuit()
		case tcell.KeyF1:
			state.closeRunSetup()
		default:
			state.handleRunSetupKey(event)
			return false
		}
	}
	if state.helpVisible {
		switch event.Key() {
		case tcell.KeyEscape:
			state.helpVisible = false
		case tcell.KeyCtrlQ, tcell.KeyCtrlC:
			return state.requestQuit()
		default:
			handleNavigationKey(screen, state, event)
		}
		return false
	}
	switch event.Key() {
	case tcell.KeyCtrlQ, tcell.KeyCtrlC:
		return state.requestQuit()
	case tcell.KeyF2:
		state.requestSave()
	case tcell.KeyCtrlF:
		state.startSearch()
	case tcell.KeyCtrlG:
		state.findNext(screen)
	case tcell.KeyCtrlSpace:
		state.requestCompletion()
	case tcell.KeyF9:
		if event.Modifiers()&tcell.ModAlt != 0 {
			state.requestCurrentEntry()
			return false
		}
		if event.Modifiers()&tcell.ModCtrl != 0 {
			state.startJob(jobs.Run)
			return false
		}
		state.startJob(jobs.Build)
	case tcell.KeyCtrlT:
		state.startJob(jobs.Test)
	case tcell.KeyCtrlK:
		state.stopJob()
	case tcell.KeyF1:
		state.menuOpen = false
		state.openHelp(screen, false)
	case tcell.KeyF10:
		state.helpVisible = false
		state.menuOpen = !state.menuOpen
		state.menuItem = 0
	case tcell.KeyF3:
		if event.Modifiers() == tcell.ModAlt {
			state.menuOpen = false
			state.requestCloseWindow()
			return false
		}
		state.menuOpen = false
		state.helpVisible = false
		state.focusSeq++
		state.setFocus(focusTree)
	case tcell.KeyF12:
		state.requestF12Navigation(event)
	case tcell.KeyF5, tcell.KeyF6:
		state.handleWindowFunctionKey(screen, event)
	case tcell.KeyEscape:
		state.cancelPendingCompletion()
		state.closeBottomView()
		state.menuOpen = false
		state.helpVisible = false
	case tcell.KeyRune:
		if handleAltShortcut(screen, state, event) {
			return false
		}
		handleMenuMnemonic(state, event)
		if !state.menuOpen && !state.helpVisible && state.focus == focusEditor && state.buffer != nil && event.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 {
			state.insertRune(screen, event.Rune())
		}
	default:
		return handleNavigationKey(screen, state, event)
	}
	return false
}

func handleAltShortcut(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	if event.Modifiers()&tcell.ModAlt == 0 {
		return false
	}
	switch event.Rune() {
	case '0':
		state.openWindowList()
	case 'e', 'E':
		state.toggleBottomMode(screen)
	case 'i', 'I':
		state.requestHover()
	case 'c', 'C':
		state.requestCompletion()
	case 'd', 'D':
		state.requestNavigation(navigationDefinition)
	case 'r', 'R':
		state.requestNavigation(navigationReferences)
	default:
		return false
	}
	return true
}

func handleMenuMnemonic(state *shellState, event *tcell.EventKey) {
	if event.Modifiers()&tcell.ModAlt == 0 {
		return
	}
	for index, definition := range menuDefinitions {
		if unicode.ToUpper(event.Rune()) != definition.mnemonic {
			continue
		}
		state.menuIndex = index
		state.menuOpen = true
		state.menuItem = 0
		state.helpVisible = false
		return
	}
}

// handleRunSetupKey navigates the run settings submenu.
func (state *shellState) handleRunSetupKey(event *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyUp:
		state.runSetupItem = (state.runSetupItem + len(runSetupActionKeys) - 1) % len(runSetupActionKeys)
	case tcell.KeyDown:
		state.runSetupItem = (state.runSetupItem + 1) % len(runSetupActionKeys)
	case tcell.KeyEnter:
		state.runSetupAction(state.runSetupItem)
	case tcell.KeyEscape:
		state.closeRunSetup()
	case tcell.KeyLeft:
		state.closeRunSetup()
		state.menuOpen = true
		state.menuIndex = menuBuild
		state.menuItem = buildMenuSetup
	}
}

// openHelp shows a help page only when the dialog fits on screen.
func (state *shellState) openHelp(screen tcell.Screen, environment bool) {
	width, height := screen.Size()
	if !helpFits(width, height) {
		state.message = i18n.Text("msg_help_needs_a_terminal_of_at_least_16x5")
		return
	}
	state.helpEnvironment = environment
	state.helpScroll = 0
	state.helpVisible = true
}

func handleNavigationKey(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	key := event.Key()
	if state.menuOpen {
		switch key {
		case tcell.KeyLeft:
			state.menuIndex = (state.menuIndex + menuCount - 1) % menuCount
			state.menuItem = 0
		case tcell.KeyRight:
			if state.menuIndex == menuBuild && state.menuItem == buildMenuSetup {
				state.runMenuAction(buildMenuSetup)
			} else {
				state.menuIndex = (state.menuIndex + 1) % menuCount
				state.menuItem = 0
			}
		case tcell.KeyUp:
			state.moveMenuItem(-1)
		case tcell.KeyDown:
			state.moveMenuItem(1)
		case tcell.KeyEnter:
			state.menuOpen = false
			switch state.menuIndex {
			case menuFile:
				if state.menuItem == 0 {
					state.requestSave()
				} else {
					return state.requestQuit()
				}
			case menuSearch:
				switch state.menuItem {
				case 0:
					state.startSearch()
				case 1:
					state.findNext(screen)
				case 7:
					state.requestHover()
				case 3:
					state.openProblems(screen)
				case 8:
					state.requestCompletion()
				case 5:
					state.requestNavigation(navigationDefinition)
				case 6:
					state.requestNavigation(navigationReferences)
				}
			case menuBuild:
				state.runMenuAction(state.menuItem)
			case menuWindow:
				state.windowAction(screen, state.menuItem)
			case menuHelp:
				state.openHelp(screen, state.menuItem == 1)
			}
		}
		return false
	}
	if state.helpVisible {
		width, height := screen.Size()
		lines := len(helpRows(*state, min(width-2, 56)-4, width, height))
		page := max(1, helpPageSize(height, lines))
		state.helpScroll = min(state.helpScroll, max(0, lines-page))
		switch key {
		case tcell.KeyUp:
			state.helpScroll = max(0, state.helpScroll-1)
		case tcell.KeyDown:
			state.helpScroll = min(max(0, lines-page), state.helpScroll+1)
		case tcell.KeyPgUp:
			state.helpScroll = max(0, state.helpScroll-page)
		case tcell.KeyPgDn:
			state.helpScroll = min(max(0, lines-page), state.helpScroll+page)
		case tcell.KeyHome:
			state.helpScroll = 0
		case tcell.KeyEnd:
			state.helpScroll = max(0, lines-page)
		}
		return false
	}
	if state.focus == focusOutput {
		state.handleOutputKey(screen, event)
		return false
	}
	if state.focus == focusEditor {
		return handleEditorKey(screen, state, event)
	}
	switch key {
	case tcell.KeyLeft:
		state.collapseOrParent(screen)
	case tcell.KeyRight:
		state.expandSelected(screen)
	case tcell.KeyUp:
		state.moveSelection(screen, -1)
	case tcell.KeyDown:
		state.moveSelection(screen, 1)
	case tcell.KeyHome:
		state.moveSelectionTo(screen, 0)
	case tcell.KeyEnd:
		if state.tree != nil {
			state.moveSelectionTo(screen, len(state.tree.Visible())-1)
		}
	case tcell.KeyPgUp:
		state.moveSelection(screen, -max(1, state.treeArea(screen).height-2))
	case tcell.KeyPgDn:
		state.moveSelection(screen, max(1, state.treeArea(screen).height-2))
	case tcell.KeyEnter:
		state.openSelected(screen)
	}
	return false
}

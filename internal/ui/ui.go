// Package ui owns terminal input, layout, and rendering.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

// Run opens the terminal shell for a project and restores the terminal on exit.
func Run(projectRoot string) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("create terminal screen: %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("initialize terminal screen: %w", err)
	}
	defer screen.Fini()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)

	return runLoop(screen, projectRoot, interrupts)
}

func runLoop(screen tcell.Screen, projectRoot string, interrupts <-chan os.Signal) error {
	return runLoopWithSource(screen, projectRoot, interrupts, project.DiskSource{})
}

func runLoopWithSource(screen tcell.Screen, projectRoot string, interrupts <-chan os.Signal, source project.Source) error {
	return runLoopWithServices(screen, projectRoot, interrupts, source, project.DiskSaver{})
}

func runLoopWithServices(screen tcell.Screen, projectRoot string, interrupts <-chan os.Signal, source project.Source, saver project.Saver) error {
	events := make(chan tcell.Event, 1)
	stopEvents := make(chan struct{})
	eventsDone := make(chan struct{})
	go func() {
		screen.ChannelEvents(events, stopEvents)
		close(eventsDone)
	}()
	defer func() {
		close(stopEvents)
		<-eventsDone
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan workRequest, 64)
	results := make(chan workResult, 64)
	for range 2 {
		go projectWorker(ctx, source, saver, requests, results)
	}
	enqueue := func(request workRequest) bool {
		select {
		case requests <- request:
			return true
		default:
			return false
		}
	}

	state := newShellState(projectRoot, enqueue)
	render(screen, state)
	for {
		// Check cancellation before the next event so a flood of terminal
		// events cannot indefinitely delay a pending signal.
		select {
		case <-interrupts:
			return nil
		default:
		}
		var event tcell.Event
		select {
		case <-interrupts:
			return nil
		case result := <-results:
			state.applyResult(result)
			state.keepSelectionVisible(screen)
			state.ensureCursorVisible(screen)
			render(screen, state)
			continue
		case next, ok := <-events:
			if !ok {
				return fmt.Errorf("terminal screen closed")
			}
			event = next
		}
		if event == nil {
			return fmt.Errorf("terminal screen closed")
		}
		if handleEvent(screen, &state, event) {
			return nil
		}
		state.keepSelectionVisible(screen)
		render(screen, state)
	}
}

type shellState struct {
	projectRoot string
	helpVisible bool
	menuOpen    bool
	menuIndex   int
	menuItem    int
	tree        *project.Tree
	selected    int
	treeScroll  int
	focus       paneFocus
	document    *project.Document
	buffer      *editor.Buffer
	syntax      *syntaxCache
	fileScroll  int
	fileColumn  int
	opening     bool
	openSeq     uint64
	saving      bool
	saveSeq     uint64
	focusSeq    uint64
	searching   bool
	searchInput string
	searchQuery string
	message     string
	confirm     confirmAction
	pendingPath string
	pendingFile *workResult
	enqueue     func(workRequest) bool
}

type confirmAction uint8

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmOpen
	confirmLoaded
)

type paneFocus uint8

const (
	focusTree paneFocus = iota
	focusEditor
)

func handleEvent(screen tcell.Screen, state *shellState, event tcell.Event) bool {
	switch event := event.(type) {
	case *tcell.EventKey:
		return handleKey(screen, state, event)
	case *tcell.EventResize:
		screen.Sync()
		state.ensureCursorVisible(screen)
	case *tcell.EventInterrupt:
		return state.requestQuit()
	}
	return false
}

func handleKey(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	if state.confirm != confirmNone {
		return state.handleConfirmation(event)
	}
	if state.searching {
		state.handleSearchKey(screen, event)
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
	case tcell.KeyF1:
		state.menuOpen = false
		state.helpVisible = !state.helpVisible
	case tcell.KeyF10:
		state.helpVisible = false
		state.menuOpen = !state.menuOpen
		state.menuItem = 0
	case tcell.KeyF3:
		state.menuOpen = false
		state.helpVisible = false
		state.focusSeq++
		state.focus = focusTree
	case tcell.KeyF6:
		if (event.Modifiers() == tcell.ModNone || event.Modifiers() == tcell.ModCtrl) && !state.menuOpen && !state.helpVisible && state.document != nil {
			state.focusSeq++
			if state.focus == focusTree {
				state.focus = focusEditor
			} else {
				state.focus = focusTree
			}
		}
	case tcell.KeyEscape:
		state.menuOpen = false
		state.helpVisible = false
	case tcell.KeyRune:
		handleMenuMnemonic(state, event)
		if !state.menuOpen && !state.helpVisible && state.focus == focusEditor && state.buffer != nil && event.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 {
			state.insertRune(screen, event.Rune())
		}
	default:
		return handleNavigationKey(screen, state, event)
	}
	return false
}

func handleMenuMnemonic(state *shellState, event *tcell.EventKey) {
	if event.Modifiers()&tcell.ModAlt == 0 {
		return
	}
	switch event.Rune() {
	case 'f', 'F':
		state.menuIndex = menuFile
	case 's', 'S':
		state.menuIndex = menuSearch
	case 'h', 'H':
		state.menuIndex = menuHelp
	default:
		return
	}
	state.menuOpen = true
	state.menuItem = 0
	state.helpVisible = false
}

func handleNavigationKey(screen tcell.Screen, state *shellState, event *tcell.EventKey) bool {
	key := event.Key()
	if state.menuOpen {
		switch key {
		case tcell.KeyLeft:
			state.menuIndex = (state.menuIndex + menuCount - 1) % menuCount
			state.menuItem = 0
		case tcell.KeyRight:
			state.menuIndex = (state.menuIndex + 1) % menuCount
			state.menuItem = 0
		case tcell.KeyUp:
			state.menuItem = (state.menuItem + len(menuActions[state.menuIndex]) - 1) % len(menuActions[state.menuIndex])
		case tcell.KeyDown:
			state.menuItem = (state.menuItem + 1) % len(menuActions[state.menuIndex])
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
				if state.menuItem == 0 {
					state.startSearch()
				} else {
					state.findNext(screen)
				}
			case menuHelp:
				state.helpVisible = true
			}
		}
		return false
	}
	if state.helpVisible {
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

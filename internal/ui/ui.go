// Package ui owns terminal input, layout, and rendering.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/jobs"
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
	pump := startScreenEvents(screen)
	defer func() {
		if pump != nil {
			pump.stop()
		}
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

	manager := jobs.NewManager(projectRoot)
	defer manager.Close()
	manager.Warm()

	state := newShellState(projectRoot, enqueue)
	state.jobs = manager
	render(screen, state)
	for {
		// Check cancellation before the next event so a flood of terminal
		// events cannot indefinitely delay a pending signal.
		select {
		case <-interrupts:
			return nil
		default:
		}
		if state.terminalRun != nil {
			if err := handoffTerminal(ctx, screen, manager, &state, interrupts, &pump); err != nil {
				return err
			}
			continue
		}
		var event tcell.Event
		select {
		case <-interrupts:
			return nil
		case jobEvent := <-manager.Events():
			state.applyJobEvent(jobEvent)
			// Drain the burst a chatty command produces so RapidGo redraws
			// once per batch instead of once per output line.
			for draining := true; draining; {
				select {
				case next := <-manager.Events():
					state.applyJobEvent(next)
				default:
					draining = false
				}
			}
			state.keepOutputAnchored(screen)
			state.ensureCursorVisible(screen)
			render(screen, state)
			continue
		case result := <-results:
			state.applyResult(result)
			state.keepSelectionVisible(screen)
			state.ensureCursorVisible(screen)
			state.keepOutputAnchored(screen)
			render(screen, state)
			continue
		case next, ok := <-pump.events:
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
		state.keepOutputAnchored(screen)
		state.ensureCursorVisible(screen)
		render(screen, state)
	}
}

type shellState struct {
	projectRoot      string
	helpVisible      bool
	helpEnvironment  bool
	helpScroll       int
	menuOpen         bool
	menuIndex        int
	menuItem         int
	tree             *project.Tree
	selected         int
	treeScroll       int
	focus            paneFocus
	mainFocus        paneFocus // the tree or editor pane the output pane was reached from
	document         *project.Document
	buffer           *editor.Buffer
	syntax           *syntaxCache
	fileScroll       int
	fileColumn       int
	opening          bool
	openSeq          uint64
	saving           bool
	saveSeq          uint64
	focusSeq         uint64
	jobs             jobRunner
	toolchain        jobs.Toolchain
	toolchainErr     error
	views            map[jobs.Kind]*jobView
	visibleJob       jobs.Kind
	jobStarted       bool
	packages         []jobs.Package // every package, for resolving a diagnostic
	mainPackages     []jobs.Package // the runnable subset
	packagesLoaded   bool
	discovering      bool
	runIntent        runIntent
	runTarget        string
	runArguments     []string
	runArgumentText  string
	runArgumentDraft string
	editingRunArgs   bool
	terminalRun      *jobs.Request
	packageSeq       uint64 // bumped when the cached listing is invalidated
	discoverySeq     uint64 // generation the running listing started under
	chooser          *runChooser
	pendingJump      *diagnostic.Diagnostic // a jump waiting on the package listing
	pendingPosition  *jumpTarget            // where to put the caret once a file loads
	searching        bool
	searchInput      string
	searchQuery      string
	message          string
	confirm          confirmAction
	pendingPath      string
	pendingFile      *workResult
	enqueue          func(workRequest) bool
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
	focusOutput
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
	if state.searching {
		state.handleSearchKey(screen, event)
		return false
	}
	if state.editingRunArgs {
		state.handleRunArgumentsKey(event)
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
	case tcell.KeyF9:
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
		if state.helpVisible {
			state.helpVisible = false
		} else {
			state.openHelp(screen, false)
		}
	case tcell.KeyF10:
		state.helpVisible = false
		state.menuOpen = !state.menuOpen
		state.menuItem = 0
	case tcell.KeyF3:
		state.menuOpen = false
		state.helpVisible = false
		state.focusSeq++
		state.setFocus(focusTree)
	case tcell.KeyF6:
		if (event.Modifiers() == tcell.ModNone || event.Modifiers() == tcell.ModCtrl) && !state.menuOpen && !state.helpVisible {
			state.focusSeq++
			state.focusNextPane(screen)
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
	case 'b', 'B':
		state.menuIndex = menuBuild
	case 'h', 'H':
		state.menuIndex = menuHelp
	default:
		return
	}
	state.menuOpen = true
	state.menuItem = 0
	state.helpVisible = false
}

// openHelp shows a help page only when the dialog fits on screen.
func (state *shellState) openHelp(screen tcell.Screen, environment bool) {
	width, height := screen.Size()
	if !helpFits(width, height) {
		state.message = "Help needs a terminal of at least 16x5"
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
			case menuBuild:
				state.runMenuAction(state.menuItem)
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

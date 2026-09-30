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
	"github.com/hangxie/rapidgo/internal/gopls"
	"github.com/hangxie/rapidgo/internal/jobs"
	"github.com/hangxie/rapidgo/internal/project"
	"github.com/hangxie/rapidgo/internal/workspace"
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
	languageRequests := make(chan languageSnapshot, 1)
	hoverRequests := make(chan languageHoverRequest, 1)
	completionRequests := make(chan languageCompletionRequest, 1)
	navigationRequests := make(chan languageNavigationRequest, 1)
	languageEvents := make(chan languageEvent, 64)
	go languageWorker(ctx, projectRoot, languageRequests, hoverRequests, completionRequests, navigationRequests, languageEvents, func(ctx context.Context, root string) (languageSession, error) {
		return gopls.Start(ctx, root)
	})
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
	state.enqueueLanguage = (latestQueue[languageSnapshot]{channel: languageRequests}).replace
	state.enqueueHover = (latestQueue[languageHoverRequest]{channel: hoverRequests}).replace
	state.enqueueCompletion = (latestQueue[languageCompletionRequest]{channel: completionRequests}).replace
	state.enqueueNavigation = (latestQueue[languageNavigationRequest]{channel: navigationRequests}).replace
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
			drainJobEvents(manager, &state)
			state.keepOutputAnchored(screen)
			state.ensureCursorVisible(screen)
			render(screen, state)
			continue
		case result := <-results:
			state.applyResult(result)
			state.syncLanguage()
			state.keepSelectionVisible(screen)
			state.ensureCursorVisible(screen)
			state.keepOutputAnchored(screen)
			render(screen, state)
			continue
		case update := <-languageEvents:
			state.applyLanguageEvent(update)
			state.ensureHoverFits(screen)
			state.ensureCompletionFits(screen)
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
		state.syncLanguage()
		state.keepSelectionVisible(screen)
		state.keepOutputAnchored(screen)
		state.ensureCursorVisible(screen)
		render(screen, state)
	}
}

func drainJobEvents(manager *jobs.Manager, state *shellState) {
	// A chatty command should trigger one redraw per burst, not per line.
	for {
		select {
		case next := <-manager.Events():
			state.applyJobEvent(next)
		default:
			return
		}
	}
}

type shellState struct {
	workspace           *workspace.Workspace
	windowSizing        bool
	windowChooser       *windowChooser
	projectRoot         string
	helpVisible         bool
	helpEnvironment     bool
	helpScroll          int
	menuOpen            bool
	menuIndex           int
	menuItem            int
	runSetupOpen        bool
	runSetupItem        int
	tree                *project.Tree
	selected            int
	treeScroll          int
	focus               paneFocus
	mainFocus           paneFocus // tree or editor work area before output took focus
	document            *project.Document
	buffer              *editor.Buffer
	syntax              *syntaxCache
	fileScroll          int
	fileColumn          int
	opening             bool
	openSeq             uint64
	saving              bool
	saveSeq             uint64
	focusSeq            uint64
	jobs                jobRunner
	toolchain           jobs.Toolchain
	toolchainErr        error
	views               map[jobs.Kind]*jobView
	visibleJob          jobs.Kind
	jobStarted          bool
	packages            []jobs.Package // every package, for resolving a diagnostic
	mainPackages        []jobs.Package // the runnable subset
	packagesLoaded      bool
	discovering         bool
	runIntent           runIntent
	runTarget           string
	runArguments        []string
	runArgumentText     string
	runArgumentDraft    string
	editingRunArgs      bool
	terminalRun         *jobs.Request
	packageSeq          uint64 // bumped when the cached listing is invalidated
	discoverySeq        uint64 // generation the running listing started under
	entrySeq            uint64 // identifies the latest current-entry discovery
	entryDiscovering    bool
	entryPath           string
	entryPendingPath    string
	chooser             *runChooser
	pendingJump         *diagnostic.Diagnostic // a jump waiting on the package listing
	pendingPosition     *jumpTarget            // where to put the caret once a file loads
	searching           bool
	searchInput         string
	searchQuery         string
	message             string
	languageStatus      string
	languageErr         error
	languageQueued      languageSnapshot
	languageVersion     int
	languageSyncedText  string
	languageDiagnostics []diagnostic.Diagnostic
	languageReports     map[string]gopls.PublishedDiagnostics
	problems            []languageProblem
	bottomMode          bottomPaneMode
	locations           []gopls.Location
	locationKind        navigationKind
	locationSelected    int
	locationScroll      int
	errorSelected       int
	errorScroll         int
	enqueueLanguage     func(languageSnapshot)
	enqueueHover        func(languageHoverRequest)
	enqueueCompletion   func(languageCompletionRequest)
	enqueueNavigation   func(languageNavigationRequest)
	navigationSeq       uint64
	hoverSeq            uint64
	hoverVisible        bool
	hoverText           string
	hoverScroll         int
	completionSeq       uint64
	completionPending   bool
	completionVisible   bool
	completionItems     []gopls.CompletionItem
	completionSelected  int
	completionScroll    int
	completionRequest   languageCompletionRequest
	confirm             confirmAction
	pendingPath         string
	pendingFile         *workResult
	enqueue             func(workRequest) bool
}

type confirmAction uint8

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmOpen
	confirmLoaded
	confirmCloseWindow
)

type paneFocus uint8

const (
	focusTree paneFocus = iota
	focusEditor
	focusOutput
)

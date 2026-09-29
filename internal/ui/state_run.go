package ui

import (
	"path/filepath"
	"strings"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// runChooser asks which runnable target to use when nothing else identifies one.
type runChooser struct {
	targets  []string
	index    int
	run      bool // false when the dialog only records the default target
	terminal bool
}

// runChoice names a runnable package.
type runChoice struct {
	target string
	dir    string
}

// runIntent is what to do once the package listing is available.
type runIntent uint8

const (
	runIntentNone runIntent = iota
	runIntentStart
	runIntentTerminal
	runIntentChoose
	runIntentJump
)

// requestRun resolves what `go run` should execute.
func (state *shellState) requestRun() { state.withPackages(runIntentStart) }

// requestCurrentEntry runs the open main file with its sibling helpers.
func (state *shellState) requestCurrentEntry() {
	if state.document == nil {
		state.message = "Open a Go entry file before running"
		return
	}
	if state.jobs == nil {
		state.message = "Go commands are not available in this session"
		return
	}
	if state.buffer != nil && state.buffer.Dirty() {
		state.message = "Save the current file before running its entry"
		return
	}
	if state.entryDiscovering {
		if filepath.Clean(state.entryPath) == filepath.Clean(state.document.Path) {
			state.entryPendingPath = ""
		} else {
			state.entryPendingPath = state.document.Path
		}
		return
	}
	state.startEntryDiscovery(state.document.Path)
}

// startEntryDiscovery launches one current-entry lookup at a time.
func (state *shellState) startEntryDiscovery(path string) {
	state.entrySeq++
	state.entryDiscovering = true
	state.entryPath = path
	state.message = "Finding current entry files..."
	state.jobs.DiscoverEntry(path, state.entrySeq)
}

// requestTerminalRun resolves a main package for terminal handoff.
func (state *shellState) requestTerminalRun() { state.withPackages(runIntentTerminal) }

// chooseRunTarget rescans and always asks, so the default can be changed.
func (state *shellState) chooseRunTarget() {
	state.invalidatePackages()
	state.withPackages(runIntentChoose)
}

// invalidatePackages drops the cached listing, and any listing in flight.
func (state *shellState) invalidatePackages() {
	state.packageSeq++
	state.packagesLoaded = false
	state.packages, state.mainPackages = nil, nil
}

// withPackages runs an intent against the listing, waiting for `go list`.
func (state *shellState) withPackages(intent runIntent) {
	if state.jobs == nil {
		state.message = "Go commands are not available in this session"
		return
	}
	if state.packagesLoaded {
		state.applyIntent(intent, "")
		return
	}
	state.runIntent = intent
	if state.discovering {
		return // A listing is already running; its result decides what happens.
	}
	state.startDiscovery()
}

func (state *shellState) startDiscovery() {
	// Posted before the listing starts so it shows only while outstanding.
	state.discovering = true
	state.discoverySeq = state.packageSeq
	state.message = "Finding runnable packages..."
	state.jobs.Discover()
}

// applyIntent acts on the listing; gone names a target this listing lost.
func (state *shellState) applyIntent(intent runIntent, gone string) {
	switch intent {
	case runIntentChoose:
		state.selectRunTarget(gone)
	case runIntentJump:
		state.resumeJump()
	case runIntentTerminal:
		state.resolveRun(gone, true)
	default:
		state.resolveRun(gone, false)
	}
}

// selectRunTarget records the fallback for when the open file is not runnable.
func (state *shellState) selectRunTarget(gone string) {
	choices := state.runChoices()
	switch len(choices) {
	case 0:
		state.message = "No runnable Go package found"
	case 1:
		state.runTarget = choices[0].target
		state.message = "Default run package: " + state.runTarget + " (the only runnable package)"
	default:
		state.openRunChooser(false, gone, false)
	}
}

// resolveRun prefers the edited package, then the only one, then the default.
func (state *shellState) resolveRun(gone string, terminal bool) {
	choices := state.runChoices()
	if target := state.editedMainPackage(choices); target != "" {
		state.startRun(target, terminal)
		return
	}
	switch len(choices) {
	case 0:
		state.message = "No runnable Go package found"
	case 1:
		state.startRun(choices[0].target, terminal)
	default:
		if state.runTarget != "" {
			state.startRun(state.runTarget, terminal)
			return
		}
		state.openRunChooser(true, gone, terminal)
	}
}

// editedMainPackage returns the open file's package when it is runnable.
func (state *shellState) editedMainPackage(choices []runChoice) string {
	if state.document == nil {
		return ""
	}
	path := filepath.Clean(state.document.Path)
	for _, choice := range choices {
		if filepath.Clean(choice.dir) == filepath.Dir(path) {
			return choice.target
		}
	}
	return ""
}

// runChoices keeps Run and its default selection package-oriented.
func (state *shellState) runChoices() []runChoice {
	var choices []runChoice
	for _, listed := range state.mainPackages {
		choices = append(choices, runChoice{target: state.targetFor(listed), dir: listed.Dir})
	}
	return choices
}

// applyEntryDiscovery starts the latest entry once its source list is ready.
func (state *shellState) applyEntryDiscovery(event jobs.Event) {
	if !state.entryDiscovering || event.ID != state.entrySeq {
		return
	}
	state.entryDiscovering = false
	if state.entryPendingPath != "" {
		path := state.entryPendingPath
		state.entryPendingPath = ""
		if state.document == nil {
			state.message = ""
			return
		}
		current := filepath.Clean(state.document.Path)
		if current == filepath.Clean(path) && current != filepath.Clean(event.EntryPath) {
			state.startEntryDiscovery(path)
			return
		}
		if current != filepath.Clean(event.EntryPath) {
			state.message = ""
			return
		}
	}
	if state.document == nil || filepath.Clean(event.EntryPath) != filepath.Clean(state.document.Path) {
		if state.message == "Finding current entry files..." {
			state.message = ""
		}
		return
	}
	if state.buffer != nil && state.buffer.Dirty() {
		state.message = "Save the current file before running its entry"
		return
	}
	if event.Err != nil {
		state.message = "Find current entry: " + event.Err.Error()
		return
	}
	if len(event.EntryFiles) == 0 && event.EntryTarget == "" {
		state.message = "Current file must contain func main() in a Go package"
		return
	}
	request := jobs.Request{Kind: jobs.Run, Dir: filepath.Dir(event.EntryPath), Arguments: append([]string(nil), state.runArguments...)}
	if event.EntryTarget != "" {
		request.Target = "."
	}
	for _, path := range event.EntryFiles {
		request.Files = append(request.Files, "./"+filepath.Base(path))
	}
	state.startRequest(request)
}

// targetFor names a package relative to the project root where it can.
func (state *shellState) targetFor(listed jobs.Package) string {
	if target, ok := relativeTarget(state.projectRoot, listed.Dir); ok {
		return target
	}
	return listed.ImportPath
}

// relativeTarget names a path within root for Go package and file arguments.
func relativeTarget(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if relative == "." {
		return ".", true
	}
	return "./" + filepath.ToSlash(relative), true
}

func (state *shellState) startRun(target string, terminal bool) {
	request := jobs.Request{Kind: jobs.Run, Target: target, Arguments: append([]string(nil), state.runArguments...)}
	if terminal {
		state.terminalRun = &request
		state.message = "Preparing terminal for " + request.Command()
		return
	}
	state.startRequest(request)
}

// editRunArguments opens the session's run-argument prompt.

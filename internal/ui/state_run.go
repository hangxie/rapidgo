package ui

import (
	"path/filepath"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

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
	runIntentCurrentEntry
)

// requestRun resolves what `go run` should execute.
func (state *shellState) requestRun() { state.withPackages(runIntentStart) }

// requestCurrentEntry runs the open main file with its sibling helpers.
func (state *shellState) requestCurrentEntry() {
	if state.document == nil {
		state.message = "Open a Go entry file before running"
		return
	}
	state.withPackages(runIntentCurrentEntry)
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
	case runIntentCurrentEntry:
		state.runCurrentEntry()
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
	if target := state.editedMainPackage(); target != "" {
		state.startRun(target, terminal)
		return
	}
	choices := state.runChoices()
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
func (state *shellState) editedMainPackage() string {
	if state.document == nil {
		return ""
	}
	path := filepath.Clean(state.document.Path)
	for _, choice := range state.runChoices() {
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

// runCurrentEntry uses the selected Go sources around the open main file.
func (state *shellState) runCurrentEntry() {
	if state.document == nil {
		state.message = "Open a Go entry file before running"
		return
	}
	path := filepath.Clean(state.document.Path)
	for _, listed := range state.mainPackages {
		if filepath.Clean(listed.Dir) != filepath.Dir(path) {
			continue
		}
		files := listed.FilesFor(filepath.Base(path))
		if len(files) == 0 {
			break
		}
		request := jobs.Request{Kind: jobs.Run, Arguments: append([]string(nil), state.runArguments...)}
		for _, name := range files {
			request.Files = append(request.Files, state.fileTarget(filepath.Join(listed.Dir, name)))
		}
		state.startRequest(request)
		return
	}
	state.message = "Current file must contain func main() in a Go package"
}

// fileTarget names a source file relative to the project root when possible.
func (state *shellState) fileTarget(path string) string {
	relative, err := filepath.Rel(state.projectRoot, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || len(relative) > 2 && relative[:3] == ".."+string(filepath.Separator) {
		return path
	}
	return "./" + filepath.ToSlash(relative)
}

// targetFor names a package relative to the project root where it can.
func (state *shellState) targetFor(listed jobs.Package) string {
	relative, err := filepath.Rel(state.projectRoot, listed.Dir)
	switch {
	case err != nil || relative == ".." || filepath.IsAbs(relative):
		return listed.ImportPath
	case relative == ".":
		return "."
	case len(relative) > 2 && relative[:3] == ".."+string(filepath.Separator):
		return listed.ImportPath
	default:
		return "./" + filepath.ToSlash(relative)
	}
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
func (state *shellState) editRunArguments() {
	state.menuOpen = false
	state.helpVisible = false
	state.editingRunArgs = true
	state.runArgumentDraft = state.runArgumentText
	state.message = "Run arguments: Enter saves, Esc cancels; quotes group spaces"
}

// handleRunArgumentsKey edits the prompt and commits only valid arguments.
func (state *shellState) handleRunArgumentsKey(event *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyEscape:
		state.editingRunArgs = false
		state.message = "Run arguments unchanged"
	case tcell.KeyEnter:
		args, err := jobs.ParseArguments(state.runArgumentDraft)
		if err != nil {
			state.message = "Run arguments: " + err.Error()
			return
		}
		state.runArguments = args
		state.runArgumentText = state.runArgumentDraft
		state.editingRunArgs = false
		state.message = "Run arguments saved for this session"
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		clusters := uniseg.NewGraphemes(state.runArgumentDraft)
		last := 0
		for clusters.Next() {
			last, _ = clusters.Positions()
		}
		state.runArgumentDraft = state.runArgumentDraft[:last]
	case tcell.KeyRune:
		if event.Modifiers()&(tcell.ModAlt|tcell.ModCtrl) == 0 && !unicode.IsControl(event.Rune()) {
			state.runArgumentDraft += string(event.Rune())
		}
	}
}

// openRunChooser lists the runnable targets, starting on the current default.
func (state *shellState) openRunChooser(run bool, gone string, terminal bool) {
	choices := state.runChoices()
	targets := make([]string, 0, len(choices))
	selected := 0
	for _, choice := range choices {
		target := choice.target
		if target == state.runTarget {
			selected = len(targets)
		}
		targets = append(targets, target)
	}
	state.chooser = &runChooser{targets: targets, index: selected, run: run, terminal: terminal}
	verb := "set the default run package"
	if run {
		verb = "run one"
		if terminal {
			verb = "run one in the terminal"
		}
	}
	reason := "Several runnable packages"
	if gone != "" {
		reason = "Default run package " + gone + " is gone"
	}
	state.message = reason + ": Up/Down and Enter to " + verb + ", Esc to cancel"
}

// handleChooserKey drives the dialog and remembers the choice for the session.
func (state *shellState) handleChooserKey(event *tcell.EventKey) {
	chooser := state.chooser
	switch event.Key() {
	case tcell.KeyEscape:
		cancelled := "no package was run"
		if !chooser.run {
			cancelled = "the default run package is unchanged"
		}
		state.chooser = nil
		state.message = "Cancelled; " + cancelled
	case tcell.KeyUp:
		chooser.index = (chooser.index + len(chooser.targets) - 1) % len(chooser.targets)
	case tcell.KeyDown:
		chooser.index = (chooser.index + 1) % len(chooser.targets)
	case tcell.KeyEnter:
		target := chooser.targets[chooser.index]
		runIt := chooser.run
		state.chooser = nil
		state.runTarget = target
		if runIt {
			state.startRun(target, chooser.terminal)
			return
		}
		state.message = "Default run package: " + target
	}
}

// forgetMissingTarget drops a target the listing lost, and returns it.
func (state *shellState) forgetMissingTarget() string {
	if state.runTarget == "" {
		return ""
	}
	for _, choice := range state.runChoices() {
		if choice.target == state.runTarget {
			return ""
		}
	}
	gone := state.runTarget
	state.runTarget = ""
	return gone
}

// applyDiscovery records the packages and resumes a waiting Run.
func (state *shellState) applyDiscovery(event jobs.Event) {
	state.discovering = false
	if state.discoverySeq != state.packageSeq {
		// The project changed while this ran, so it is already out of date.
		if state.runIntent != runIntentNone {
			state.startDiscovery()
		}
		return
	}
	if event.Err != nil {
		state.runIntent = runIntentNone
		state.message = "Find runnable packages: " + event.Err.Error()
		return
	}
	state.packages = event.Packages
	state.mainPackages = state.mainPackages[:0]
	for _, listed := range event.Packages {
		if listed.Name == "main" {
			state.mainPackages = append(state.mainPackages, listed)
		}
	}
	state.packagesLoaded = true
	gone := state.forgetMissingTarget()
	if intent := state.runIntent; intent != runIntentNone {
		state.runIntent = runIntentNone
		state.applyIntent(intent, gone)
	}
}

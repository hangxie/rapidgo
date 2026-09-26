package ui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// runChooser asks which main package to run when nothing else identifies one.
type runChooser struct {
	targets []string
	index   int
	run     bool // false when the dialog only records the default target
}

// runIntent is what to do once the package listing is available.
type runIntent uint8

const (
	runIntentNone runIntent = iota
	runIntentStart
	runIntentChoose
)

// requestRun resolves what `go run` should execute.
func (state *shellState) requestRun() { state.withPackages(runIntentStart) }

// chooseRunTarget rescans and always asks, so the session default can be
// changed and a package added since the last listing shows up.
func (state *shellState) chooseRunTarget() {
	state.invalidatePackages()
	state.withPackages(runIntentChoose)
}

// invalidatePackages drops the cached listing so the next run rescans. The
// generation also discards a listing that is already running.
func (state *shellState) invalidatePackages() {
	state.packageSeq++
	state.packagesLoaded = false
	state.mainPackages = nil
}

// withPackages runs an intent against the module's runnable packages, waiting
// for `go list` when no listing is cached.
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

// applyIntent acts on the listing. gone names a remembered target this listing
// lost, scoped to one attempt so a later one cannot repeat it.
func (state *shellState) applyIntent(intent runIntent, gone string) {
	if intent == runIntentChoose {
		state.selectRunTarget(gone)
		return
	}
	state.resolveRun(gone)
}

// selectRunTarget records the fallback Run uses when the open file is not
// itself runnable; the open file still wins.
func (state *shellState) selectRunTarget(gone string) {
	switch len(state.mainPackages) {
	case 0:
		state.message = "No runnable Go package found"
	case 1:
		state.runTarget = state.targetFor(state.mainPackages[0])
		state.message = "Run target: " + state.runTarget + " (the only runnable package)"
	default:
		state.openRunChooser(false, gone)
	}
}

// resolveRun prefers the main package being edited, then the module's only
// one, then the session default, and otherwise asks.
func (state *shellState) resolveRun(gone string) {
	if target := state.editedMainPackage(); target != "" {
		state.startRun(target)
		return
	}
	switch len(state.mainPackages) {
	case 0:
		state.message = "No runnable Go package found"
	case 1:
		state.startRun(state.targetFor(state.mainPackages[0]))
	default:
		if state.runTarget != "" {
			state.startRun(state.runTarget)
			return
		}
		state.openRunChooser(true, gone)
	}
}

// editedMainPackage returns the run target for the open file's package when
// that package is runnable.
func (state *shellState) editedMainPackage() string {
	if state.document == nil {
		return ""
	}
	directory := filepath.Clean(filepath.Dir(state.document.Path))
	for _, listed := range state.mainPackages {
		if filepath.Clean(listed.Dir) == directory {
			return state.targetFor(listed)
		}
	}
	return ""
}

// targetFor names a package relative to the project root, falling back to the
// import path for anything outside it.
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

func (state *shellState) startRun(target string) {
	state.startRequest(jobs.Request{Kind: jobs.Run, Target: target})
}

// openRunChooser lists the runnable packages, starting on the current default.
func (state *shellState) openRunChooser(run bool, gone string) {
	targets := make([]string, 0, len(state.mainPackages))
	selected := 0
	for _, listed := range state.mainPackages {
		target := state.targetFor(listed)
		if target == state.runTarget {
			selected = len(targets)
		}
		targets = append(targets, target)
	}
	state.chooser = &runChooser{targets: targets, index: selected, run: run}
	verb := "set the run target"
	if run {
		verb = "run one"
	}
	reason := "Several runnable packages"
	if gone != "" {
		reason = "Run target " + gone + " is gone"
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
			cancelled = "the run target is unchanged"
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
			state.startRun(target)
			return
		}
		state.message = "Run target: " + target
	}
}

// forgetMissingTarget drops a remembered target the listing no longer has and
// returns it, so the resolution that follows can say what went.
func (state *shellState) forgetMissingTarget() string {
	if state.runTarget == "" {
		return ""
	}
	for _, listed := range state.mainPackages {
		if state.targetFor(listed) == state.runTarget {
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
	state.mainPackages = event.Packages
	state.packagesLoaded = true
	gone := state.forgetMissingTarget()
	if intent := state.runIntent; intent != runIntentNone {
		state.runIntent = runIntentNone
		state.applyIntent(intent, gone)
	}
}

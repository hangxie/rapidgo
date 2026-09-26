package ui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/jobs"
)

// runChooser asks which main package to run when a module has several and
// nothing else identifies one.
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

// chooseRunTarget always asks, so the default chosen earlier in a session can
// be changed without opening a file in another runnable package.
func (state *shellState) chooseRunTarget() { state.withPackages(runIntentChoose) }

// withPackages runs an intent against the module's runnable packages. The
// listing comes from `go list`, so the first request of a session waits for
// it; later ones reuse it.
func (state *shellState) withPackages(intent runIntent) {
	if state.jobs == nil {
		state.message = "Go commands are not available in this session"
		return
	}
	if state.packagesLoaded {
		state.applyIntent(intent)
		return
	}
	state.runIntent = intent
	if state.discovering {
		return
	}
	// The message goes up before the listing starts so it is only visible
	// while the answer is genuinely outstanding.
	state.discovering = true
	state.message = "Finding runnable packages..."
	state.jobs.Discover()
}

func (state *shellState) applyIntent(intent runIntent) {
	if intent == runIntentChoose {
		state.selectRunTarget()
		return
	}
	state.resolveRun()
}

// selectRunTarget records which package Run should default to. The open file
// still wins when it belongs to a runnable package, so this sets the fallback
// rather than a permanent override.
func (state *shellState) selectRunTarget() {
	switch len(state.mainPackages) {
	case 0:
		state.message = "No runnable Go package found"
	case 1:
		state.runTarget = state.targetFor(state.mainPackages[0])
		state.message = "Run target: " + state.runTarget + " (the only runnable package)"
	default:
		state.openRunChooser(false)
	}
}

// resolveRun picks a run target: the main package being edited, then the only
// main package in the module, then the one chosen earlier this session, and
// otherwise it asks.
func (state *shellState) resolveRun() {
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
		state.openRunChooser(true)
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

// targetFor names a package the way a person would type it, relative to the
// project root, and falls back to the import path for anything outside it.
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

// openRunChooser lists the runnable packages, starting on the current default
// so reopening the dialog shows what is in effect.
func (state *shellState) openRunChooser(run bool) {
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
	state.message = "Several runnable packages: Up/Down and Enter to " + verb + ", Esc to cancel"
}

// handleChooserKey drives the run-target dialog. The choice is remembered for
// the rest of the session.
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

// applyDiscovery records the runnable packages and resumes a Run that was
// waiting for them.
func (state *shellState) applyDiscovery(event jobs.Event) {
	state.discovering = false
	if event.Err != nil {
		state.runIntent = runIntentNone
		state.message = "Find runnable packages: " + event.Err.Error()
		return
	}
	state.mainPackages = event.Packages
	state.packagesLoaded = true
	if intent := state.runIntent; intent != runIntentNone {
		state.runIntent = runIntentNone
		state.applyIntent(intent)
	}
}

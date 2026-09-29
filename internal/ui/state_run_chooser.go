package ui

import (
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/jobs"
)

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

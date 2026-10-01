package ui

import (
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

func (state *shellState) editRunArguments() {
	state.menuOpen = false
	state.helpVisible = false
	state.editingRunArgs = true
	state.runArgumentDraft = state.runArgumentText
	state.message = i18n.Text("msg_run_arguments_enter_saves_esc_cancels_quotes_group_spaces")
}

// handleRunArgumentsKey edits the prompt and commits only valid arguments.
func (state *shellState) handleRunArgumentsKey(event *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyEscape:
		state.editingRunArgs = false
		state.message = i18n.Text("msg_run_arguments_unchanged")
	case tcell.KeyEnter:
		args, err := jobs.ParseArguments(state.runArgumentDraft)
		if err != nil {
			state.message = i18n.Format("msg_run_arguments_s", err.Error())
			return
		}
		state.runArguments = args
		state.runArgumentText = state.runArgumentDraft
		state.editingRunArgs = false
		state.message = i18n.Text("msg_run_arguments_saved_for_this_session")
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
	verb := i18n.Text("msg_set_the_default_run_package")
	if run {
		verb = i18n.Text("msg_run_one")
		if terminal {
			verb = i18n.Text("msg_run_one_in_the_terminal")
		}
	}
	reason := i18n.Text("msg_several_runnable_packages")
	if gone != "" {
		reason = i18n.Format("msg_default_run_package_s_is_gone", gone)
	}
	state.message = i18n.Format("msg_s_up_down_and_enter_to_s_esc_to_cancel", reason, verb)
}

// handleChooserKey drives the dialog and remembers the choice for the session.
func (state *shellState) handleChooserKey(event *tcell.EventKey) {
	chooser := state.chooser
	switch event.Key() {
	case tcell.KeyEscape:
		cancelled := i18n.Text("msg_no_package_was_run")
		if !chooser.run {
			cancelled = i18n.Text("msg_the_default_run_package_is_unchanged")
		}
		state.chooser = nil
		state.message = i18n.Format("msg_cancelled_s", cancelled)
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
		state.message = i18n.Format("msg_default_run_package_s", target)
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
		state.message = i18n.Format("msg_find_runnable_packages_s", event.Err.Error())
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

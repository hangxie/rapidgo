package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/jobs"
	"github.com/hangxie/rapidgo/internal/project"
)

const runRoot = "/tmp/project"

func mainPackage(relative string) jobs.Package {
	return jobs.Package{
		Name:       "main",
		Dir:        filepath.Join(runRoot, filepath.FromSlash(relative)),
		ImportPath: "example.com/module/" + relative,
	}
}

// runState builds a shell whose package listing answers immediately.
func runState(t *testing.T, packages ...jobs.Package) (*shellState, *fakeRunner) {
	t.Helper()
	runner := &fakeRunner{packages: packages}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	runner.state = state
	return state, runner
}

func TestRunResolvesTheOnlyMainPackage(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/rapidgo"))
	state.startJob(jobs.Run)

	assert.Equal(t, 1, runner.discovered)
	require.Len(t, runner.started, 1)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "./cmd/rapidgo"}, runner.started[0])
	assert.Equal(t, "Starting go run ./cmd/rapidgo", state.message)

	// The listing is reused, so a second run does not ask go list again.
	state.startJob(jobs.Run)
	assert.Equal(t, 1, runner.discovered)
	assert.Len(t, runner.started, 2)
}

func TestRunPrefersThePackageBeingEdited(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/admin"), mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.document = &project.Document{Path: filepath.Join(runRoot, "cmd", "worker", "main.go")}
	state.startJob(jobs.Run)

	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/worker", runner.started[0].Target)
	assert.Nil(t, state.chooser, "editing a main package answers the question on its own")

	// A file outside any main package falls through to the chooser.
	state.document = &project.Document{Path: filepath.Join(runRoot, "internal", "ui", "render.go")}
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)
	assert.Equal(t, []string{"./cmd/admin", "./cmd/server", "./cmd/worker"}, state.chooser.targets)
}

func TestRunChooserRemembersTheChoice(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)
	assert.Empty(t, runner.started, "nothing runs until a package is chosen")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.Equal(t, 1, state.chooser.index)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.Equal(t, 1, state.chooser.index, "Up from the first entry wraps to the last")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Nil(t, state.chooser)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/worker", runner.started[0].Target)

	// The remembered target skips the chooser next time.
	state.startJob(jobs.Run)
	assert.Nil(t, state.chooser)
	require.Len(t, runner.started, 2)
	assert.Equal(t, "./cmd/worker", runner.started[1].Target)
}

func TestRunChooserCancels(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.Nil(t, state.chooser)
	assert.Empty(t, runner.started)
	assert.Equal(t, "Cancelled; no package was run", state.message)
	assert.Empty(t, state.runTarget, "cancelling must not remember a target")
}

func TestRunWithoutAnyMainPackage(t *testing.T) {
	t.Parallel()

	state, runner := runState(t)
	state.startJob(jobs.Run)
	assert.Empty(t, runner.started)
	assert.Equal(t, "No runnable Go package found", state.message)
}

func TestRunReportsDiscoveryFailure(t *testing.T) {
	t.Parallel()

	state, runner := runState(t)
	runner.listErr = errors.New("go list: go.mod file not found")
	state.startJob(jobs.Run)

	assert.Empty(t, runner.started)
	assert.False(t, state.packagesLoaded, "a failed listing must not be cached")
	assert.Equal(t, "Find runnable packages: go list: go.mod file not found", state.message)

	// The next Run asks again rather than staying stuck.
	runner.listErr = nil
	runner.packages = []jobs.Package{mainPackage("cmd/rapidgo")}
	state.startJob(jobs.Run)
	assert.Equal(t, 2, runner.discovered)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/rapidgo", runner.started[0].Target)
}

func TestRunWaitsForASlowListing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{} // Discover does not answer until the event arrives.
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.startJob(jobs.Run)
	assert.Equal(t, "Finding runnable packages...", state.message)
	assert.True(t, state.discovering)

	state.startJob(jobs.Run) // A second press must not start a second listing.
	assert.Equal(t, 1, runner.discovered)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/rapidgo")}})
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/rapidgo", runner.started[0].Target)
	assert.False(t, state.discovering)
}

func TestRunTargetNaming(t *testing.T) {
	t.Parallel()

	state := &shellState{projectRoot: runRoot}
	assert.Equal(t, ".", state.targetFor(jobs.Package{Dir: runRoot, ImportPath: "example.com/module"}))
	assert.Equal(t, "./cmd/tool", state.targetFor(mainPackage("cmd/tool")))
	// A package outside the project root cannot be named relative to it.
	assert.Equal(t, "example.com/other", state.targetFor(jobs.Package{Dir: "/tmp/elsewhere", ImportPath: "example.com/other"}))
}

func TestRenderRunChooser(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)
	state.chooser.index = 1
	render(screen, *state)

	var dialog strings.Builder
	for y := range 24 {
		dialog.WriteString(rowText(screen, y, 0, 80))
		dialog.WriteString("\n")
	}
	assert.Contains(t, dialog.String(), "Run which package?")
	assert.Contains(t, dialog.String(), "./cmd/server")
	assert.Contains(t, dialog.String(), "./cmd/worker")
	assert.Contains(t, dialog.String(), "Up/Down Move  Enter Run  Esc Cancel")

	// The highlighted row uses the selection color.
	boxWidth, boxHeight := 56, 7
	x, y := (80-boxWidth)/2, (24-boxHeight)/2
	assertCellColors(t, screen, x+2, y+3, turboBlack, turboGreen)
	assertCellColors(t, screen, x+2, y+2, turboBlack, turboLightGray)
}

// A tiny terminal must not panic or draw outside the screen.
func TestRenderRunChooserAtNarrowSizes(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.startJob(jobs.Run)
	for _, size := range [][2]int{{1, 1}, {12, 3}, {19, 6}, {30, 8}, {80, 24}} {
		screen.SetSize(size[0], size[1])
		render(screen, *state)
	}
}

func TestRunTargetCanBeChangedFromTheMenu(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	// Run Target asks even though a target is already remembered, and it
	// starts on the one in effect.
	state.runMenuAction(buildMenuTarget)
	require.NotNil(t, state.chooser)
	assert.Equal(t, 1, state.chooser.index)
	assert.False(t, state.chooser.run, "choosing a target must not launch it")

	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyUp, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Equal(t, "./cmd/server", state.runTarget)
	assert.Equal(t, "Run target: ./cmd/server", state.message)
	assert.Empty(t, runner.started, "setting the target does not run anything")

	// The new default is what Ctrl+F9 then runs.
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/server", runner.started[0].Target)
}

func TestRunTargetMenuCancelKeepsTheCurrentTarget(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	state.runMenuAction(buildMenuTarget)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, 0)))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.Equal(t, "./cmd/worker", state.runTarget)
	assert.Equal(t, "Cancelled; the run target is unchanged", state.message)
}

func TestRunTargetMenuWithOneOrNoPackages(t *testing.T) {
	t.Parallel()

	single, runner := runState(t, mainPackage("cmd/rapidgo"))
	single.runMenuAction(buildMenuTarget)
	assert.Nil(t, single.chooser, "one package needs no dialog")
	assert.Equal(t, "./cmd/rapidgo", single.runTarget)
	assert.Equal(t, "Run target: ./cmd/rapidgo (the only runnable package)", single.message)
	assert.Empty(t, runner.started)

	none, _ := runState(t)
	none.runMenuAction(buildMenuTarget)
	assert.Equal(t, "No runnable Go package found", none.message)
}

// Choosing a target before the listing arrives must not start a run.
func TestRunTargetMenuWaitsForTheListing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.runMenuAction(buildMenuTarget)
	assert.Equal(t, "Finding runnable packages...", state.message)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/server"), mainPackage("cmd/worker")}})
	require.NotNil(t, state.chooser)
	assert.False(t, state.chooser.run)
	assert.Empty(t, runner.started)
}

// The open file still wins over a target set from the menu, because the run
// target is the fallback rather than a permanent override.
func TestEditedPackageOutranksTheChosenTarget(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/server"
	state.document = &project.Document{Path: filepath.Join(runRoot, "cmd", "worker", "main.go")}

	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/worker", runner.started[0].Target)
	assert.Equal(t, "./cmd/server", state.runTarget, "running the edited package leaves the default alone")
}

// Build -> Run Target only records a target, so the dialog must not offer to
// run the package it highlights.
func TestRenderRunChooserNamesWhatEnterDoes(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runMenuAction(buildMenuTarget)
	require.NotNil(t, state.chooser)
	render(screen, *state)

	var dialog strings.Builder
	for y := range 24 {
		dialog.WriteString(rowText(screen, y, 0, 80))
		dialog.WriteString("\n")
	}
	assert.Contains(t, dialog.String(), "Select run target")
	assert.NotContains(t, dialog.String(), "Run which package?")
	assert.Contains(t, dialog.String(), "Up/Down Move  Enter Select  Esc Cancel")
	assert.NotContains(t, dialog.String(), "Enter Run")
	assert.Contains(t, state.message, "Up/Down and Enter to set the run target")
}

// Saving can add or remove a main package, and RapidGo does not watch the
// filesystem, so the cached listing must not outlive a write.
func TestSaveRefreshesTheRunnablePackages(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/rapidgo"))
	state.startJob(jobs.Run)
	require.Equal(t, 1, runner.discovered)
	state.startJob(jobs.Run)
	require.Equal(t, 1, runner.discovered, "the listing is cached until something changes")

	state.invalidatePackages()
	runner.packages = []jobs.Package{mainPackage("cmd/rapidgo"), mainPackage("cmd/tool")}
	state.document = &project.Document{Path: filepath.Join(runRoot, "cmd", "tool", "main.go")}
	state.startJob(jobs.Run)
	assert.Equal(t, 2, runner.discovered)
	require.Len(t, runner.started, 3)
	assert.Equal(t, "./cmd/tool", runner.started[2].Target, "the package added by the save is runnable")
}

func TestSaveResultInvalidatesTheListing(t *testing.T) {
	t.Parallel()

	state, _ := runState(t, mainPackage("cmd/rapidgo"))
	state.startJob(jobs.Run)
	require.True(t, state.packagesLoaded)

	document := project.Document{Path: filepath.Join(runRoot, "main.go"), Text: "package main\n"}
	buffer, err := editor.New(document.Text)
	require.NoError(t, err)
	state.document = &document
	state.buffer = buffer
	state.saving = true
	state.saveSeq = 1
	state.applySaveResult(workResult{
		request: workRequest{kind: saveFile, path: document.Path, seq: 1, revision: buffer.Revision()},
		saved:   project.SaveResult{Content: "package main\n"},
	})
	assert.Contains(t, state.message, "Saved")
	assert.False(t, state.packagesLoaded, "a completed save drops the cached listing")
	assert.Empty(t, state.mainPackages)
}

// Choosing a target is when a package added since the last listing should show
// up, so the menu action rescans.
func TestRunTargetMenuRescans(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/server"))
	state.startJob(jobs.Run)
	require.Equal(t, 1, runner.discovered)

	runner.packages = []jobs.Package{mainPackage("cmd/server"), mainPackage("cmd/worker")}
	state.runMenuAction(buildMenuTarget)
	assert.Equal(t, 2, runner.discovered)
	require.NotNil(t, state.chooser)
	assert.Equal(t, []string{"./cmd/server", "./cmd/worker"}, state.chooser.targets)
}

func TestRememberedTargetIsForgottenWhenItDisappears(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	// The package is renamed away, so the remembered target must not be run.
	state.invalidatePackages()
	// go list reports packages sorted by import path.
	runner.packages = []jobs.Package{mainPackage("cmd/runner"), mainPackage("cmd/server")}
	state.startJob(jobs.Run)
	assert.Empty(t, state.runTarget)
	require.NotNil(t, state.chooser, "a gone target falls back to asking")
	assert.Equal(t, []string{"./cmd/runner", "./cmd/server"}, state.chooser.targets)
	assert.Contains(t, state.message, "is gone")

	// A target that survives the rescan is kept.
	state.chooser = nil
	state.runTarget = "./cmd/server"
	state.invalidatePackages()
	state.startJob(jobs.Run)
	assert.Equal(t, "./cmd/server", state.runTarget)
	assert.Nil(t, state.chooser)
}

// A listing that was already running when the project changed describes the
// project as it was, so its result must not become the cache.
func TestSaveDuringDiscoveryDiscardsTheStaleListing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{} // Discover does not answer on its own.
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.startJob(jobs.Run)
	require.Equal(t, 1, runner.discovered)
	require.True(t, state.discovering)

	// A save lands while go list is still running.
	state.invalidatePackages()

	// The in-flight listing finishes with what the project looked like before.
	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/old")}})
	assert.False(t, state.packagesLoaded, "the stale listing must not be cached")
	assert.Empty(t, state.mainPackages)
	assert.Empty(t, runner.started, "nothing runs on a listing from before the save")
	assert.Equal(t, 2, runner.discovered, "the waiting run asks again")

	// The fresh listing is the one that counts.
	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/new")}})
	assert.True(t, state.packagesLoaded)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/new", runner.started[0].Target)
}

// With nothing waiting on it, a stale listing is dropped without asking again;
// the next run rescans because the cache stayed empty.
func TestStaleListingWithoutAWaitingRunIsDropped(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.startJob(jobs.Run)
	state.runIntent = runIntentNone // the pending run was satisfied another way
	state.invalidatePackages()

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/old")}})
	assert.False(t, state.packagesLoaded)
	assert.Equal(t, 1, runner.discovered, "no one is waiting, so nothing is relaunched")

	runner.packages = []jobs.Package{mainPackage("cmd/new")}
	runner.state = state
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/new", runner.started[0].Target)
}

// Run Target promises a rescan, which has to hold even mid-listing.
func TestRunTargetDuringDiscoveryRescans(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.startJob(jobs.Run)
	state.runMenuAction(buildMenuTarget)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/old")}})
	assert.False(t, state.packagesLoaded)
	assert.Equal(t, 2, runner.discovered)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/a"), mainPackage("cmd/b")}})
	require.NotNil(t, state.chooser)
	assert.Equal(t, []string{"./cmd/a", "./cmd/b"}, state.chooser.targets)
}

// The note about a vanished target belongs to the resolution that saw it go,
// not to later ones that open a chooser for an unrelated reason.
func TestVanishedTargetIsNotReportedTwice(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	state.runTarget = "./cmd/worker"

	// worker disappears and only server is left, so it runs without a chooser.
	state.invalidatePackages()
	runner.packages = []jobs.Package{mainPackage("cmd/server")}
	state.startJob(jobs.Run)
	require.Nil(t, state.chooser)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "./cmd/server", runner.started[0].Target)
	assert.Empty(t, state.runTarget)

	// Much later a second package appears and a chooser opens for that reason.
	state.invalidatePackages()
	runner.packages = []jobs.Package{mainPackage("cmd/server"), mainPackage("cmd/tool")}
	state.startJob(jobs.Run)
	require.NotNil(t, state.chooser)
	assert.Contains(t, state.message, "Several runnable packages")
	assert.NotContains(t, state.message, "is gone")
}

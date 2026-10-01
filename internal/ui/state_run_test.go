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

func openRunDefaultFromMenu(state *shellState) {
	state.runMenuAction(buildMenuSetup)
	state.runSetupAction(0)
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

func TestRunKeepsMultiMainPackageTarget(t *testing.T) {
	t.Parallel()

	listed := mainPackage(".")
	listed.GoFiles = []string{"first.go", "helper.go", "second.go"}
	state, runner := runState(t, listed)
	state.document = &project.Document{Path: filepath.Join(runRoot, "second.go")}
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "."}, runner.started[0])
	assert.Nil(t, state.chooser)

	state.document = nil
	state.runTarget = ""
	state.startJob(jobs.Run)
	assert.Nil(t, state.chooser)
	require.Len(t, runner.started, 2)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "."}, runner.started[1])
}

func TestRunKeepsNestedMultiMainPackageTarget(t *testing.T) {
	t.Parallel()

	listed := mainPackage("scripts")
	listed.GoFiles = []string{"first.go", "helper.go", "second.go"}
	state, runner := runState(t, listed)
	state.document = &project.Document{Path: filepath.Join(runRoot, "scripts", "second.go")}
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "./scripts"}, runner.started[0])
}

func TestRunCurrentEntryIncludesHelpersAndKeepsRunPackageOriented(t *testing.T) {
	t.Parallel()

	listed := mainPackage("examples")
	listed.GoFiles = []string{"bar.go", "foo.go", "helper.go"}
	state, runner := runState(t, listed)
	runner.entryFiles = []string{filepath.Join(runRoot, "examples", "foo.go"), filepath.Join(runRoot, "examples", "helper.go")}
	state.document = &project.Document{Path: filepath.Join(runRoot, "examples", "foo.go")}
	state.runArguments = []string{"--name", "demo"}
	state.runMenuAction(buildMenuCurrent)
	require.Len(t, runner.started, 1)
	assert.Equal(t, jobs.Request{
		Kind: jobs.Run, Dir: filepath.Join(runRoot, "examples"), Files: []string{"./foo.go", "./helper.go"},
		Arguments: []string{"--name", "demo"},
	}, runner.started[0])
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyF9, 0, tcell.ModAlt)))
	require.Len(t, runner.started, 2)
	assert.Equal(t, runner.started[0], runner.started[1])

	state.startJob(jobs.Run)
	require.Len(t, runner.started, 3)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "./examples", Arguments: []string{"--name", "demo"}}, runner.started[2])
}

func TestRunCurrentEntryRequiresMainFile(t *testing.T) {
	t.Parallel()

	state, runner := runState(t)
	state.requestCurrentEntry()
	assert.Empty(t, runner.started)
	assert.Contains(t, state.message, "Open")

	listed := mainPackage("examples")
	listed.GoFiles = []string{"foo.go", "helper.go"}
	state, runner = runState(t, listed)
	state.document = &project.Document{Path: filepath.Join(runRoot, "examples", "helper.go")}
	state.requestCurrentEntry()
	assert.Empty(t, runner.started)
	assert.Contains(t, state.message, "main()")
}

func TestRunCurrentEntryWaitsForPackageListing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: filepath.Join(runRoot, "examples", "foo.go")}}
	state.requestCurrentEntry()
	assert.Equal(t, 1, runner.entryDiscovered)
	assert.Empty(t, runner.started)
	listed := mainPackage("examples")
	listed.GoFiles = []string{"foo.go", "helper.go"}
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: state.document.Path, EntryFiles: []string{filepath.Join(listed.Dir, "foo.go"), filepath.Join(listed.Dir, "helper.go")}})
	require.Len(t, runner.started, 1)
	assert.Equal(t, []string{"./foo.go", "./helper.go"}, runner.started[0].Files)
}

func TestRunCurrentEntryReportsInactiveFile(t *testing.T) {
	t.Parallel()
	state, runner := runState(t)
	state.document = &project.Document{Path: filepath.Join(runRoot, "examples", "entry.go")}
	runner.entryErr = jobs.ErrInactiveEntry
	state.requestCurrentEntry()
	assert.Empty(t, runner.started)
	assert.Contains(t, state.message, "excluded by Go build constraints")
}

func TestRunCurrentEntryCoalescesDiscovery(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	entry := filepath.Join(runRoot, "examples", "entry.go")
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: entry}}
	state.requestCurrentEntry()
	state.requestCurrentEntry()
	assert.Equal(t, 1, runner.entryDiscovered)
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: entry, EntryFiles: []string{entry}})
	assert.False(t, state.entryDiscovering)
	require.Len(t, runner.started, 1)
	assert.Equal(t, []string{"./entry.go"}, runner.started[0].Files)
}

func TestRunCurrentEntryRefreshesChangedFile(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	first := filepath.Join(runRoot, "examples", "first.go")
	second := filepath.Join(runRoot, "examples", "second.go")
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: first}}
	state.requestCurrentEntry()
	state.document = &project.Document{Path: second}
	state.requestCurrentEntry()
	assert.Equal(t, 1, runner.entryDiscovered)
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: first, EntryFiles: []string{first}})
	assert.Equal(t, 2, runner.entryDiscovered)
	assert.Empty(t, runner.started)
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 2, EntryPath: second, EntryFiles: []string{second}})
	require.Len(t, runner.started, 1)
	assert.Equal(t, []string{"./second.go"}, runner.started[0].Files)
}

func TestRunCurrentEntryUsesResultAfterReturningToOriginalFile(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	first := filepath.Join(runRoot, "examples", "first.go")
	second := filepath.Join(runRoot, "examples", "second.go")
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: first}}
	state.requestCurrentEntry()
	state.document = &project.Document{Path: second}
	state.requestCurrentEntry()
	state.document = &project.Document{Path: first}
	state.requestCurrentEntry()
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: first, EntryFiles: []string{first}})
	assert.Equal(t, 1, runner.entryDiscovered)
	require.Len(t, runner.started, 1)
	assert.Equal(t, []string{"./first.go"}, runner.started[0].Files)
	assert.Empty(t, state.entryPendingPath)
}

func TestRunCurrentEntryClearsStaleStatus(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	first := filepath.Join(runRoot, "examples", "first.go")
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: first}}
	state.requestCurrentEntry()
	state.document = &project.Document{Path: filepath.Join(runRoot, "examples", "second.go")}
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: first, EntryFiles: []string{first}})
	assert.Empty(t, runner.started)
	assert.Empty(t, state.message)
	assert.False(t, state.entryDiscovering)
}

func TestRunCurrentEntryCanUsePackageTarget(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	entry := filepath.Join(runRoot, "examples", "entry.go")
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: entry}}
	state.requestCurrentEntry()
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: entry, EntryTarget: filepath.Dir(entry)})
	require.Len(t, runner.started, 1)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Dir: filepath.Dir(entry), Target: "."}, runner.started[0])
}

func TestRunCurrentEntryOutsideProjectUsesEntryDirectory(t *testing.T) {
	t.Parallel()
	for _, packageRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "files", true: "package"}[packageRun], func(t *testing.T) {
			t.Parallel()
			runner := &fakeRunner{}
			entry := "/tmp/external/main.go"
			state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: entry}}
			state.requestCurrentEntry()
			event := jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: entry, EntryFiles: []string{entry}}
			if packageRun {
				event.EntryFiles = nil
				event.EntryTarget = filepath.Dir(entry)
			}
			state.applyJobEvent(event)
			require.Len(t, runner.started, 1)
			assert.Equal(t, filepath.Dir(entry), runner.started[0].Dir)
			if packageRun {
				assert.Equal(t, ".", runner.started[0].Target)
			} else {
				assert.Equal(t, []string{"./main.go"}, runner.started[0].Files)
			}
		})
	}
}

func TestRunCurrentEntryRequiresSavedBuffer(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	entry := filepath.Join(runRoot, "main.go")
	buffer, err := editor.New("package main\n")
	require.NoError(t, err)
	require.NoError(t, buffer.MoveTo(editor.Position{Line: 1}, false))
	require.NoError(t, buffer.Insert("func main() {}\n"))
	state := &shellState{projectRoot: runRoot, jobs: runner, document: &project.Document{Path: entry}, buffer: buffer}
	state.requestCurrentEntry()
	assert.Zero(t, runner.entryDiscovered)
	assert.Contains(t, state.message, "Save")

	buffer.MarkSaved()
	state.requestCurrentEntry()
	assert.Equal(t, 1, runner.entryDiscovered)
	require.NoError(t, buffer.Insert("// unsaved\n"))
	state.applyJobEvent(jobs.Event{Type: jobs.EntryDiscovered, ID: 1, EntryPath: entry, EntryFiles: []string{entry}})
	assert.Empty(t, runner.started)
	assert.Contains(t, state.message, "Save")
}

func TestTerminalRunUsesResolvedPackageAndArguments(t *testing.T) {
	t.Parallel()
	state, runner := runState(t, mainPackage("cmd/rapidgo"))
	state.runArguments = []string{"tui", "file with spaces"}
	state.requestTerminalRun()
	require.NotNil(t, state.terminalRun)
	assert.Equal(t, jobs.Request{Kind: jobs.Run, Target: "./cmd/rapidgo", Arguments: []string{"tui", "file with spaces"}}, *state.terminalRun)
	assert.Empty(t, runner.started, "the terminal run waits for terminal handoff")
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

func TestRunWithoutAnyMainPackage(t *testing.T) {
	t.Parallel()

	unavailable := &shellState{}
	unavailable.startJob(jobs.Run)
	assert.Equal(t, "Go commands are not available in this session", unavailable.message)

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

func TestRenderPackageRunChooserTitles(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _ := runState(t, mainPackage("cmd/one"), mainPackage("cmd/two"))

	checkTitle := func(want string) {
		t.Helper()
		render(screen, *state)
		var dialog strings.Builder
		for y := range 24 {
			dialog.WriteString(rowText(screen, y, 0, 80))
		}
		assert.Contains(t, dialog.String(), want)
	}
	state.startJob(jobs.Run)
	checkTitle("Run which package?")
	state.chooseRunTarget()
	checkTitle("Set default run package")
	state.requestTerminalRun()
	checkTitle("Run in terminal: which package?")
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

func TestRunTargetMenuWithOneOrNoPackages(t *testing.T) {
	t.Parallel()

	single, runner := runState(t, mainPackage("cmd/rapidgo"))
	openRunDefaultFromMenu(single)
	assert.Nil(t, single.chooser, "one package needs no dialog")
	assert.Equal(t, "./cmd/rapidgo", single.runTarget)
	assert.Equal(t, "Default run package: ./cmd/rapidgo (the only runnable package)", single.message)
	assert.Empty(t, runner.started)

	none, _ := runState(t)
	openRunDefaultFromMenu(none)
	assert.Equal(t, "No runnable Go package found", none.message)
}

// Choosing a target before the listing arrives must not start a run.
func TestRunTargetMenuWaitsForTheListing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	openRunDefaultFromMenu(state)
	assert.Equal(t, "Finding runnable packages...", state.message)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/server"), mainPackage("cmd/worker")}})
	require.NotNil(t, state.chooser)
	assert.False(t, state.chooser.run)
	assert.Empty(t, runner.started)
}

// The open file wins over a menu target, which is only the fallback.
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

// Build -> Run Options -> Default Package... records a target without running.
func TestRenderRunChooserNamesWhatEnterDoes(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _ := runState(t, mainPackage("cmd/server"), mainPackage("cmd/worker"))
	openRunDefaultFromMenu(state)
	require.NotNil(t, state.chooser)
	render(screen, *state)

	var dialog strings.Builder
	for y := range 24 {
		dialog.WriteString(rowText(screen, y, 0, 80))
		dialog.WriteString("\n")
	}
	assert.Contains(t, dialog.String(), "Set default run package")
	assert.NotContains(t, dialog.String(), "Run which package?")
	assert.Contains(t, dialog.String(), "Up/Down Move  Enter Select  Esc Cancel")
	assert.NotContains(t, dialog.String(), "Enter Run")
	assert.Contains(t, state.message, "Up/Down and Enter to set the default run package")
}

// A save can change which packages are runnable, so the cache must not outlive it.
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

// Choosing a target rescans, so a newly added package shows up.
func TestRunTargetMenuRescans(t *testing.T) {
	t.Parallel()

	state, runner := runState(t, mainPackage("cmd/server"))
	state.startJob(jobs.Run)
	require.Equal(t, 1, runner.discovered)

	runner.packages = []jobs.Package{mainPackage("cmd/server"), mainPackage("cmd/worker")}
	openRunDefaultFromMenu(state)
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

// A listing the project changed underneath must not become the cache.
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

// With nothing waiting, a stale listing is dropped without asking again.
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

// Default Package... promises a rescan, even mid-listing.
func TestRunTargetDuringDiscoveryRescans(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	state := &shellState{projectRoot: runRoot, jobs: runner}
	state.startJob(jobs.Run)
	openRunDefaultFromMenu(state)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/old")}})
	assert.False(t, state.packagesLoaded)
	assert.Equal(t, 2, runner.discovered)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{mainPackage("cmd/a"), mainPackage("cmd/b")}})
	require.NotNil(t, state.chooser)
	assert.Equal(t, []string{"./cmd/a", "./cmd/b"}, state.chooser.targets)
}

// The vanished-target note belongs only to the resolution that saw it go.
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

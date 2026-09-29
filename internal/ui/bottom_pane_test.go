package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/gopls"
	"github.com/hangxie/rapidgo/internal/jobs"
)

func TestBottomPaneShowsOutputThenErrorsOnBuildFailure(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newJobState(&fakeRunner{})
	state.focus = focusEditor
	state.startJob(jobs.Build)
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Equal(t, focusEditor, state.focus)
	view := state.activeView()
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "./main.go:3:2: undefined: x", Stream: jobs.Stderr})
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed})
	assert.Equal(t, bottomErrors, state.bottomMode)
	assert.Equal(t, focusEditor, state.focus, "a failed build should not move keyboard focus")
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "ERRORS")
	assert.Contains(t, paneText(screen, 0, 23), "undefined: x")
	state.startJob(jobs.Test)
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestFailedJobWithoutLocatedErrorKeepsOutput(t *testing.T) {
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Test)
	view := state.activeView()
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Test, Type: jobs.Output, Line: "plain test failure"})
	view.lines = append(view.lines, outputLine{problem: &diagnostic.Diagnostic{Path: "main_test.go", Line: 3, Severity: diagnostic.Info, Source: diagnostic.SourceTest, Message: "t.Log"}})
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Test, Type: jobs.Finished, State: jobs.Failed})
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Empty(t, state.errorItems())
}

func TestOlderJobFailureDoesNotReplaceCurrentOutput(t *testing.T) {
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	build := state.activeView()
	state.applyJobEvent(jobs.Event{ID: build.id, Kind: jobs.Build, Type: jobs.Output, Line: "./main.go:3:2: undefined: x", Stream: jobs.Stderr})
	state.startJob(jobs.Test)
	state.applyJobEvent(jobs.Event{ID: build.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed})
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Equal(t, jobs.Test, state.visibleJob)
}

func TestGoplsUpdatesDoNotSwitchBottomPane(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root, bottomMode: bottomOutput, focus: focusEditor}
	setTestDocument(t, state, path, "package main\n")
	state.languageVersion = 1
	state.languageSyncedText = state.buffer.Text()
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 1, Items: []gopls.Diagnostic{{Message: "bad", Severity: 1}}})
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Equal(t, focusEditor, state.focus)
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'x', 0)))
	assert.Contains(t, state.buffer.Text(), "xpackage main")
	assert.Equal(t, bottomOutput, state.bottomMode)
	state.openProblems(screen)
	assert.Equal(t, bottomErrors, state.bottomMode)
	assert.Equal(t, focusOutput, state.focus)
}

func TestOpeningErrorsOnVeryShortTerminalKeepsEditorFocus(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(20, 5)
	state := &shellState{focus: focusEditor, mainFocus: focusEditor}
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt)))
	assert.Equal(t, focusEditor, state.focus)
	assert.Contains(t, state.message, "terminal")
}

func TestAltETogglesErrorsAndLeavesF8Available(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{focus: focusEditor, mainFocus: focusEditor}
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt)))
	assert.Equal(t, bottomErrors, state.bottomMode)
	assert.Equal(t, focusOutput, state.focus)
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyF8, 0, 0)))
	assert.Equal(t, bottomErrors, state.bottomMode)
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt)))
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestBottomErrorsJumpAndSwitchBackToOutput(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\na𐐀界b\n"), 0o600))
	state := &shellState{projectRoot: root, focus: focusEditor}
	setTestDocument(t, state, path, "package main\na𐐀界b\n")
	state.problems = []languageProblem{{Diagnostic: diagnostic.Diagnostic{Path: path, Line: 2, Severity: diagnostic.Error, Source: "gopls", Message: "bad"}, utf16Column: 4}}
	state.openProblems(screen)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, focusEditor, state.focus)
	assert.Equal(t, 3, state.buffer.Cursor().Column)
	state.toggleBottomMode(screen)
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestBottomPaneKeepsRawOutputWhileErrorsAreVisible(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Build)
	view := state.activeView()
	for _, line := range []string{"# example.com/m", "./main.go:3:2: undefined: x", "unparsed compiler detail"} {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: line, Stream: jobs.Stderr})
	}
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed})
	state.problems = []languageProblem{{Diagnostic: diagnostic.Diagnostic{Path: filepath.Join(state.projectRoot, "other.go"), Line: 5, Severity: diagnostic.Warning, Source: "gopls", Message: "unused"}}}
	assert.Len(t, state.errorItems(), 2)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "unused")
	state.toggleBottomMode(screen)
	render(screen, *state)
	output := paneText(screen, 0, 23)
	assert.Contains(t, output, "unparsed compiler detail")
	assert.Contains(t, output, "undefined: x")
}

func TestBottomErrorsOpenCompilerLocationInAnotherFile(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	first := filepath.Join(root, "first.go")
	second := filepath.Join(root, "second.go")
	require.NoError(t, os.WriteFile(second, []byte("package main\nvar x = missing\n"), 0o600))
	var queued workRequest
	state := &shellState{projectRoot: root, jobs: &fakeRunner{}, enqueue: func(request workRequest) bool { queued = request; return true }}
	setTestDocument(t, state, first, "package main\n")
	state.startJob(jobs.Build)
	view := state.activeView()
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "./second.go:2:9: undefined: missing", Stream: jobs.Stderr})
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed})
	state.openProblems(screen)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, second, queued.path)
	require.NotNil(t, state.pendingPosition)
	assert.Equal(t, 9, state.pendingPosition.byteColumn)
}

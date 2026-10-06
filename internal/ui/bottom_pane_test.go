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

func TestErrorItemsMergesProblemsBuildAndGoplsBothReport(t *testing.T) {
	const text = "package main\nvar s = \"界\"; var _ = g.Helo\n"
	// The compiler counts bytes and gopls counts UTF-16 units: column 24 is 21 units in.
	const message = "g.Helo undefined (type greet.Greeter has no field or method Helo)"
	tests := []struct {
		name    string
		build   string
		file    string
		gopls   gopls.Diagnostic
		sources [][]string
	}{
		{
			// In `var _ = "界"; var _ = x+x`, the second x is 23 UTF-16 units in and the first x is at byte column 24.
			name:    "file not open with a multibyte line",
			build:   "./other.go:2:24: " + message,
			file:    "other.go",
			gopls:   gopls.Diagnostic{Message: message, Severity: 1, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 23}}},
			sources: [][]string{{"compile"}, {"gopls"}},
		},
		{
			name:    "same problem",
			build:   "./main.go:2:24: " + message,
			gopls:   gopls.Diagnostic{Message: message, Severity: 1, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 21}}},
			sources: [][]string{{"compile", "gopls"}},
		},
		{
			name:    "different message",
			build:   "./main.go:2:24: " + message,
			gopls:   gopls.Diagnostic{Message: "other", Severity: 1, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 21}}},
			sources: [][]string{{"compile"}, {"gopls"}},
		},
		{
			name:    "different column",
			build:   "./main.go:2:24: " + message,
			gopls:   gopls.Diagnostic{Message: message, Severity: 1, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 23}}},
			sources: [][]string{{"compile"}, {"gopls"}},
		},
		{
			name:    "different severity",
			build:   "./main.go:2:24: " + message,
			gopls:   gopls.Diagnostic{Message: message, Severity: 2, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 21}}},
			sources: [][]string{{"compile"}, {"gopls"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.go")
			state := newJobState(&fakeRunner{})
			state.projectRoot = root
			setTestDocument(t, state, path, text)
			state.languageVersion = 1
			state.languageSyncedText = state.buffer.Text()
			state.startJob(jobs.Build)
			view := state.activeView()
			state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: test.build, Stream: jobs.Stderr})
			reported := path
			if test.file != "" {
				reported = filepath.Join(root, test.file)
			}
			state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: reported, Version: 1, Items: []gopls.Diagnostic{test.gopls}})
			items := state.errorItems()
			sources := make([][]string, 0, len(items))
			for _, item := range items {
				sources = append(sources, item.sources)
			}
			assert.Equal(t, test.sources, sources)
		})
	}
}

func TestErrorsViewShowsMergedSources(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := newJobState(&fakeRunner{})
	state.projectRoot = root
	setTestDocument(t, state, path, "package main\nvar _ = g.Helo\n")
	state.languageVersion = 1
	state.languageSyncedText = state.buffer.Text()
	state.startJob(jobs.Build)
	view := state.activeView()
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: "./main.go:2:9: g.Helo undefined", Stream: jobs.Stderr})
	state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Finished, State: jobs.Failed})
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 1, Items: []gopls.Diagnostic{{Message: "g.Helo undefined", Severity: 1, Range: gopls.Range{Start: gopls.Position{Line: 1, Character: 8}}}}})
	render(screen, *state)
	text := paneText(screen, 0, 23)
	assert.Contains(t, text, "ERRORS  1 diagnostic(s)")
	assert.Contains(t, text, "main.go:2:9 [error] compile, gopls: g.Helo undefined")
	assert.Len(t, view.lines, 1, "job output keeps its own line")
}

func TestErrorItemsResolvesTestPathsInTheirPackage(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "greet")
	path := filepath.Join(directory, "greet_test.go")
	state := &shellState{projectRoot: root, jobStarted: true, visibleJob: jobs.Test, packages: []jobs.Package{{Dir: directory, ImportPath: "example/greet"}}}
	state.views = map[jobs.Kind]*jobView{jobs.Test: {lines: []outputLine{
		{problem: &diagnostic.Diagnostic{Path: "greet_test.go", Line: 4, Column: 2, Severity: diagnostic.Error, Source: diagnostic.SourceTest, Package: "example/greet", Message: "bad"}},
	}}}
	state.problems = []languageProblem{{Diagnostic: diagnostic.Diagnostic{Path: path, Line: 4, Column: 2, Severity: diagnostic.Error, Source: "gopls", Message: "bad"}, utf16Column: 1}}
	items := state.errorItems()
	require.Len(t, items, 1)
	assert.Equal(t, []string{"test", "gopls"}, items[0].sources)
}

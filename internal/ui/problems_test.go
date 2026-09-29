package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestProblemsAcrossProjectFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.go")
	second := filepath.Join(root, "b.go")
	require.NoError(t, os.WriteFile(second, []byte("package p\nvar s = \"界𐐀x\"\n"), 0o600))
	state := &shellState{projectRoot: root}
	setTestDocument(t, state, first, "package p\n")
	state.languageVersion = 1
	state.languageSyncedText = state.buffer.Text()
	problem := gopls.Diagnostic{Message: "second file", Severity: 1}
	problem.Range.Start = gopls.Position{Line: 1, Character: 12}
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: second, Items: []gopls.Diagnostic{problem}})
	assert.Empty(t, state.languageDiagnostics)
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state.openProblems(screen)
	require.Len(t, state.problems, 1)
	assert.Equal(t, second, state.problems[0].Path)
	assert.Equal(t, bottomErrors, state.bottomMode)
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: second})
	assert.Empty(t, state.problems)
}

func TestProblemsNavigationAndStaleCurrentFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root}
	setTestDocument(t, state, path, "package p\n")
	state.languageVersion = 2
	state.languageSyncedText = state.buffer.Text()
	problem := gopls.Diagnostic{Message: "old", Severity: 1}
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 1, Items: []gopls.Diagnostic{problem}})
	assert.Empty(t, state.problems)
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 2, Items: []gopls.Diagnostic{problem}})
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(20, 12)
	state.openProblems(screen)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 11), "ERRORS")
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)))
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestProblemJumpIntoOtherFileKeepsUTF16Column(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.go")
	second := filepath.Join(root, "b.go")
	state := &shellState{projectRoot: root, enqueue: func(workRequest) bool { return true }}
	setTestDocument(t, state, first, "package p\n")
	problem := languageProblem{Diagnostic: diagnostic.Diagnostic{Path: second, Line: 2, Message: "bad"}, utf16Column: 4}
	require.NoError(t, os.WriteFile(second, []byte("package p\na𐐀界b\n"), 0o600))
	state.jumpToLanguage(problem)
	require.NotNil(t, state.pendingPosition)
	assert.True(t, state.pendingPosition.fromUTF16)
	setTestDocument(t, state, second, "package p\na𐐀界b\n")
	state.applyPendingPosition()
	assert.Equal(t, editor.Position{Line: 1, Column: 3}, state.buffer.Cursor())
}

func TestProblemJumpInOpenFileUsesGraphemeColumn(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := "package main\na𐐀界b\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	state := &shellState{projectRoot: root}
	setTestDocument(t, state, path, content)
	state.jumpToLanguage(languageProblem{Diagnostic: diagnostic.Diagnostic{Path: path, Line: 2}, utf16Column: 4})
	assert.Equal(t, editor.Position{Line: 1, Column: 3}, state.buffer.Cursor())
	assert.Nil(t, state.pendingPosition)
}

func TestProblemListFiltersAndSorts(t *testing.T) {
	root := t.TempDir()
	state := &shellState{projectRoot: root}
	item := gopls.Diagnostic{Message: "bad", Severity: 2}
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: filepath.Join(root, "z.go"), Items: []gopls.Diagnostic{item}})
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: filepath.Join(root, "a.go"), Items: []gopls.Diagnostic{item}})
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: filepath.Join(root, "..", "outside.go"), Items: []gopls.Diagnostic{item}})
	require.Len(t, state.problems, 2)
	assert.Equal(t, filepath.Join(root, "a.go"), state.problems[0].Path)
	assert.Equal(t, diagnostic.Warning, state.problems[0].Severity)
}

func TestProblemListKeyboardOnShortTerminal(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(24, 12)
	state := &shellState{projectRoot: t.TempDir()}
	for index := range 8 {
		state.problems = append(state.problems, languageProblem{Diagnostic: diagnostic.Diagnostic{Path: filepath.Join(state.projectRoot, "missing.go"), Line: index + 1, Message: "missing"}})
	}
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt)))
	assert.Equal(t, bottomErrors, state.bottomMode)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone))
	assert.Equal(t, state.outputRows(screen), state.errorSelected)
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone))
	assert.Equal(t, 7, state.errorSelected)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 11), "ERRORS")
	assert.Contains(t, paneText(screen, 0, 11), "missing.go:8")
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	assert.Equal(t, bottomErrors, state.bottomMode)
	assert.Contains(t, state.message, "Cannot locate")
}

func TestProblemListNavigationAndEmptyState(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(30, 14)
	state := &shellState{projectRoot: t.TempDir(), bottomMode: bottomErrors, focus: focusOutput}
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 13), "No diagnostics")
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, "No error selected", state.message)
	for index := range 5 {
		state.problems = append(state.problems, languageProblem{Diagnostic: diagnostic.Diagnostic{Path: filepath.Join(state.projectRoot, "a.go"), Line: index + 1}})
	}
	for _, test := range []struct {
		key  tcell.Key
		want int
	}{
		{tcell.KeyDown, 1},
		{tcell.KeyDown, 2},
		{tcell.KeyUp, 1},
		{tcell.KeyEnd, 4},
		{tcell.KeyPgUp, 1},
		{tcell.KeyHome, 0},
		{tcell.KeyPgDn, 3},
	} {
		state.handleOutputKey(screen, tcell.NewEventKey(test.key, 0, 0))
		assert.Equal(t, test.want, state.errorSelected)
	}
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyRight, 0, 0))
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestProblemReportsRejectInvalidPositionsAndNonGoPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root}
	setTestDocument(t, state, path, "package main\n")
	state.languageVersion = 1
	state.languageSyncedText = state.buffer.Text()
	invalid := gopls.Diagnostic{Message: "bad position"}
	invalid.Range.Start = gopls.Position{Line: -1, Character: 0}
	outside := gopls.Diagnostic{Message: "outside buffer"}
	outside.Range.Start = gopls.Position{Line: 99, Character: 0}
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 1, Items: []gopls.Diagnostic{invalid, outside}})
	assert.Empty(t, state.problems)
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: filepath.Join(root, "README.md"), Items: []gopls.Diagnostic{outside}})
	assert.Empty(t, state.problems)
	assert.False(t, projectFile("", path))
	assert.False(t, projectFile(root, "relative.go"))
}

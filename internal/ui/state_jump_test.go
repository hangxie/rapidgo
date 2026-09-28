package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// jumpProject returns a shell over a module whose listing is already loaded.
func jumpProject(t *testing.T) (*shellState, *fakeRunner, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.26\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))
	sub := filepath.Join(root, "internal", "sub")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "sub_test.go"), []byte("package sub\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n"), 0o600))

	runner := &fakeRunner{packages: []jobs.Package{
		{ImportPath: "example.com/m", Dir: root, Name: "main"},
		{ImportPath: "example.com/m/internal/sub", Dir: sub, Name: "sub"},
	}}
	state := &shellState{projectRoot: root, jobs: runner, enqueue: func(workRequest) bool { return true }}
	runner.state = state
	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: runner.packages})
	return state, runner, root
}

func TestResolveCompilerPath(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	for _, reported := range []string{"main.go", "./main.go"} {
		path, ok := state.resolvePath(diagnostic.Diagnostic{Path: reported})
		assert.True(t, ok, reported)
		assert.Equal(t, filepath.Join(root, "main.go"), path)
	}

	path, ok := state.resolvePath(diagnostic.Diagnostic{Path: "internal/sub/sub_test.go"})
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(root, "internal", "sub", "sub_test.go"), path)
}

// A test failure resolves through the package its verdict line supplied.
func TestResolveTestPathThroughItsPackage(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	problem := diagnostic.Diagnostic{Path: "sub_test.go", Line: 5, Source: diagnostic.SourceTest, Package: "example.com/m/internal/sub"}
	path, ok := state.resolvePath(problem)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "internal", "sub", "sub_test.go"), path)

	// Without the package there is nothing to resolve it against.
	_, ok = state.resolvePath(diagnostic.Diagnostic{Path: "sub_test.go", Source: diagnostic.SourceTest})
	assert.False(t, ok)
}

func TestResolveRejectsWhatIsNotThere(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	for _, reported := range []string{"", "missing.go", "internal/sub/gone.go"} {
		_, ok := state.resolvePath(diagnostic.Diagnostic{Path: reported})
		assert.False(t, ok, reported)
	}
	// A directory is not a file to open.
	_, ok := state.resolvePath(diagnostic.Diagnostic{Path: "internal/sub"})
	assert.False(t, ok)

	// An absolute path is used as reported.
	path, ok := state.resolvePath(diagnostic.Diagnostic{Path: filepath.Join(root, "main.go")})
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(root, "main.go"), path)
	_, ok = state.resolvePath(diagnostic.Diagnostic{Path: filepath.Join(root, "nope.go")})
	assert.False(t, ok)
}

func TestJumpMovesTheCaretInAnOpenFile(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state, _, root := jumpProject(t)
	path := filepath.Join(root, "main.go")
	setTestDocument(t, state, path, "package main\n\nfunc main() {}\n")

	state.jumpTo(diagnostic.Diagnostic{Path: "main.go", Line: 3, Column: 6})
	assert.Equal(t, 2, state.buffer.Cursor().Line)
	assert.Equal(t, 5, state.buffer.Cursor().Column)
	assert.Equal(t, focusEditor, state.focus)
	assert.Contains(t, state.message, "main.go:3:6")
}

// A position the file does not have is clamped, not rejected.
func TestJumpClampsAPositionPastTheEnd(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	path := filepath.Join(root, "main.go")
	setTestDocument(t, state, path, "package main\n")

	state.jumpTo(diagnostic.Diagnostic{Path: "main.go", Line: 400, Column: 20})
	assert.Equal(t, 1, state.buffer.Cursor().Line, "clamped to the last line")
	assert.Contains(t, state.message, "past the end of the file")

	state.jumpTo(diagnostic.Diagnostic{Path: "main.go", Line: 1, Column: 900})
	assert.Equal(t, 12, state.buffer.Cursor().Column, "clamped to the end of the line")
	assert.Contains(t, state.message, "past the end of the file")

	// A diagnostic with no column lands at the start of its line.
	state.jumpTo(diagnostic.Diagnostic{Path: "main.go", Line: 1})
	assert.Equal(t, 0, state.buffer.Cursor().Column)
	assert.NotContains(t, state.message, "past the end")
}

func TestJumpReportsAFileItCannotFind(t *testing.T) {
	t.Parallel()

	state, _, _ := jumpProject(t)
	state.jumpTo(diagnostic.Diagnostic{Path: "vanished.go", Line: 2})
	assert.Equal(t, "Cannot locate vanished.go", state.message)
}

// The listing is needed to resolve a test path, so a jump waits for it.
func TestJumpWaitsForThePackageListing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "internal", "sub")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "sub_test.go"), []byte("package sub\n"), 0o600))
	runner := &fakeRunner{}
	state := &shellState{projectRoot: root, jobs: runner, enqueue: func(workRequest) bool { return true }}

	state.jumpTo(diagnostic.Diagnostic{Path: "sub_test.go", Line: 1, Source: diagnostic.SourceTest, Package: "example.com/m/internal/sub"})
	assert.Equal(t, 1, runner.discovered, "the jump asked for the listing")
	assert.Equal(t, "Finding runnable packages...", state.message)
	require.NotNil(t, state.pendingJump)

	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: []jobs.Package{
		{ImportPath: "example.com/m/internal/sub", Dir: sub, Name: "sub"},
	}})
	assert.Nil(t, state.pendingJump)
	assert.Contains(t, state.message, "Opening")
}

func TestJumpFromTheSelectedOutputLine(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _, root := jumpProject(t)
	setTestDocument(t, state, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	state.startJob(jobs.Build)
	view := state.activeView()
	for _, line := range []string{"# example.com/m", "./main.go:3:6: undefined: x"} {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Build, Type: jobs.Output, Line: line, Stream: jobs.Stderr})
	}
	state.setFocus(focusOutput)

	// The header carries no problem.
	view.selectLine(0, state.outputRows(screen))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Equal(t, "No problem on this line", state.message)

	view.selectLine(1, state.outputRows(screen))
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.Equal(t, 2, state.buffer.Cursor().Line)
	assert.Equal(t, focusEditor, state.focus)
}

// The view fills in the package once the verdict line names it.
func TestVerdictNamesThePackageOfTestOutputAboveIt(t *testing.T) {
	t.Parallel()

	state := newJobState(&fakeRunner{})
	state.startJob(jobs.Test)
	view := state.activeView()
	for _, line := range []string{
		"--- FAIL: TestX (0.00s)",
		"    sub_test.go:8: boom",
		"FAIL",
		"FAIL\texample.com/m/internal/sub\t0.177s",
		"--- FAIL: TestY (0.00s)",
		"    other_test.go:3: bang",
		"FAIL\texample.com/m/internal/other\t0.1s",
	} {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Test, Type: jobs.Output, Line: line})
	}

	found := view.diagnostics()
	require.Len(t, found, 2)
	assert.Equal(t, "example.com/m/internal/sub", found[0].Package)
	assert.Equal(t, "example.com/m/internal/other", found[1].Package, "each verdict names only its own output")
}

// Testify locations can jump through both package-relative and absolute paths.
func TestJumpFromTestifyFailureLocations(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state, _, root := jumpProject(t)
	state.startJob(jobs.Test)
	view := state.activeView()
	path := filepath.Join(root, "internal", "sub", "sub_test.go")
	for _, line := range []string{
		"--- FAIL: TestX (0.00s)",
		"    sub_test.go:4:",
		"        Error Trace:" + path + ":5",
		"                    " + path + ":4",
		"        Error: Not equal:",
		"FAIL\texample.com/m/internal/sub\t0.01s",
	} {
		state.applyJobEvent(jobs.Event{ID: view.id, Kind: jobs.Test, Type: jobs.Output, Line: line})
	}
	state.setFocus(focusOutput)
	for _, test := range []struct{ index, line int }{{1, 4}, {2, 5}, {3, 4}} {
		view.selectLine(test.index, state.outputRows(screen))
		assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
		require.NotNil(t, state.pendingPosition)
		assert.Equal(t, test.line-1, state.pendingPosition.line)
		assert.Contains(t, state.message, path)
		state.setFocus(focusOutput)
	}
}

// Opening another file for a jump respects unsaved changes.
func TestJumpAsksBeforeDiscardingEdits(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	setTestDocument(t, state, filepath.Join(root, "main.go"), "package main\n")
	require.NoError(t, state.buffer.Insert("x"))
	require.True(t, state.buffer.Dirty())

	state.jumpTo(diagnostic.Diagnostic{Path: "internal/sub/sub_test.go", Line: 1})
	assert.Equal(t, confirmOpen, state.confirm)
	assert.Contains(t, state.message, "Unsaved changes")
	assert.Equal(t, filepath.Join(root, "internal", "sub", "sub_test.go"), state.pendingPath)
}

// A save in flight owns the buffer, so a jump must not race it.
func TestJumpWaitsForASaveToFinish(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	setTestDocument(t, state, filepath.Join(root, "main.go"), "package main\n")
	state.saving = true

	state.jumpTo(diagnostic.Diagnostic{Path: "internal/sub/sub_test.go", Line: 1})
	assert.Contains(t, state.message, "Save in progress")
	assert.Nil(t, state.pendingPosition)
}

func TestJumpWithNoSelectionDoesNothing(t *testing.T) {
	t.Parallel()

	state := newJobState(&fakeRunner{})
	state.jumpToSelected()
	assert.Empty(t, state.message)

	state.startJob(jobs.Build)
	state.jumpToSelected()
	assert.Equal(t, "Starting go build ./...", state.message, "an empty view has no line to act on")
}

// A same-named file in the project root must not win over the package's.
func TestTestPathPrefersItsOwnPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "internal", "sub")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "foo_test.go"), []byte("package m\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "foo_test.go"), []byte("package sub\n"), 0o600))

	runner := &fakeRunner{packages: []jobs.Package{
		{ImportPath: "example.com/m", Dir: root, Name: "main"},
		{ImportPath: "example.com/m/internal/sub", Dir: sub, Name: "sub"},
	}}
	state := &shellState{projectRoot: root, jobs: runner}
	runner.state = state
	state.applyJobEvent(jobs.Event{Type: jobs.Discovered, Packages: runner.packages})

	path, ok := state.resolvePath(diagnostic.Diagnostic{
		Path: "foo_test.go", Line: 20, Source: diagnostic.SourceTest,
		Package: "example.com/m/internal/sub",
	})
	require.True(t, ok)
	assert.Equal(t, filepath.Join(sub, "foo_test.go"), path, "the package directory wins for test output")

	// A compiler path is relative to the project root, so that still wins.
	path, ok = state.resolvePath(diagnostic.Diagnostic{
		Path: "foo_test.go", Line: 1, Source: diagnostic.SourceCompile,
		Package: "example.com/m/internal/sub",
	})
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "foo_test.go"), path)
}

// Go reports byte column 23 below, where the cluster column is 17.
func TestJumpConvertsAByteColumnToAGraphemeColumn(t *testing.T) {
	t.Parallel()

	state, _, root := jumpProject(t)
	source := "package main\n\nfunc main() {\n\tprintln(\"é界🎉\", undefinedName)\n}\n"
	setTestDocument(t, state, filepath.Join(root, "main.go"), source)

	state.jumpTo(diagnostic.Diagnostic{Path: "main.go", Line: 4, Column: 23})
	assert.Equal(t, 3, state.buffer.Cursor().Line)
	assert.Equal(t, 16, state.buffer.Cursor().Column, "zero-based cluster column for undefinedName")
	assert.Contains(t, state.message, "main.go:4:17")
	assert.NotContains(t, state.message, "past the end")
}

func TestGraphemeColumn(t *testing.T) {
	t.Parallel()

	const line = "\tprintln(\"é界🎉\", x)"
	clusters := uniseg.GraphemeClusterCount(line)
	require.Equal(t, 18, clusters, "the line is 18 clusters wide and more bytes than that")
	for _, test := range []struct {
		name       string
		byteColumn int
		want       int
		outside    bool
	}{
		{name: "no column reported", byteColumn: 0},
		{name: "first column", byteColumn: 1},
		{name: "before the multibyte run", byteColumn: 10, want: 9},
		{name: "inside a cluster rounds down", byteColumn: 12, want: 10},
		{name: "after the multibyte run", byteColumn: 20, want: 13},
		{name: "end of the line", byteColumn: len(line) + 1, want: clusters},
		{name: "past the end", byteColumn: len(line) + 40, want: clusters, outside: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			column, outside := graphemeColumn(line, test.byteColumn, false)
			assert.Equal(t, test.want, column)
			assert.Equal(t, test.outside, outside)
		})
	}

	// An empty line has only its start, and a clamped line stays clamped.
	column, outside := graphemeColumn("", 5, false)
	assert.Zero(t, column)
	assert.True(t, outside)
	_, outside = graphemeColumn("abc", 1, true)
	assert.True(t, outside, "a clamped line is still outside the file")
}

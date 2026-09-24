package ui

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/project"
)

func TestBrowseAndOpenFile(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	require.Len(t, requests, 1)
	assert.Equal(t, listDirectory, requests[0].kind)
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{
		{Name: "main.go"}, {Name: "go.mod", Regular: true}, {Name: "nested", IsDir: true},
	}})
	assert.True(t, state.tree.Root.Module)
	assert.Equal(t, "nested", state.tree.Visible()[1].Node.Name)

	state.moveSelection(screen, 1)
	state.openSelected(screen)
	require.Len(t, requests, 2)
	assert.Equal(t, listDirectory, requests[1].kind)
	state.applyResult(workResult{request: requests[1], entries: []project.Entry{{Name: "child.go"}}})
	assert.Equal(t, "child.go", state.tree.Visible()[2].Node.Name)
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	require.Len(t, requests, 3)
	assert.Equal(t, openFile, requests[2].kind)
	state.applyResult(workResult{request: requests[2], document: project.Document{Path: requests[2].path, Lines: []string{"package nested", "// 界"}}})
	require.NotNil(t, state.document)
	assert.Equal(t, []string{"package nested", "// 界"}, state.document.Lines)
	render(screen, state)
	value, _, _ := screen.Get(27, 2)
	assert.Equal(t, "p", value, "opened UTF-8 file is visible in preview")
}

func TestDirectoryLoadPreservesSelectedNode(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{
		{Name: "dirA", IsDir: true}, {Name: "fileB.go"},
	}})
	state.moveSelection(screen, 1)
	state.expandSelected(screen)
	require.Len(t, requests, 2)
	state.moveSelection(screen, 1)
	selected := state.selectedNode()
	require.NotNil(t, selected)
	assert.Equal(t, "fileB.go", selected.Name)
	assert.Equal(t, 2, state.selected)

	state.applyResult(workResult{request: requests[1], entries: []project.Entry{{Name: "childA.go"}}})
	assert.Same(t, selected, state.selectedNode())
	assert.Equal(t, 3, state.selected)
	state.openSelected(screen)
	require.Len(t, requests, 3)
	assert.Equal(t, openFile, requests[2].kind)
	assert.Equal(t, selected.Path, requests[2].path)
}

func TestOpenResultCannotReplaceNewerFile(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "a.go"}, {Name: "b.go"}}})
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	state.applyResult(workResult{request: requests[2], document: project.Document{Path: "b.go", Lines: []string{"b"}}})
	state.applyResult(workResult{request: requests[1], document: project.Document{Path: "a.go", Lines: []string{"a"}}})
	assert.Equal(t, "b.go", state.document.Path)
}

func TestOpenCompletionDoesNotOverrideNewerFocusChoice(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(35, 12)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "new.go"}}})
	state.document = &project.Document{Path: "old.go", Lines: []string{"old"}}
	state.focus = focusPreview
	key := func(code tcell.Key) {
		assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(code, 0, 0)))
	}
	key(tcell.KeyF3)
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	require.Len(t, requests, 2)
	key(tcell.KeyF6)
	assert.Equal(t, focusPreview, state.focus)
	key(tcell.KeyF3)
	assert.Equal(t, focusTree, state.focus)

	state.applyResult(workResult{request: requests[1], document: project.Document{Path: "new.go", Lines: []string{"new"}}})
	require.NotNil(t, state.document)
	assert.Equal(t, "new.go", state.document.Path)
	assert.Equal(t, focusTree, state.focus)
	assert.True(t, state.treeAccessible(screen), "the tree remains visible on a narrow terminal")
}

func TestOpenCompletionRespectsF3WhileTreeAlreadyFocused(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(35, 12)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "main.go"}}})
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	require.Len(t, requests, 2)
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF3, 0, 0)))
	state.applyResult(workResult{request: requests[1], document: project.Document{Path: "main.go"}})
	assert.Equal(t, "main.go", state.document.Path)
	assert.Equal(t, focusTree, state.focus)
}

func TestDirectoryLoadErrorHintDoesNotAssumeCurrentSelection(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{
		{Name: "dirA", IsDir: true}, {Name: "fileB.go"},
	}})
	state.moveSelection(screen, 1)
	state.expandSelected(screen)
	require.Len(t, requests, 2)
	state.moveSelection(screen, 1)
	state.applyResult(workResult{request: requests[1], err: os.ErrPermission})
	assert.Contains(t, state.message, "Failed to load /tmp/work/dirA")
	assert.Contains(t, state.message, "select directory and press Enter to retry")
	assert.Equal(t, "fileB.go", state.selectedNode().Name)
	state.openSelected(screen)
	require.Len(t, requests, 3)
	assert.Equal(t, filepath.Join("/tmp/work", "fileB.go"), requests[2].path)
}

func TestDirectoryErrorAndNarrowTree(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(35, 12)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	permissionError := os.ErrPermission
	state.applyResult(workResult{request: requests[0], err: permissionError})
	assert.ErrorIs(t, state.tree.Root.Error, permissionError)
	assert.Contains(t, state.message, "retry")
	assert.True(t, state.treeAccessible(screen), "narrow terminals start in the tree")
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF3, 0, 0)))
	assert.True(t, state.treeAccessible(screen))
	state.openSelected(screen)
	require.Len(t, requests, 2)
	state.applyResult(workResult{request: requests[1], entries: []project.Entry{{Name: "main.go"}}})
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	state.applyResult(workResult{request: requests[2], document: project.Document{Path: "/tmp/work/main.go", Lines: []string{"package main"}}})
	assert.Equal(t, focusPreview, state.focus, "opening a file focuses preview on narrow screens")
	assert.False(t, state.treeAccessible(screen))
	assert.Contains(t, state.message, "read-only preview")
}

func TestFileOpenFailureKeepsCurrentPreview(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "main.go"}}})
	state.moveSelection(screen, 1)
	state.document = &project.Document{Path: "old.go", Lines: []string{"old"}}
	state.openSelected(screen)
	state.applyResult(workResult{request: requests[1], err: os.ErrNotExist})
	assert.Equal(t, "old.go", state.document.Path)
	assert.Contains(t, state.message, "file does not exist")
}

func TestTreeKeyNavigationAndPreviewScroll(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 12)
	var requests []workRequest
	state := newShellState("/tmp/work", func(request workRequest) bool {
		requests = append(requests, request)
		return true
	})
	state.applyResult(workResult{request: requests[0], entries: []project.Entry{{Name: "nested", IsDir: true}, {Name: "main.go"}}})
	key := func(code tcell.Key) {
		assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(code, 0, 0)))
		state.keepSelectionVisible(screen)
	}
	key(tcell.KeyEnd)
	assert.Equal(t, "main.go", state.selectedNode().Name)
	key(tcell.KeyLeft)
	assert.Equal(t, "work", state.selectedNode().Name)
	key(tcell.KeyDown)
	assert.Equal(t, "nested", state.selectedNode().Name)
	key(tcell.KeyRight)
	require.Len(t, requests, 2)
	state.applyResult(workResult{request: requests[1], entries: []project.Entry{{Name: "child.go"}}})
	key(tcell.KeyDown)
	assert.Equal(t, "child.go", state.selectedNode().Name)
	key(tcell.KeyLeft)
	assert.Equal(t, "nested", state.selectedNode().Name)
	key(tcell.KeyLeft)
	assert.False(t, state.selectedNode().Expanded)
	key(tcell.KeyHome)
	assert.Equal(t, "work", state.selectedNode().Name)

	lines := make([]string, 30)
	for index := range lines {
		lines[index] = "line"
	}
	state.document = &project.Document{Path: "/tmp/work/main.go", Lines: lines}
	state.focus = focusPreview
	key(tcell.KeyDown)
	assert.Equal(t, 1, state.fileScroll)
	key(tcell.KeyUp)
	assert.Zero(t, state.fileScroll)
	key(tcell.KeyPgDn)
	assert.Greater(t, state.fileScroll, 0)
	key(tcell.KeyPgUp)
	assert.Zero(t, state.fileScroll)
	key(tcell.KeyTab)
	assert.Equal(t, focusPreview, state.focus, "Tab is reserved for editing")
	key(tcell.KeyF6)
	assert.Equal(t, focusTree, state.focus)
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModCtrl)))
	assert.Equal(t, focusPreview, state.focus)
	assert.False(t, handleEvent(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModShift)))
	assert.Equal(t, focusPreview, state.focus, "other modified F6 keys must not switch panes")
	key(tcell.KeyF1)
	key(tcell.KeyF6)
	assert.Equal(t, focusPreview, state.focus, "help must consume pane switching")
	key(tcell.KeyPgDn)
	assert.Zero(t, state.fileScroll, "help must consume preview navigation")
}

func TestQueueFullAndSymlinkAreReported(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newShellState("/tmp/work", func(workRequest) bool { return false })
	assert.ErrorIs(t, state.tree.Root.Error, errWorkQueueFull)
	assert.False(t, state.tree.Root.Loading)
	state.tree.Expand(state.tree.Root)
	state.tree.Apply(state.tree.Root, []project.Entry{{Name: "link.go", Symlink: true}}, nil)
	state.moveSelection(screen, 1)
	state.openSelected(screen)
	assert.Contains(t, state.message, "Symlinks")
}

type blockingSource struct {
	started  chan struct{}
	finished chan struct{}
}

func (source blockingSource) List(ctx context.Context, _ string) ([]project.Entry, error) {
	close(source.started)
	<-ctx.Done()
	close(source.finished)
	return nil, ctx.Err()
}

func (blockingSource) Open(context.Context, string) (project.Document, error) {
	return project.Document{}, nil
}

func TestSlowDirectoryReadDoesNotBlockQuit(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	source := blockingSource{started: make(chan struct{}), finished: make(chan struct{})}
	finished := make(chan error, 1)
	root := filepath.Join(t.TempDir(), "work")
	go func() { finished <- runLoopWithSource(screen, root, nil, source) }()
	select {
	case <-source.started:
	case <-time.After(2 * time.Second):
		t.Fatal("directory read did not start")
	}
	require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyCtrlQ, 0, 0)))
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("UI loop blocked on directory read")
	}
	select {
	case <-source.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("directory worker did not observe cancellation")
	}
}

func TestQueuedWorkIsCancelledBySignal(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	interrupts := make(chan os.Signal, 1)
	interrupts <- syscall.SIGTERM
	finished := make(chan error, 1)
	go func() { finished <- runLoopWithSource(screen, t.TempDir(), interrupts, project.DiskSource{}) }()
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("signal did not stop the UI loop")
	}
}

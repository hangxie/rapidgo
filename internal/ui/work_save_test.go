package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/project"
)

type fakeSaver func(context.Context, project.SaveRequest) (project.SaveResult, error)

func (f fakeSaver) Save(ctx context.Context, request project.SaveRequest) (project.SaveResult, error) {
	return f(ctx, request)
}

func TestRunLoopFormatsAndSavesFromF2(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o600))
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	finished := make(chan error, 1)
	interrupts := make(chan os.Signal, 1)
	t.Cleanup(func() { interrupts <- os.Interrupt })
	go func() {
		finished <- runLoopWithServices(screen, root, interrupts, project.DiskSource{}, project.DiskSaver{})
	}()
	post := func(key tcell.Key, value rune) {
		require.NoError(t, screen.PostEvent(tcell.NewEventKey(key, value, 0)))
	}
	shown := func(x, y int, want string) bool {
		value, _, _ := screen.Get(x, y)
		return value == want
	}
	require.Eventually(t, func() bool { return shown(7, 3, "m") }, 3*time.Second, 10*time.Millisecond)
	post(tcell.KeyEnd, 0)
	post(tcell.KeyEnter, 0)
	require.Eventually(t, func() bool { return shown(27, 2, "p") }, 3*time.Second, 10*time.Millisecond)
	post(tcell.KeyRune, ' ')
	post(tcell.KeyF2, 0)
	require.Eventually(t, func() bool {
		prefix := ""
		for x := 2; x < 7; x++ {
			value, _, _ := screen.Get(x, 19)
			prefix += value
		}
		return prefix == "Saved"
	}, 3*time.Second, 10*time.Millisecond)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data), "gofmt should remove the added leading space")
	post(tcell.KeyCtrlQ, 0)
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("UI did not quit after a successful save")
	}
}

func TestProjectWorkerRunsSaveOffUIState(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan workRequest, 1)
	results := make(chan workResult, 1)
	want := project.SaveRequest{Path: "/tmp/main.go", Original: "old", Content: "new"}
	saver := fakeSaver(func(_ context.Context, got project.SaveRequest) (project.SaveResult, error) {
		assert.Equal(t, want, got)
		return project.SaveResult{Content: "formatted"}, nil
	})
	done := make(chan struct{})
	go func() {
		projectWorker(ctx, project.DiskSource{}, saver, requests, results)
		close(done)
	}()
	requests <- workRequest{kind: saveFile, save: want}
	result := <-results
	require.NoError(t, result.err)
	assert.Equal(t, "formatted", result.saved.Content)
	cancel()
	<-done
}

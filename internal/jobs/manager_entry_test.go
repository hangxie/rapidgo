package jobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRunPassesLeadingGoArgument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/args\ngo 1.23\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "entry.go"), []byte("package main\nimport (\"fmt\"; \"os\")\nfunc main() { fmt.Printf(\"%q\\n\", os.Args[1:]) }\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	var goCommands atomic.Int32
	var binaries []string
	manager.command = func(ctx context.Context, dir, name string, args []string) *exec.Cmd {
		goCommands.Add(1)
		if len(args) >= 3 && args[0] == "build" && args[1] == "-o" {
			binaries = append(binaries, args[2])
		}
		return defaultCommand(ctx, dir, name, args)
	}
	request := Request{Kind: Run, Files: []string{"./entry.go"}, Arguments: []string{"payload.go", "two words"}}
	events := collect(t, manager, manager.Start(request))
	assert.Equal(t, Succeeded, events[len(events)-1].State, "%v", events[len(events)-1].Err)
	assert.Contains(t, outputLines(events, Stdout), `["payload.go" "two words"]`)
	assert.EqualValues(t, 1, goCommands.Load())
	var stdout, stderr strings.Builder
	require.NoError(t, manager.RunAttached(t.Context(), request, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Contains(t, stdout.String(), `["payload.go" "two words"]`)
	assert.EqualValues(t, 2, goCommands.Load())
	require.Len(t, binaries, 2)
	for _, binary := range binaries {
		assert.Equal(t, root, filepath.Dir(filepath.Dir(binary)))
		assert.NoDirExists(t, filepath.Dir(binary))
	}
}

func TestCurrentEntryPreservesMalformedBuildTagErrorWithActiveSibling(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/malformed\ngo 1.23\n"), 0o600))
	entry := filepath.Join(root, "entry.go")
	require.NoError(t, os.WriteFile(entry, []byte("//go:build &&\n\npackage main\nfunc main() {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sibling.go"), []byte("package main\nfunc main() {}\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	_, err := manager.entryPlan(t.Context(), entry)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInactiveEntry)
	assert.ErrorContains(t, err, "parsing //go:build line")
}

func TestFileRunBuildCancels(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, t.TempDir(), "sleep")
	id := manager.Start(Request{Kind: Run, Files: []string{"./entry.go"}, Arguments: []string{"input.go"}})
	for {
		select {
		case event := <-manager.Events():
			if event.ID != id {
				continue
			}
			if event.Type == Output && event.Line == "started" {
				assert.True(t, manager.Cancel(Run))
				final := collect(t, manager, id)
				assert.Equal(t, Cancelled, final[len(final)-1].State)
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("file-list build did not start")
		}
	}
}

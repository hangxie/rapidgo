package jobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandaloneDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	child := filepath.Join(root, "script")
	require.NoError(t, os.Mkdir(child, 0o700))
	assert.True(t, standaloneDirectory(child))
	manager := NewManager(child)
	t.Cleanup(manager.Close)
	assert.Contains(t, manager.goCommand(t.Context(), "go", []string{"version"}).Environ(), "GO111MODULE=off")
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/work\n"), 0o600))
	assert.False(t, standaloneDirectory(child))
	assert.Nil(t, manager.goCommand(t.Context(), "go", []string{"version"}).Env)
}

func TestStandaloneCommandCacheTracksNestedModules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	command := func() *exec.Cmd { return manager.goCommand(t.Context(), "go", []string{"version"}) }
	assert.Contains(t, command().Environ(), "GO111MODULE=off")
	assert.Contains(t, command().Environ(), "GO111MODULE=off")
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))
	module := filepath.Join(nested, "go.mod")
	require.NoError(t, os.WriteFile(module, []byte("module example.com/nested\ngo 1.23\n"), 0o600))
	assert.Nil(t, command().Env)
	require.NoError(t, os.Remove(module))
	assert.Contains(t, command().Environ(), "GO111MODULE=off")
}

func TestStandaloneCacheRefreshesAfterTimestampWindow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	assert.True(t, manager.standaloneDirectory(root))
	info, err := os.Stat(nested)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600))
	require.NoError(t, os.Chtimes(nested, info.ModTime(), info.ModTime()))
	manager.modeCache[root].scan.scannedAt = time.Now().Add(-3 * time.Second)
	assert.False(t, manager.standaloneDirectory(root))
}

func TestModuleScansDoNotHoldManagerLock(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	first := filepath.Join(base, "first")
	second := filepath.Join(base, "second")
	require.NoError(t, os.Mkdir(first, 0o700))
	require.NoError(t, os.Mkdir(second, 0o700))
	manager := NewManager(base)
	t.Cleanup(manager.Close)
	started := make(chan struct{})
	release := make(chan struct{})
	manager.scanTree = func(dir string) moduleScan {
		if dir == first {
			close(started)
			<-release
		}
		return scanModules(dir)
	}
	firstDone := make(chan bool, 1)
	go func() { firstDone <- manager.standaloneDirectory(first) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first scan did not start")
	}
	secondDone := make(chan bool, 1)
	go func() { secondDone <- manager.standaloneDirectory(second) }()
	select {
	case standalone := <-secondDone:
		assert.True(t, standalone)
	case <-time.After(5 * time.Second):
		t.Error("independent scan waited for manager lock")
	}
	close(release)
	select {
	case standalone := <-firstDone:
		assert.True(t, standalone)
	case <-time.After(5 * time.Second):
		t.Fatal("first scan did not finish")
	}
}

func TestExplicitGoModeKeepsCommandEnvironment(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, setting := range []string{"GOWORK=/tmp/external.go.work", "GO111MODULE=on"} {
		t.Run(setting, func(t *testing.T) {
			t.Parallel()
			manager := NewManager(root)
			t.Cleanup(manager.Close)
			manager.command = func(ctx context.Context, dir, name string, args []string) *exec.Cmd {
				command := defaultCommand(ctx, dir, name, args)
				command.Env = []string{setting}
				return command
			}
			command := manager.goEntryCommand(t.Context(), root, "go", []string{"list", "."})
			assert.NotContains(t, command.Environ(), "GO111MODULE=off")
		})
	}
}

func TestRootContainingNestedModuleStaysInModuleMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "examples", "tool")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/tool\ngo 1.23\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	assert.False(t, standaloneDirectory(root))
	assert.Nil(t, manager.goCommand(t.Context(), "go", []string{"version"}).Env)
	events := collect(t, manager, manager.Start(Request{Kind: Build}))
	assert.Equal(t, Failed, events[len(events)-1].State)
	output := strings.Join(outputLines(events, Stderr), "\n")
	assert.NotContains(t, output, "cannot find package")
	assert.Contains(t, output, "directory prefix")
	_, err := manager.packages(t.Context())
	assert.ErrorContains(t, err, "directory prefix")
}

func TestWorkspaceDirectoryKeepsModuleMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workspaceChild := filepath.Join(root, "unmoduled")
	require.NoError(t, os.Mkdir(workspaceChild, 0o700))
	app := filepath.Join(root, "app")
	lib := filepath.Join(root, "lib")
	require.NoError(t, os.Mkdir(app, 0o700))
	require.NoError(t, os.Mkdir(lib, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.23\nuse (\n ./app\n ./lib\n)\n"), 0o600))
	assert.False(t, standaloneDirectory(workspaceChild))
	workspaceManager := NewManager(workspaceChild)
	t.Cleanup(workspaceManager.Close)
	assert.Nil(t, workspaceManager.goCommand(t.Context(), "go", []string{"version"}).Env)
	require.NoError(t, os.WriteFile(filepath.Join(app, "go.mod"), []byte("module example.com/app\ngo 1.23\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "go.mod"), []byte("module example.com/lib\ngo 1.23\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "lib.go"), []byte("package lib\nfunc Message() string { return \"workspace ready\" }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(app, "main.go"), []byte("package main\nimport (\"fmt\"; \"example.com/lib\")\nfunc main() { fmt.Println(lib.Message()) }\n"), 0o600))
	manager := NewManager(app)
	t.Cleanup(manager.Close)
	assert.False(t, standaloneDirectory(app))
	assert.Nil(t, manager.goCommand(t.Context(), "go", []string{"version"}).Env)
	packages, err := manager.packages(t.Context())
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, "main", packages[0].Name)
	events := collect(t, manager, manager.Start(Request{Kind: Run}))
	assert.Equal(t, Succeeded, events[len(events)-1].State, "%v", events[len(events)-1].Err)
	assert.Contains(t, outputLines(events, Stdout), "workspace ready")
}

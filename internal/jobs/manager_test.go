package jobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helperEnv = "RAPIDGO_JOB_HELPER"

// TestJobHelperProcess is not a test: it stands in for the job process.
func TestJobHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	arguments := os.Args
	for index, argument := range arguments {
		if argument == "--" {
			arguments = arguments[index+1:]
			break
		}
	}
	require.NotEmpty(t, arguments, "helper process needs a behavior")
	switch arguments[0] {
	case "echo":
		_, _ = fmt.Fprintln(os.Stdout, "args: "+strings.Join(arguments[1:], " "))
		_, _ = fmt.Fprintln(os.Stderr, "diagnostic line")
		_, _ = fmt.Fprint(os.Stdout, "trailing line without newline")
	case "list":
		_, _ = fmt.Fprintln(os.Stdout, `{"Dir":"/p/cmd/tool","ImportPath":"example.com/m/cmd/tool","Name":"main"}`)
		_, _ = fmt.Fprintln(os.Stdout, `{"Dir":"/p","ImportPath":"example.com/m","Name":"m"}`)
	case "listfail":
		_, _ = fmt.Fprintln(os.Stderr, "go.mod file not found in current directory")
		os.Exit(1)
	case "cwd":
		directory, err := os.Getwd()
		require.NoError(t, err)
		_, _ = fmt.Fprintln(os.Stdout, directory)
	case "fail":
		_, _ = fmt.Fprintln(os.Stderr, "build failed")
		os.Exit(2)
	case "sleep":
		_, _ = fmt.Fprintln(os.Stdout, "started")
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func helperCommand(behavior string) CommandFunc {
	return func(ctx context.Context, dir, _ string, args []string) *exec.Cmd {
		helper := append([]string{"-test.run=TestJobHelperProcess", "--", behavior}, args...)
		command := exec.CommandContext(ctx, os.Args[0], helper...)
		command.Dir = dir
		command.Env = append(os.Environ(), helperEnv+"=1")
		return command
	}
}

func newTestManager(t *testing.T, root, behavior string) *Manager {
	t.Helper()
	manager := NewManager(root)
	manager.detected.Do(func() { manager.tool = Toolchain{Path: "go", Version: "go1.26.0"} })
	manager.command = helperCommand(behavior)
	t.Cleanup(manager.Close)
	return manager
}

// collect drains events until the job finishes or the test times out.
func collect(t *testing.T, manager *Manager, id uint64) []Event {
	t.Helper()
	deadline := time.After(30 * time.Second)
	var events []Event
	for {
		select {
		case event, ok := <-manager.Events():
			if !ok {
				t.Fatalf("event channel closed before job %d finished", id)
			}
			if event.ID != id {
				continue
			}
			events = append(events, event)
			if event.Type == Finished {
				return events
			}
		case <-deadline:
			t.Fatalf("timed out waiting for job %d; saw %v", id, events)
		}
	}
}

func outputLines(events []Event, stream Stream) []string {
	var lines []string
	for _, event := range events {
		if event.Type == Output && event.Stream == stream {
			lines = append(lines, event.Line)
		}
	}
	return lines
}

func TestManagerStreamsOutput(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "echo")
	id := manager.Start(Request{Kind: Build})
	require.NotZero(t, id)
	events := collect(t, manager, id)

	require.GreaterOrEqual(t, len(events), 2)
	assert.Equal(t, Started, events[0].Type)
	assert.Equal(t, "go build ./...", events[0].Command)
	assert.Equal(t, Build, events[0].Kind)
	final := events[len(events)-1]
	assert.Equal(t, Finished, final.Type)
	assert.Equal(t, Succeeded, final.State)
	assert.NoError(t, final.Err)
	assert.Equal(t, []string{"args: build ./...", "trailing line without newline"}, outputLines(events, Stdout))
	assert.Equal(t, []string{"diagnostic line"}, outputLines(events, Stderr))
}

func TestManagerRunsInProjectRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := newTestManager(t, root, "cwd")
	events := collect(t, manager, manager.Start(Request{Kind: Test}))
	lines := outputLines(events, Stdout)
	require.Len(t, lines, 1)
	// macOS reports a symlinked temporary directory, so compare resolved paths.
	assert.Equal(t, resolve(t, root), resolve(t, lines[0]))
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(strings.TrimSpace(path))
	require.NoError(t, err)
	return resolved
}

func TestManagerReportsFailure(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "fail")
	events := collect(t, manager, manager.Start(Request{Kind: Test}))
	final := events[len(events)-1]
	assert.Equal(t, Failed, final.State)
	require.Error(t, final.Err)
	assert.Contains(t, final.Err.Error(), "exit status 2")
	assert.Equal(t, []string{"build failed"}, outputLines(events, Stderr))
}

func TestManagerCancelStopsRunningJob(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "sleep")
	id := manager.Start(Request{Kind: Run, Target: "./cmd/tool"})
	waitForOutput(t, manager, id)
	assert.True(t, manager.Cancel(Run))

	final := collect(t, manager, id)
	assert.Equal(t, Cancelled, final[len(final)-1].State)
	assert.Eventually(t, func() bool { return !manager.Running(Run) }, 10*time.Second, 10*time.Millisecond)
	assert.False(t, manager.Cancel(Run))
}

func TestManagerReplacesJobOfSameKind(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "sleep")
	first := manager.Start(Request{Kind: Build})
	waitForOutput(t, manager, first)

	manager.command = helperCommand("echo")
	second := manager.Start(Request{Kind: Build})
	require.NotEqual(t, first, second)

	// The replaced job must finish before the replacement starts.
	var order []uint64
	deadline := time.After(30 * time.Second)
	for {
		select {
		case event := <-manager.Events():
			if event.Type != Finished && event.Type != Started {
				continue
			}
			order = append(order, event.ID)
			if event.ID == second && event.Type == Finished {
				assert.Equal(t, Succeeded, event.State)
				assert.Equal(t, []uint64{first, second, second}, order)
				return
			}
			if event.ID == first && event.Type == Finished {
				assert.Equal(t, Cancelled, event.State)
			}
		case <-deadline:
			t.Fatalf("timed out; saw %v", order)
		}
	}
}

func TestManagerCloseCancelsEverything(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "sleep")
	id := manager.Start(Request{Kind: Build})
	waitForOutput(t, manager, id)
	assert.Equal(t, 1, manager.CancelAll())

	manager.Close()
	manager.Close() // Closing twice must not panic.
	drain(t, manager)
	assert.Zero(t, manager.Start(Request{Kind: Build}), "a closed manager rejects new jobs")
	manager.Warm()
	manager.Discover()
}

func TestManagerRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "echo")
	assert.Zero(t, manager.Start(Request{Kind: kindCount}))
}

func TestManagerReportsMissingToolchain(t *testing.T) {
	t.Parallel()

	manager := NewManager(t.TempDir())
	manager.detected.Do(func() { manager.toolErr = fmt.Errorf("locate go executable: not found") })
	t.Cleanup(manager.Close)

	events := collect(t, manager, manager.Start(Request{Kind: Build}))
	require.Len(t, events, 1)
	assert.Equal(t, Finished, events[0].Type)
	assert.Equal(t, Failed, events[0].State)
	assert.ErrorContains(t, events[0].Err, "locate go executable")
}

func TestManagerWarmReportsToolchain(t *testing.T) {
	t.Parallel()

	manager := NewManager(t.TempDir())
	manager.detected.Do(func() { manager.tool = Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"} })
	t.Cleanup(manager.Close)

	manager.Warm()
	select {
	case event := <-manager.Events():
		assert.Equal(t, Detected, event.Type)
		assert.Equal(t, "go1.26.0 (/usr/bin/go)", event.Tool.Describe())
		assert.NoError(t, event.Err)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the Detected event")
	}
}

// drain reads the remaining buffered events and requires the channel to close.
func drain(t *testing.T, manager *Manager) {
	t.Helper()
	for {
		select {
		case _, open := <-manager.Events():
			if !open {
				return
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Close must close the event channel")
		}
	}
}

// waitForOutput blocks until the job's first line proves it is running.
func waitForOutput(t *testing.T, manager *Manager, id uint64) {
	t.Helper()
	for {
		select {
		case event := <-manager.Events():
			if event.ID == id && event.Type == Output {
				return
			}
			if event.ID == id && event.Type == Finished {
				t.Fatalf("job %d finished before producing output: %v", id, event.State)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for job %d to start", id)
		}
	}
}

func TestManagerDiscoversMainPackages(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "list")
	manager.Discover()
	event := awaitDiscovery(t, manager)
	require.NoError(t, event.Err)
	require.Len(t, event.Packages, 2)
	assert.Equal(t, "example.com/m", event.Packages[0].ImportPath)
	assert.Equal(t, "example.com/m/cmd/tool", event.Packages[1].ImportPath)
	assert.Equal(t, "/p/cmd/tool", event.Packages[1].Dir)
}

func TestManagerReportsListFailure(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, t.TempDir(), "listfail")
	manager.Discover()
	event := awaitDiscovery(t, manager)
	assert.Empty(t, event.Packages)
	assert.ErrorContains(t, event.Err, "go list: go.mod file not found in current directory")
}

func TestDiscoverReportsMissingToolchain(t *testing.T) {
	t.Parallel()

	manager := NewManager(t.TempDir())
	manager.detected.Do(func() { manager.toolErr = fmt.Errorf("locate go executable: not found") })
	t.Cleanup(manager.Close)

	manager.Discover()
	assert.ErrorContains(t, awaitDiscovery(t, manager).Err, "locate go executable")
}

func awaitDiscovery(t *testing.T, manager *Manager) Event {
	t.Helper()
	for {
		select {
		case event := <-manager.Events():
			if event.Type == Discovered {
				return event
			}
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for the Discovered event")
		}
	}
}

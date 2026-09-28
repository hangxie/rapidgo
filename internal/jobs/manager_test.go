package jobs

import (
	"context"
	"fmt"
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

func TestManagerRunAttached(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, t.TempDir(), "echo")
	var stdout, stderr strings.Builder
	err := manager.RunAttached(t.Context(), Request{Kind: Run, Target: "./cmd/tool", Arguments: []string{"tui", "file with spaces"}}, strings.NewReader(""), &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "args: run ./cmd/tool tui file with spaces")
	assert.Contains(t, stderr.String(), "diagnostic line")
}

func TestManagerRunAttachedCancels(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, t.TempDir(), "sleep")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{}, 1)
	writer := &notifyWriter{started: started}
	finished := make(chan error, 1)
	go func() {
		finished <- manager.RunAttached(ctx, Request{Kind: Run}, strings.NewReader(""), writer, writer)
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("attached process did not start")
	}
	cancel()
	select {
	case err := <-finished:
		assert.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("attached process did not stop")
	}
}

// notifyWriter reports first output from the helper process.
type notifyWriter struct{ started chan<- struct{} }

func (writer *notifyWriter) Write(data []byte) (int, error) {
	select {
	case writer.started <- struct{}{}:
	default:
	}
	return len(data), nil
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

func TestStandaloneGoFileCommands(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	source := []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"standalone ready\") }\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), source, 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	assert.True(t, standaloneDirectory(root))

	packages, err := manager.packages(t.Context())
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, "main", packages[0].Name)
	assert.Equal(t, resolve(t, root), resolve(t, packages[0].Dir))

	for _, kind := range []Kind{Build, Test, Run} {
		events := collect(t, manager, manager.Start(Request{Kind: kind}))
		assert.Equal(t, Succeeded, events[len(events)-1].State, "%s: %v", kind, events)
		if kind == Run {
			assert.Contains(t, outputLines(events, Stdout), "standalone ready")
		}
	}

	var stdout, stderr strings.Builder
	err = manager.RunAttached(t.Context(), Request{Kind: Run}, strings.NewReader(""), &stdout, &stderr)
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stdout.String(), "standalone ready")
}

func TestStandaloneBuildFromLibraryKeepsProjectScope(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	library := filepath.Join(root, "library")
	require.NoError(t, os.Mkdir(library, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(library, "helper.go"), []byte("package library\nfunc Helper() {}\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	events := collect(t, manager, manager.Start(Request{Kind: Build}))
	require.NotEmpty(t, events)
	assert.Equal(t, "go build ./...", events[0].Command)
	assert.Equal(t, Succeeded, events[len(events)-1].State)
}

func TestStandaloneBuildDoesNotNeedPackageDiscovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := newTestManager(t, root, "echo")
	var listed atomic.Bool
	manager.command = func(ctx context.Context, dir, name string, args []string) *exec.Cmd {
		if args[0] == "list" {
			listed.Store(true)
			return helperCommand("listfail")(ctx, dir, name, args)
		}
		return helperCommand("echo")(ctx, dir, name, args)
	}
	events := collect(t, manager, manager.Start(Request{Kind: Build}))
	require.NotEmpty(t, events)
	assert.Equal(t, "go build ./...", events[0].Command)
	assert.Equal(t, Succeeded, events[len(events)-1].State)
	assert.False(t, listed.Load())
}

func TestStandaloneScriptsShareHelperWithoutOtherMain(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	for _, directory := range []string{".", "scripts"} {
		t.Run(directory, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			sourceDir := filepath.Join(root, directory)
			if directory != "." {
				require.NoError(t, os.Mkdir(sourceDir, 0o700))
			}
			for name, source := range map[string]string{
				"first.go":       "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"first\", helper()) }\n",
				"second.go":      "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"second\", helper()) }\n",
				"helper.go":      "package main\nfunc helper() string { return \"shared\" }\n",
				"helper_test.go": "package main\nimport \"testing\"\nfunc TestHelper(t *testing.T) { if helper() != \"shared\" { t.Fatal(\"wrong helper\") } }\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(sourceDir, name), []byte(source), 0o600))
			}
			manager := NewManager(root)
			t.Cleanup(manager.Close)
			packages, err := manager.packages(t.Context())
			require.NoError(t, err)
			require.Len(t, packages, 1)
			assert.Equal(t, resolve(t, sourceDir), resolve(t, packages[0].Dir))

			for _, kind := range []Kind{Build, Test} {
				events := collect(t, manager, manager.Start(Request{Kind: kind}))
				assert.Equal(t, (Request{Kind: kind}).Command(), events[0].Command)
				assert.Equal(t, Failed, events[len(events)-1].State, "%s keeps package scope", kind)
			}
			for _, entry := range []string{"first", "second"} {
				relative, err := filepath.Rel(root, sourceDir)
				require.NoError(t, err)
				plan, err := manager.entryPlan(t.Context(), filepath.Join(sourceDir, entry+".go"))
				require.NoError(t, err)
				names := plan.Files
				assert.Equal(t, []string{filepath.Join(sourceDir, entry+".go"), filepath.Join(sourceDir, "helper.go")}, names)
				files := make([]string, 0, len(names))
				for _, name := range names {
					files = append(files, "./"+filepath.ToSlash(filepath.Join(relative, filepath.Base(name))))
				}
				events := collect(t, manager, manager.Start(Request{Kind: Run, Files: files}))
				assert.Equal(t, Succeeded, events[len(events)-1].State, "%s: %v", entry, events)
				assert.Contains(t, outputLines(events, Stdout), entry+" shared")
			}
			assert.NoFileExists(t, filepath.Join(sourceDir, "first"))
			assert.NoFileExists(t, filepath.Join(sourceDir, "second"))
		})
	}
}

func TestModuleExamplesWithSeveralMainsRunAsFiles(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	examples := filepath.Join(root, "examples")
	require.NoError(t, os.Mkdir(examples, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/work\n\ngo 1.23\n"), 0o600))
	for name, source := range map[string]string{
		"first.go":  "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"first\", helper()) }\n",
		"second.go": "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"second\", helper()) }\n",
		"helper.go": "package main\nfunc helper() string { return \"shared\" }\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(examples, name), []byte(source), 0o600))
	}
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	assert.False(t, standaloneDirectory(examples))
	packages, err := manager.packages(t.Context())
	require.NoError(t, err)
	require.Len(t, packages, 1)

	for _, entry := range []string{"first", "second"} {
		plan, err := manager.entryPlan(t.Context(), filepath.Join(examples, entry+".go"))
		require.NoError(t, err)
		files := plan.Files
		require.Len(t, files, 2)
		for index, name := range files {
			files[index] = "./examples/" + filepath.Base(name)
		}
		events := collect(t, manager, manager.Start(Request{Kind: Run, Files: files}))
		assert.Equal(t, Succeeded, events[len(events)-1].State, "%s: %v", entry, events)
		assert.Contains(t, outputLines(events, Stdout), entry+" shared")
	}
	for _, kind := range []Kind{Build, Test} {
		events := collect(t, manager, manager.Start(Request{Kind: kind}))
		assert.Equal(t, Failed, events[len(events)-1].State, "%s keeps package scope inside a module", kind)
	}
}

func TestCurrentEntryFindsBuildTaggedExample(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	for _, withHelper := range []bool{false, true} {
		t.Run(fmt.Sprintf("helper=%t", withHelper), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := filepath.Join(root, "examples", "arrow_to_parquet")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/work\n\ngo 1.23\n"), 0o600))
			entry := filepath.Join(dir, "arrow_to_parquet.go")
			source := "//go:build example\n\npackage main\nimport \"fmt\"\nfunc main() { fmt.Println(\"entry\") }\n"
			require.NoError(t, os.WriteFile(entry, []byte(source), 0o600))
			if withHelper {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package main\nfunc helper() {}\n"), 0o600))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "other.go"), []byte("//go:build example\n\npackage main\nfunc main() {}\n"), 0o600))
			manager := NewManager(root)
			t.Cleanup(manager.Close)
			_, err := manager.entryPlan(t.Context(), entry)
			require.ErrorIs(t, err, ErrInactiveEntry)
			manager.command = func(ctx context.Context, dir, name string, args []string) *exec.Cmd {
				command := defaultCommand(ctx, dir, name, args)
				command.Env = append(os.Environ(), "GOFLAGS=-tags=example")
				return command
			}
			plan, err := manager.entryPlan(t.Context(), entry)
			require.NoError(t, err)
			files := plan.Files
			want := []string{entry}
			if withHelper {
				want = append(want, filepath.Join(dir, "helper.go"))
			}
			assert.Equal(t, want, files)
			relative := make([]string, len(files))
			for index, path := range files {
				relative[index] = "./" + strings.TrimPrefix(path, root+string(filepath.Separator))
			}
			events := collect(t, manager, manager.Start(Request{Kind: Run, Files: relative}))
			assert.Equal(t, Succeeded, events[len(events)-1].State)
			assert.Contains(t, outputLines(events, Stdout), "entry")
		})
	}
}

func TestCurrentEntryUsesGoSelectedPlatformHelpers(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/work\n\ngo 1.23\n"), 0o600))
	for name, source := range map[string]string{
		"entry.go":          "package main\nfunc main() { helper() }\n",
		"helper_linux.go":   "package main\nfunc helper() {}\n",
		"helper_windows.go": "package main\nfunc helper() {}\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(source), 0o600))
	}
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	for _, platform := range []string{"linux", "windows"} {
		manager.command = func(ctx context.Context, dir, name string, args []string) *exec.Cmd {
			command := defaultCommand(ctx, dir, name, args)
			command.Env = append(os.Environ(), "GOOS="+platform)
			return command
		}
		plan, err := manager.entryPlan(t.Context(), filepath.Join(root, "entry.go"))
		require.NoError(t, err)
		files := plan.Files
		assert.Equal(t, []string{filepath.Join(root, "entry.go"), filepath.Join(root, "helper_"+platform+".go")}, files)
	}
}

func TestCurrentEntryIncludesCgoSources(t *testing.T) {
	t.Parallel()
	if output, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err != nil || strings.TrimSpace(string(output)) != "1" {
		t.Skip("cgo is not enabled")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/cgo\n\ngo 1.23\n"), 0o600))
	for name, source := range map[string]string{
		"entry.go":      "package main\nimport \"fmt\"\nfunc main() { fmt.Println(helper()) }\n",
		"helper_cgo.go": "package main\n/* int value() { return 42; } */\nimport \"C\"\nfunc helper() int { return int(C.value()) }\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(source), 0o600))
	}
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	plan, err := manager.entryPlan(t.Context(), filepath.Join(root, "entry.go"))
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "entry.go"), filepath.Join(root, "helper_cgo.go")}, plan.Files)
	assert.Empty(t, plan.Target)
	events := collect(t, manager, manager.Start(Request{Kind: Run, Files: []string{"./entry.go", "./helper_cgo.go"}}))
	assert.Equal(t, Succeeded, events[len(events)-1].State)
	assert.Contains(t, outputLines(events, Stdout), "42")
}

func TestCurrentEntryAcceptsCgoMain(t *testing.T) {
	t.Parallel()
	if output, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err != nil || strings.TrimSpace(string(output)) != "1" {
		t.Skip("cgo is not enabled")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/cgoentry\n\ngo 1.23\n"), 0o600))
	entry := filepath.Join(root, "entry.go")
	source := "package main\n/* int value() { return 42; } */\nimport \"C\"\nimport \"fmt\"\nfunc main() { fmt.Println(int(C.value()), helper()) }\n"
	require.NoError(t, os.WriteFile(entry, []byte(source), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "helper.go"), []byte("package main\nfunc helper() string { return \"ready\" }\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	plan, err := manager.entryPlan(t.Context(), entry)
	require.NoError(t, err)
	assert.Equal(t, []string{entry, filepath.Join(root, "helper.go")}, plan.Files)
	events := collect(t, manager, manager.Start(Request{Kind: Run, Files: []string{"./entry.go", "./helper.go"}}))
	assert.Equal(t, Succeeded, events[len(events)-1].State)
	assert.Contains(t, outputLines(events, Stdout), "42 ready")
}

func TestCurrentEntryUsesPackageForAssembly(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/asm\n\ngo 1.23\n"), 0o600))
	for name, source := range map[string]string{
		"entry.go": "package main\nfunc helper()\nfunc main() { helper() }\n",
		"helper.s": "TEXT ·helper(SB),$0-0\nRET\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(source), 0o600))
	}
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	plan, err := manager.entryPlan(t.Context(), filepath.Join(root, "entry.go"))
	require.NoError(t, err)
	assert.Empty(t, plan.Files)
	assert.Equal(t, root, plan.Target)
	events := collect(t, manager, manager.Start(Request{Kind: Run, Target: "."}))
	assert.Equal(t, Succeeded, events[len(events)-1].State, "%v", events[len(events)-1].Err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.go"), []byte("package main\nfunc main() {}\n"), 0o600))
	_, err = manager.entryPlan(t.Context(), filepath.Join(root, "entry.go"))
	assert.ErrorIs(t, err, ErrAssemblyEntries)
}

func TestCurrentEntryPreservesNestedModuleError(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/outer\n\ngo 1.23\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/inner\n\ngo 1.23\n"), 0o600))
	entry := filepath.Join(nested, "entry.go")
	require.NoError(t, os.WriteFile(entry, []byte("package main\nfunc main() {}\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	_, err := manager.entryPlan(t.Context(), entry)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInactiveEntry)
	assert.ErrorContains(t, err, "go list:")
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

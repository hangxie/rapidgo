package debug

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/dap"
)

func startInstalledDelve(ctx context.Context, dir string) (Adapter, error) {
	session, err := dap.Start(ctx, dir)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// installedProject writes a module whose test and main package exercise breakpoints, output, and exit codes.
func installedProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv is not installed")
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod":       "module example.com/smoke\n\ngo 1.26.0\n",
		"calc/calc.go": "package calc\n\nfunc Double(x int) int {\n\treturn x * 2\n}\n",
		"calc/calc_test.go": "package calc\n\nimport (\n\t\"fmt\"\n\t\"testing\"\n)\n\n" +
			"func TestDouble(t *testing.T) {\n\tgot := Double(21)\n\tfmt.Println(\"double\", got)\n}\n\nfunc TestOther(t *testing.T) {\n\tfmt.Println(\"other\")\n}\n",
		"cmd/exit/main.go": "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\tfmt.Fprintln(os.Stderr, \"leaving\")\n\tos.Exit(3)\n}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return root
}

// awaitKind applies events until one of kind arrives, failing if the session ends first.
func awaitKind(t *testing.T, session *Session, kind EventKind, seen *[]Event) Event {
	t.Helper()
	timeout := time.After(2 * time.Minute)
	for {
		select {
		case event, ok := <-session.Events():
			require.True(t, ok, "session ended early: %v; events %+v", session.Err(), *seen)
			*seen = append(*seen, event)
			if event.Kind == kind {
				return event
			}
		case <-timeout:
			require.FailNow(t, "no event", "waiting for %d; events %+v", kind, *seen)
		}
	}
}

func skipOldDelve(t *testing.T, session *Session) {
	t.Helper()
	if err := session.Err(); err != nil && strings.Contains(err.Error(), "too old") {
		t.Skipf("installed dlv cannot debug this Go version: %v", err)
	}
}

func TestInstalledDelveDebugsATest(t *testing.T) {
	root := installedProject(t)
	dir := filepath.Join(root, "calc")
	path := filepath.Join(dir, "calc_test.go")
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	test, ok := FindTest(path, string(source), 10)
	require.True(t, ok)
	set := NewBreakpoints()
	set.Toggle(path, 10)
	plan := Plan{
		Dir:         root,
		Config:      dap.LaunchConfig{Mode: dap.ModeTest, Program: "./calc", Cwd: test.Dir, Args: test.Args()},
		Breakpoints: []Request{set.Request(path)},
	}
	session := Launch(context.Background(), startInstalledDelve, plan)
	t.Cleanup(session.Stop)
	var seen []Event
	resolved := awaitKind(t, session, Resolved, &seen)
	skipOldDelve(t, session)
	require.NoError(t, resolved.Err)
	require.Len(t, resolved.Results, 1)
	require.True(t, resolved.Results[0].Verified, resolved.Results[0].Message)

	stopped := awaitKind(t, session, Stopped, &seen)
	require.Equal(t, "breakpoint", stopped.Reason)
	session.Continue(stopped.ThreadID)
	exited := awaitKind(t, session, Exited, &seen)
	require.Equal(t, 0, exited.ExitCode)
	for range session.Events() {
	}
	require.NoError(t, session.Err())

	var output []string
	for _, event := range seen {
		if event.Kind == Output && event.Category == "stdout" {
			output = append(output, event.Text)
		}
	}
	require.Contains(t, output, "double 42")
	require.NotContains(t, output, "other", "only the test under the caret runs")
	assertNoDebugBinary(t, root)
}

// assertNoDebugBinary checks that Delve's executable never lands in the user's project.
func assertNoDebugBinary(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		require.False(t, strings.HasPrefix(entry.Name(), "__debug_bin"), "debug binary left at %s", path)
		return nil
	}))
}

func TestInstalledDelveReportsExitAndStop(t *testing.T) {
	root := installedProject(t)
	plan := Plan{Dir: root, Config: dap.LaunchConfig{Mode: dap.ModeDebug, Program: "./cmd/exit", Cwd: root}}
	session := Launch(context.Background(), startInstalledDelve, plan)
	var seen []Event
	exited := awaitKind(t, session, Exited, &seen)
	require.Equal(t, 3, exited.ExitCode)
	for range session.Events() {
	}
	require.NoError(t, session.Err())
	require.Contains(t, seen, Event{Kind: Output, Category: "stderr", Text: "leaving"})

	broken := Plan{Dir: root, Config: dap.LaunchConfig{Mode: dap.ModeDebug, Program: filepath.Join(root, "missing"), Cwd: root}}
	failed := Launch(context.Background(), startInstalledDelve, broken)
	for range failed.Events() {
	}
	require.Error(t, failed.Err(), "a build failure is reported")
	assertNoDebugBinary(t, root)
}

func TestInstalledDelveBuildsOutsideTheProject(t *testing.T) {
	root := installedProject(t)
	dir := filepath.Join(root, "calc")
	path := filepath.Join(dir, "calc_test.go")
	set := NewBreakpoints()
	set.Toggle(path, 10)
	plan := Plan{
		Dir:         root,
		Config:      dap.LaunchConfig{Mode: dap.ModeTest, Program: "./calc", Cwd: dir, Args: Test{Dir: dir, Name: "TestDouble"}.Args()},
		Breakpoints: []Request{set.Request(path)},
	}
	session := Launch(context.Background(), startInstalledDelve, plan)
	var seen []Event
	awaitKind(t, session, Stopped, &seen)
	// Delve removes its binary when it closes cleanly, so look while the program is paused.
	assertNoDebugBinary(t, root)
	session.Stop()
	for range session.Events() {
	}
}

func TestInstalledDelveDebugsAStandaloneDirectory(t *testing.T) {
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv is not installed")
	}
	t.Setenv("GO111MODULE", "")
	t.Setenv("GOFLAGS", "")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(4) }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"no\") }\n"), 0o600))
	plan := Plan{Dir: root, Config: dap.LaunchConfig{Mode: dap.ModeDebug, Program: ".", Cwd: root}}

	modules := Launch(context.Background(), startInstalledDelve, plan)
	for range modules.Events() {
	}
	require.Error(t, modules.Err(), "module mode cannot build a directory without go.mod")

	legacy := func(ctx context.Context, dir string) (Adapter, error) {
		session, err := dap.Start(ctx, dir, "GO111MODULE=off")
		if err != nil {
			return nil, err
		}
		return session, nil
	}
	session := Launch(context.Background(), legacy, plan)
	var seen []Event
	exited := awaitKind(t, session, Exited, &seen)
	require.Equal(t, 4, exited.ExitCode)
	for range session.Events() {
	}
	require.NoError(t, session.Err())

	plan.Config = dap.LaunchConfig{Mode: dap.ModeTest, Program: ".", Cwd: root, Args: Test{Name: "TestFails"}.Args()}
	session = Launch(context.Background(), legacy, plan)
	seen = nil
	exited = awaitKind(t, session, Exited, &seen)
	require.Equal(t, 1, exited.ExitCode)
	for range session.Events() {
	}
	require.NoError(t, session.Err())
}

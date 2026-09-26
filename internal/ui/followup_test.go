package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/jobs"
)

func TestInstalledGoTestOutputSeverity(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.26.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture_test.go"), []byte("package fixture\nimport \"testing\"\nfunc TestMixed(t *testing.T) {\n t.Log(\"log marker\")\n t.Error(\"error marker\")\n}\n"), 0o600))
	manager := jobs.NewManager(root)
	defer manager.Close()
	id := manager.Start(jobs.Request{Kind: jobs.Test})
	view := jobView{parser: diagnostic.New(diagnostic.Tool)}
	typed := false
	deadline := time.After(30 * time.Second)
	for {
		select {
		case event := <-manager.Events():
			if event.ID != id {
				continue
			}
			switch event.Type {
			case jobs.Output:
				view.append(event)
				typed = typed || event.TestOutputType != ""
			case jobs.TestFailed:
				view.markFailedTest(event)
			case jobs.Finished:
				assert.Equal(t, jobs.Failed, event.State)
				problems := view.diagnostics()
				require.Len(t, problems, 2)
				assert.Equal(t, "log marker", problems[0].Message)
				assert.Equal(t, "error marker", problems[1].Message)
				assert.Equal(t, diagnostic.Error, problems[1].Severity)
				if typed {
					assert.Equal(t, diagnostic.Info, problems[0].Severity)
				} else {
					assert.Equal(t, diagnostic.Error, problems[0].Severity)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for go test -json")
		}
	}
}

func TestStructuredTestSeverityAndCompilerDetails(t *testing.T) {
	t.Parallel()
	view := jobView{parser: diagnostic.New(diagnostic.Tool)}
	for _, event := range []jobs.Event{
		{Line: "    f_test.go:4: useful context", TestPackage: "example/m", StructuredTest: true},
		{Line: "    f_test.go:5: actual failure", TestPackage: "example/m", TestOutputType: "error", StructuredTest: true},
		{Line: "./f.go:9: cannot use value as function", TestPackage: "example/m", StructuredTest: true},
		{Line: "\thave func(日本語)", TestPackage: "example/m", StructuredTest: true},
		{Line: "\twant func(int)", TestPackage: "example/m", StructuredTest: true},
		{Line: "unrelated output", TestPackage: "example/m", StructuredTest: true},
		{Line: "\thave old detail", TestPackage: "example/m", StructuredTest: true},
	} {
		view.append(event)
	}
	problems := view.diagnostics()
	require.Len(t, problems, 3)
	assert.Equal(t, diagnostic.Info, problems[0].Severity)
	assert.Equal(t, diagnostic.Error, problems[1].Severity)
	assert.Equal(t, "example/m", problems[0].Package)
	assert.Equal(t, []string{"have func(日本語)", "want func(int)"}, problems[2].Details)
	assert.Len(t, view.lines, 7, "continuation and unmatched output remain visible")
}

func TestUntypedTestOutputIsGradedAtFailure(t *testing.T) {
	t.Parallel()
	view := jobView{parser: diagnostic.New(diagnostic.Tool)}
	for _, event := range []jobs.Event{
		{Line: "    old_test.go:4: log", TestPackage: "example/m", TestName: "TestOld", StructuredTest: true},
		{Line: "    new_test.go:4: log", TestPackage: "example/m", TestName: "TestNew", StructuredTest: true},
		{Line: "=== RUN TestNew", TestPackage: "example/m", TestName: "TestNew", TestOutputType: "frame", StructuredTest: true},
		{Line: "    old_test.go:5: actual failure", TestPackage: "example/m", TestName: "TestOld", StructuredTest: true},
		{Line: "    new_test.go:5: actual failure", TestPackage: "example/m", TestName: "TestNew", TestOutputType: "error", StructuredTest: true},
		{Line: "    pass_test.go:2: passing log", TestPackage: "example/m", TestName: "TestPass", StructuredTest: true},
	} {
		view.append(event)
	}
	view.markFailedTest(jobs.Event{TestPackage: "example/m", TestName: "TestOld"})
	view.markFailedTest(jobs.Event{TestPackage: "example/m", TestName: "TestNew"})
	assert.Equal(t, diagnostic.Error, view.lines[0].problem.Severity)
	assert.Equal(t, diagnostic.Info, view.lines[1].problem.Severity)
	assert.Equal(t, diagnostic.Error, view.lines[3].problem.Severity)
	assert.Equal(t, diagnostic.Error, view.lines[4].problem.Severity)
	assert.Equal(t, diagnostic.Info, view.lines[5].problem.Severity)
}

func TestRunArgumentsPrompt(t *testing.T) {
	t.Parallel()
	state, runner := runState(t, mainPackage("cmd/tool"))
	state.editRunArguments()
	for _, char := range `--name "two words"` {
		state.handleRunArgumentsKey(tcell.NewEventKey(tcell.KeyRune, char, 0))
	}
	state.handleRunArgumentsKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.False(t, state.editingRunArgs)
	assert.Equal(t, []string{"--name", "two words"}, state.runArguments)
	state.startJob(jobs.Run)
	require.Len(t, runner.started, 1)
	assert.Equal(t, []string{"--name", "two words"}, runner.started[0].Arguments)
	assert.Contains(t, state.activeView().command, "--name 'two words'")

	state.editRunArguments()
	state.handleRunArgumentsKey(tcell.NewEventKey(tcell.KeyRune, '\'', 0))
	state.handleRunArgumentsKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.True(t, state.editingRunArgs, "invalid quoting keeps the prompt open")
	state.handleRunArgumentsKey(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	assert.Equal(t, `--name "two words"`, state.runArgumentText)
	assert.Equal(t, []string{"--name", "two words"}, state.runArguments)
}

package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/jobs"
)

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

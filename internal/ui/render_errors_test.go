package ui

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/jobs"
)

func TestErrorsViewShowsLineWithoutInventingColumn(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	state := shellState{projectRoot: root, bottomMode: bottomErrors, focus: focusOutput, jobStarted: true}
	state.views = map[jobs.Kind]*jobView{jobs.Build: {
		lines: []outputLine{{problem: &diagnostic.Diagnostic{Path: filepath.Join(root, "main.go"), Line: 8, Severity: diagnostic.Error, Message: "missing symbol"}}},
	}}
	render(screen, state)
	text := paneText(screen, 0, 23)
	assert.Contains(t, text, "main.go:8 [error] missing symbol")
	assert.NotContains(t, text, "main.go:8:0")
}

package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
	"github.com/hangxie/rapidgo/internal/project"
)

func TestEnvironmentHelpShowsSessionValues(t *testing.T) {
	t.Parallel()

	state := shellState{
		projectRoot: "/work", document: &project.Document{Path: "/work/main.go"},
		toolchainErr: errors.New("go unavailable"), runTarget: "./main.go",
		runArgumentText: "--name test",
	}
	entries := environmentEntries(state, 80, 24)
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		values[entry.shortcut] = entry.action
	}
	assert.Equal(t, "/work/main.go", values["Open file"])
	assert.Equal(t, "unavailable", values["Go version"])
	assert.Equal(t, "go unavailable", values["Go detection"])
	assert.Equal(t, "./main.go", values["Run default"])
	assert.Equal(t, "--name test", values["Run arguments"])
}

func TestHelpListsWindowZoom(t *testing.T) {
	for _, entry := range helpEntries() {
		if entry.shortcut == "F5" {
			assert.Contains(t, entry.action, "Zoom")
			return
		}
	}
	t.Fatal("F5 zoom shortcut missing from help")
}

func TestRenderFramedPanesAndHelpDialog(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project"})
	assertCellColors(t, screen, 0, 1, turboLightCyan, turboBlue)  // Project frame below menu.
	assertCellColors(t, screen, 3, 1, turboWhite, turboBlue)      // Project title.
	assertCellColors(t, screen, 21, 1, turboLightCyan, turboBlue) // Editor frame.
	var editorTitle strings.Builder
	for x := 23; x < 48; x++ {
		value, _, _ := screen.Get(x, 1)
		editorTitle.WriteString(value)
	}
	assert.Contains(t, editorTitle.String(), "/tmp/project")

	render(screen, shellState{
		projectRoot: "/tmp/project", helpVisible: true,
		toolchain: jobs.Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"},
	})
	helpRow := func(y int) string {
		var row strings.Builder
		for x := 13; x < 67; x++ {
			value, _, _ := screen.Get(x, y)
			row.WriteString(value)
		}
		return row.String()
	}
	assert.Contains(t, helpRow(3), "Shortcut")
	assert.Contains(t, helpRow(3), "Action")
	assert.Contains(t, helpRow(4), "F3")
	assert.Contains(t, helpRow(5), "Alt+F3")
	assert.Contains(t, helpRow(6), "F6 / Shift+F6")
	assert.Equal(t, strings.Index(helpRow(4), "Focus tree"), strings.Index(helpRow(6), "Next / previous"))
	assert.Contains(t, helpRow(7), "Alt+0")
	assert.Contains(t, helpRow(7), "List open editor windows")
	assertCellColors(t, screen, 12, 1, turboWhite, turboLightGray) // Dialog border.
	assertCellColors(t, screen, 14, 2, turboBlack, turboLightGray) // Dialog text.
	assertCellColors(t, screen, 14, 4, turboRed, turboLightGray)   // Help shortcut.

	screen.SetSize(30, 6)
	render(screen, shellState{projectRoot: "/tmp/project", helpVisible: true})
	assertCellColors(t, screen, 24, 5, turboBlack, turboLightGray) // Shadow preserves status line.
}

func TestHelpFooterNamesEscapeAsCloseKey(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	render(screen, shellState{projectRoot: "/tmp/project", helpVisible: true, helpEnvironment: true})
	var content strings.Builder
	for y := range 24 {
		content.WriteString(rowText(screen, y, 0, 80))
	}
	assert.Contains(t, content.String(), "Esc close")
	assert.NotContains(t, content.String(), "F1/Esc close")
}

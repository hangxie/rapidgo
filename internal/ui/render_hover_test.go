package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHoverRowsWrapWideText(t *testing.T) {
	assert.Equal(t, []string{"a界", "b", "", "xyz"}, hoverRows("a界b\n\nxyz", 3))
}

func TestHoverRendersInSmallTerminal(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(16, 5)
	state := shellState{hoverVisible: true, hoverText: "Go type"}
	render(screen, state)
	assert.Contains(t, paneText(screen, 0, 4), "Go type")
}

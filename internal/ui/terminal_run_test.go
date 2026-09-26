package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScreenEventsRestartAfterTerminalHandoff(t *testing.T) {
	t.Parallel()
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	for range 2 {
		pump := startScreenEvents(screen)
		require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyF1, 0, 0)))
		_, ok := <-pump.events
		assert.True(t, ok)
		pump.stop()
		screen.Fini()
		require.NoError(t, screen.Init())
	}
}

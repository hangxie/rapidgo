package ui

import (
	"errors"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingScreen counts the screen lifecycle calls a terminal handoff makes.
type recordingScreen struct {
	tcell.SimulationScreen
	fini       int
	suspend    int
	resume     int
	suspendErr error
	resumeErr  error
}

func (screen *recordingScreen) Fini() {
	screen.fini++
	screen.SimulationScreen.Fini()
}

func (screen *recordingScreen) Suspend() error {
	screen.suspend++
	if screen.suspendErr != nil {
		return screen.suspendErr
	}
	return screen.SimulationScreen.Suspend()
}

func (screen *recordingScreen) Resume() error {
	screen.resume++
	if screen.resumeErr != nil {
		return screen.resumeErr
	}
	return screen.SimulationScreen.Resume()
}

func newRecordingScreen(t *testing.T) *recordingScreen {
	t.Helper()
	screen := &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("")}
	require.NoError(t, screen.Init())
	return screen
}

func TestSuspendedScreenKeepsFiniForQuit(t *testing.T) {
	t.Parallel()
	screen := newRecordingScreen(t)
	pump := startScreenEvents(screen)
	lent := false
	require.NoError(t, withSuspendedScreen(screen, &pump, func() { lent = true }))
	assert.True(t, lent)
	assert.Equal(t, 1, screen.suspend)
	assert.Equal(t, 1, screen.resume)
	// A Fini here would be tcell's only one, leaving the shell unrestored.
	assert.Equal(t, 0, screen.fini)
	require.NotNil(t, pump)
	pump.stop()
	screen.Fini()
	assert.Equal(t, 1, screen.fini)
}

func TestSuspendedScreenReportsFailures(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		screen  func(*recordingScreen)
		message string
		lends   bool
	}{
		"suspend fails": {
			screen:  func(screen *recordingScreen) { screen.suspendErr = errors.New("no tty") },
			message: "suspend terminal screen: no tty",
		},
		"resume fails": {
			screen:  func(screen *recordingScreen) { screen.resumeErr = errors.New("no tty") },
			message: "restore terminal screen: no tty",
			lends:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			screen := newRecordingScreen(t)
			defer screen.Fini()
			test.screen(screen)
			pump := startScreenEvents(screen)
			lent := false
			err := withSuspendedScreen(screen, &pump, func() { lent = true })
			require.EqualError(t, err, test.message)
			assert.Equal(t, test.lends, lent)
			assert.Nil(t, pump)
		})
	}
}

func TestScreenEventsRestartAfterTerminalHandoff(t *testing.T) {
	t.Parallel()
	screen := newRecordingScreen(t)
	defer screen.Fini()
	pump := startScreenEvents(screen)
	for range 2 {
		require.NoError(t, screen.PostEvent(tcell.NewEventKey(tcell.KeyF1, 0, 0)))
		_, ok := <-pump.events
		assert.True(t, ok)
		require.NoError(t, withSuspendedScreen(screen, &pump, func() {}))
	}
	pump.stop()
}

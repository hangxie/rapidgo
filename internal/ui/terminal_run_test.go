package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/jobs"
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

func TestTerminalHandoffReportsChildResult(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}
	for _, test := range []struct {
		name    string
		target  string
		message string
		failed  bool
	}{
		{"success", ".", "go run . finished", false},
		{"failure", "./missing.go", "go run ./missing.go failed", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600))
			manager := jobs.NewManager(root)
			defer manager.Close()
			screen := newRecordingScreen(t)
			defer screen.Fini()
			screen.SetSize(80, 24)
			pump := startScreenEvents(screen)
			defer func() {
				if pump != nil {
					pump.stop()
				}
			}()

			input, err := os.CreateTemp(t.TempDir(), "stdin")
			require.NoError(t, err)
			defer func() { _ = input.Close() }()
			_, err = input.WriteString("\n")
			require.NoError(t, err)
			_, err = input.Seek(0, 0)
			require.NoError(t, err)
			output, err := os.CreateTemp(t.TempDir(), "stdout")
			require.NoError(t, err)
			defer func() { _ = output.Close() }()
			oldInput, oldOutput, oldError := os.Stdin, os.Stdout, os.Stderr
			os.Stdin, os.Stdout, os.Stderr = input, output, output
			defer func() { os.Stdin, os.Stdout, os.Stderr = oldInput, oldOutput, oldError }()

			request := jobs.Request{Kind: jobs.Run, Target: test.target}
			state := &shellState{projectRoot: root, terminalRun: &request}
			require.NoError(t, handoffTerminal(t.Context(), screen, manager, state, make(chan os.Signal), &pump))
			assert.Nil(t, state.terminalRun)
			assert.Contains(t, state.message, test.message)
			assert.Equal(t, 1, screen.suspend)
			assert.Equal(t, 1, screen.resume)
			written, err := os.ReadFile(output.Name())
			require.NoError(t, err)
			assert.Contains(t, string(written), "Press Enter to return to RapidGo")
			assert.Equal(t, test.failed, strings.Contains(state.message, "failed"))
		})
	}
}

func TestTerminalHandoffWaitsForOtherJobs(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is not installed")
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"time\"\nfunc main() { time.Sleep(2 * time.Second) }\n"), 0o600))
	manager := jobs.NewManager(root)
	t.Cleanup(manager.Close)
	require.NotZero(t, manager.Start(jobs.Request{Kind: jobs.Run}))
	screen := newRecordingScreen(t)
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	pump := startScreenEvents(screen)
	t.Cleanup(pump.stop)
	request := jobs.Request{Kind: jobs.Run}
	state := &shellState{projectRoot: root, terminalRun: &request}
	require.NoError(t, handoffTerminal(t.Context(), screen, manager, state, make(chan os.Signal), &pump))
	assert.Nil(t, state.terminalRun)
	assert.Equal(t, "Stop other Go jobs before running in the terminal", state.message)
	assert.Zero(t, screen.suspend)
}

package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestNavigationRequestAndLocationList(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root, focus: focusEditor, mainFocus: focusEditor, languageStatus: "ready", enqueueLanguage: func(languageSnapshot) {}}
	setTestDocument(t, state, path, "package main\nfunc a𐐀() { a𐐀() }\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 8}, false))
	var queued languageNavigationRequest
	state.enqueueNavigation = func(request languageNavigationRequest) { queued = request }
	state.requestNavigation(navigationReferences)
	assert.Equal(t, 9, queued.position.Character)
	assert.Equal(t, navigationReferences, queued.kind)
	items := []gopls.Location{{Path: path, Position: gopls.Position{Line: 1, Character: 15}}, {Path: path, Position: gopls.Position{Line: 1, Character: 5}}}
	state.applyNavigationResult(languageNavigationResult{request: queued, locations: items})
	assert.Equal(t, bottomLocations, state.bottomMode)
	assert.Equal(t, focusOutput, state.focus)
	assert.Len(t, state.locations, 2)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "REFERENCES")
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyDown, 0, 0))
	state.handleOutputKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Equal(t, focusEditor, state.focus)
	assert.Equal(t, 5, state.buffer.Cursor().Column)
}

type blockingNavigationSession struct {
	*fakeLanguageSession
	started chan struct{}
}

func (fake *blockingNavigationSession) References(ctx context.Context, _ string, _ gopls.Position) ([]gopls.Location, error) {
	close(fake.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestNavigationWorkerStopsOnCancellation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &blockingNavigationSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8), diags: make(chan gopls.PublishedDiagnostics)}, started: make(chan struct{})}
	requests := make(chan languageSnapshot, 1)
	navigations := make(chan languageNavigationRequest, 1)
	events := make(chan languageEvent, 8)
	done := make(chan struct{})
	requests <- languageSnapshot{path: path, text: "package main\n", seq: 1}
	go func() {
		languageWorker(ctx, root, requests, nil, nil, navigations, events, func(context.Context, string) (languageSession, error) { return fake, nil })
		close(done)
	}()
	navigations <- languageNavigationRequest{snapshot: languageSnapshot{path: path, text: "package main\n", seq: 1}, kind: navigationReferences}
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("reference request did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("language worker did not stop after cancellation")
	}
}

func TestNavigationConnectionReportsSyncFailureAndDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	request := languageNavigationRequest{snapshot: languageSnapshot{path: path, text: "package main\n", seq: 1}, kind: navigationDefinition}
	for _, test := range []struct {
		name string
		fake languageSession
		want string
	}{
		{"sync failure", &failingLanguageSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8)}, fail: "open"}, "open failed"},
		{"definition", &fakeLanguageSession{log: make(chan string, 8)}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan languageEvent, 2)
			connection := languageConnection{session: test.fake, ctx: context.Background(), events: events}
			connection.navigate(request)
			event := <-events
			if event.kind == languageSynced {
				event = <-events
			}
			result := event.navigation
			if test.want == "" {
				assert.NoError(t, result.err)
			} else {
				require.ErrorContains(t, result.err, test.want)
			}
		})
	}
}

func TestLocationsKeyboardPagingAndMissingSource(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	state := &shellState{projectRoot: root, focus: focusOutput, mainFocus: focusEditor, bottomMode: bottomLocations}
	state.locations = make([]gopls.Location, 20)
	for index := range state.locations {
		state.locations[index] = gopls.Location{Path: filepath.Join(root, "missing.go"), Position: gopls.Position{Line: index}}
	}
	page := state.outputRows(screen)
	for _, test := range []struct {
		key  tcell.Key
		want int
	}{
		{tcell.KeyDown, 1},
		{tcell.KeyPgDn, 1 + page},
		{tcell.KeyEnd, 19},
		{tcell.KeyUp, 18},
		{tcell.KeyPgUp, 18 - page},
		{tcell.KeyHome, 0},
	} {
		state.handleLocationsKey(screen, tcell.NewEventKey(test.key, 0, 0))
		assert.Equal(t, test.want, state.locationSelected)
	}
	state.handleLocationsKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	assert.Contains(t, state.message, "Cannot locate")
	state.handleLocationsKey(screen, tcell.NewEventKey(tcell.KeyRight, 0, 0))
	assert.Equal(t, bottomOutput, state.bottomMode)
}

func TestNavigationRejectsUnavailableAndWrongPane(t *testing.T) {
	root := t.TempDir()
	state := &shellState{projectRoot: root, focus: focusEditor}
	state.requestNavigation(navigationDefinition)
	assert.Contains(t, state.message, "Open a Go file")
	setTestDocument(t, state, filepath.Join(root, "main.go"), "package main\n")
	state.requestNavigation(navigationDefinition)
	assert.Contains(t, state.message, "gopls is unavailable")
	state.focus = focusTree
	state.requestNavigation(navigationReferences)
	assert.Contains(t, state.message, "Focus the editor")
}

func TestNavigationDiscardsStaleResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root, focus: focusEditor, languageStatus: "ready", enqueueLanguage: func(languageSnapshot) {}}
	setTestDocument(t, state, path, "package main\n")
	var queued languageNavigationRequest
	state.enqueueNavigation = func(request languageNavigationRequest) { queued = request }
	state.requestNavigation(navigationDefinition)
	require.NoError(t, state.buffer.MoveTo(editor.Position{Column: 1}, false))
	state.applyNavigationResult(languageNavigationResult{request: queued, locations: []gopls.Location{{Path: path}}})
	assert.NotEqual(t, bottomLocations, state.bottomMode)
	assert.Contains(t, state.message, "cancelled")
}

func TestNavigationDefinitionJumpsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "main.go")
	second := filepath.Join(root, "helper.go")
	require.NoError(t, os.WriteFile(second, []byte("package main\nvar 𐐀 = 1\n"), 0o600))
	var opened workRequest
	state := &shellState{projectRoot: root, focus: focusEditor, languageStatus: "ready", enqueueLanguage: func(languageSnapshot) {}, enqueue: func(request workRequest) bool { opened = request; return true }}
	setTestDocument(t, state, first, "package main\n")
	var queued languageNavigationRequest
	state.enqueueNavigation = func(request languageNavigationRequest) { queued = request }
	state.requestNavigation(navigationDefinition)
	state.applyNavigationResult(languageNavigationResult{request: queued, locations: []gopls.Location{{Path: second, Position: gopls.Position{Line: 1, Character: 4}}}})
	assert.Equal(t, second, opened.path)
	require.NotNil(t, state.pendingPosition)
	assert.True(t, state.pendingPosition.fromUTF16)
	assert.Equal(t, 4, state.pendingPosition.utf16Column)
	assert.Equal(t, focusEditor, state.focus)
}

func TestNavigationResultStatesAndShortcuts(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	state := &shellState{projectRoot: root, focus: focusEditor, mainFocus: focusEditor, languageStatus: "ready", enqueueLanguage: func(languageSnapshot) {}}
	setTestDocument(t, state, path, "package main\n")
	var queued languageNavigationRequest
	state.enqueueNavigation = func(request languageNavigationRequest) { queued = request }
	for _, test := range []struct {
		key  *tcell.EventKey
		kind navigationKind
	}{
		{tcell.NewEventKey(tcell.KeyF12, 0, 0), navigationDefinition},
		{tcell.NewEventKey(tcell.KeyF12, 0, tcell.ModShift), navigationReferences},
		{tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModAlt), navigationDefinition},
		{tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModAlt), navigationReferences},
	} {
		handleKey(screen, state, test.key)
		assert.Equal(t, test.kind, queued.kind)
	}
	state.applyNavigationResult(languageNavigationResult{request: queued})
	assert.Contains(t, state.message, "No references")
	state.requestNavigation(navigationDefinition)
	state.applyNavigationResult(languageNavigationResult{request: queued, err: errors.New("server stopped")})
	assert.Contains(t, state.message, "server stopped")
	state.requestNavigation(navigationReferences)
	state.applyNavigationResult(languageNavigationResult{request: queued, locations: []gopls.Location{{Path: path}}})
	assert.Equal(t, bottomLocations, state.bottomMode)
	handleKey(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	assert.Equal(t, bottomOutput, state.bottomMode)
	assert.Equal(t, focusEditor, state.focus)
}

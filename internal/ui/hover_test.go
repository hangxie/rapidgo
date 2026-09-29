package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestHoverActionShowsResultAndCloses(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{projectRoot: t.TempDir(), focus: focusEditor, mainFocus: focusEditor, syntax: &syntaxCache{}, languageStatus: "ready"}
	path := filepath.Join(state.projectRoot, "main.go")
	setTestDocument(t, state, path, "package main\nfunc main() {}\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 5}, false))
	var asked languageHoverRequest
	state.enqueueLanguage = func(languageSnapshot) {}
	state.enqueueHover = func(request languageHoverRequest) { asked = request }
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyRune, 'i', tcell.ModAlt)))
	assert.Equal(t, path, asked.snapshot.path)
	assert.Equal(t, 5, asked.position.Character)
	assert.Contains(t, state.message, "Inspecting")
	state.applyLanguageEvent(languageEvent{kind: languageHovered, hover: languageHoverResult{request: asked, content: "func main()\nA useful description"}})
	assert.True(t, state.hoverVisible)
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "A useful description")
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, 0)))
	assert.False(t, state.hoverVisible)
}

func TestHoverResultDroppedAfterEdit(t *testing.T) {
	state := &shellState{languageStatus: "ready"}
	path := filepath.Join(t.TempDir(), "main.go")
	setTestDocument(t, state, path, "package main\n")
	var asked languageHoverRequest
	state.enqueueLanguage = func(languageSnapshot) {}
	state.enqueueHover = func(request languageHoverRequest) { asked = request }
	state.requestHover()
	require.NoError(t, state.buffer.Insert("// changed"))
	state.applyLanguageEvent(languageEvent{kind: languageHovered, hover: languageHoverResult{request: asked, content: "old result"}})
	assert.False(t, state.hoverVisible)
}

func TestInspectSymbolMenuAction(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{menuOpen: true, menuIndex: menuSearch, menuItem: 2, languageStatus: "ready"}
	setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "package main\n")
	state.enqueueLanguage = func(languageSnapshot) {}
	asked := false
	state.enqueueHover = func(languageHoverRequest) { asked = true }
	assert.False(t, handleEvent(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, 0)))
	assert.True(t, asked)
	assert.False(t, state.menuOpen)
}

func TestGraphemeUTF16Column(t *testing.T) {
	assert.Equal(t, 3, graphemeUTF16Column("a𐐀b", 2))
}

func TestLanguageWorkerHover(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	fake := &fakeLanguageSession{diags: make(chan gopls.PublishedDiagnostics), log: make(chan string, 8)}
	requests := make(chan languageSnapshot, 2)
	hovers := make(chan languageHoverRequest, 1)
	events := make(chan languageEvent, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		languageWorker(ctx, root, requests, hovers, events, func(context.Context, string) (languageSession, error) { return fake, nil })
		close(done)
	}()
	snapshot := languageSnapshot{path: path, text: "package main\n", seq: 2}
	requests <- snapshot
	require.Eventually(t, func() bool { return strings.Contains(fake.calls(), "open:main.go") }, time.Second, time.Millisecond)
	hovers <- languageHoverRequest{snapshot: snapshot, position: gopls.Position{Line: 0, Character: 8}, ticket: 1}
	var result languageHoverResult
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.kind == languageHovered {
				result = event.hover
				return true
			}
		default:
		}
		return false
	}, time.Second, time.Millisecond)
	assert.Equal(t, "symbol information", result.content)
	requests <- languageSnapshot{path: path, text: "stale", seq: 1}
	connection := languageConnection{session: fake, current: snapshot, ctx: ctx, events: events}
	require.NoError(t, connection.apply(languageSnapshot{path: path, text: "stale", seq: 1}))
	assert.Equal(t, snapshot, connection.current)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("language worker did not stop")
	}
}

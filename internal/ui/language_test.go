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

func TestLanguageDiagnosticsAppearInEditor(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{projectRoot: t.TempDir(), focus: focusEditor, mainFocus: focusEditor, syntax: &syntaxCache{}}
	path := filepath.Join(state.projectRoot, "main.go")
	setTestDocument(t, state, path, "package main\nfunc main() { missing() }\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1}, false))
	problem := gopls.Diagnostic{Message: "undefined: missing", Severity: 1}
	problem.Range.Start = gopls.Position{Line: 1, Character: 14}
	state.applyLanguageEvent(languageEvent{kind: languageReady})
	state.applyLanguageEvent(languageEvent{kind: languageSynced, path: path, text: state.buffer.Text(), version: 2})
	state.applyLanguageEvent(languageEvent{kind: languagePublished, published: gopls.PublishedDiagnostics{
		Path: path, Version: 2, Items: []gopls.Diagnostic{problem},
	}})
	render(screen, *state)
	assert.Contains(t, paneText(screen, 0, 23), "gopls 1 error")
	assert.Contains(t, paneText(screen, 0, 23), "undefined: missing")
	assert.Contains(t, paneText(screen, 1, 1), "gopls")

	state.applyLanguageEvent(languageEvent{kind: languagePublished, published: gopls.PublishedDiagnostics{Path: path, Version: 1}})
	assert.Len(t, state.languageDiagnostics, 1, "stale notifications must not clear current problems")
}

func TestLanguageWorkerSyncsLatestGoDocument(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	fake := &fakeLanguageSession{diags: make(chan gopls.PublishedDiagnostics), log: make(chan string, 8)}
	requests := make(chan languageSnapshot, 2)
	events := make(chan languageEvent, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		languageWorker(ctx, root, requests, nil, events, func(context.Context, string) (languageSession, error) { return fake, nil })
		close(done)
	}()
	requests <- languageSnapshot{}
	requests <- languageSnapshot{path: path, text: "package main\n"}
	require.Eventually(t, func() bool { return strings.Contains(fake.calls(), "open:main.go") }, time.Second, time.Millisecond)
	requests <- languageSnapshot{path: path, text: "package main\nfunc main() {}\n"}
	require.Eventually(t, func() bool { return strings.Contains(fake.calls(), "change:main.go") }, time.Second, time.Millisecond)
	requests <- languageSnapshot{}
	require.Eventually(t, func() bool { return strings.Contains(fake.calls(), "close:main.go") }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("language worker did not stop")
	}
}

func TestUTF16ByteColumn(t *testing.T) {
	line := "a界𐐀b"
	for _, test := range []struct{ utf16, byteColumn int }{{0, 1}, {1, 2}, {2, 5}, {3, 5}, {4, 9}, {5, 10}} {
		assert.Equal(t, test.byteColumn, utf16ByteColumn(line, test.utf16))
	}
}

func TestSyncLanguageTracksEditorChanges(t *testing.T) {
	state := &shellState{}
	var queued []languageSnapshot
	state.enqueueLanguage = func(snapshot languageSnapshot) { queued = append(queued, snapshot) }
	path := filepath.Join(t.TempDir(), "main.go")
	setTestDocument(t, state, path, "package main\n")
	state.syncLanguage()
	require.Len(t, queued, 1)
	assert.Equal(t, path, queued[0].path)
	assert.Equal(t, "starting", state.languageStatus)
	state.applyLanguageEvent(languageEvent{kind: languageReady})
	state.applyLanguageEvent(languageEvent{kind: languageSynced, path: path, text: queued[0].text, version: 1})
	assert.Equal(t, 1, state.languageVersion)
	require.NoError(t, state.buffer.Insert("// edit"))
	state.syncLanguage()
	require.Len(t, queued, 2)
	assert.Zero(t, state.languageVersion)
	state.document.Path = filepath.Join(filepath.Dir(path), "README.md")
	state.syncLanguage()
	require.Len(t, queued, 3)
	assert.Empty(t, queued[2].path)
}

type fakeLanguageSession struct {
	diags chan gopls.PublishedDiagnostics
	log   chan string
}

func (fake *fakeLanguageSession) calls() string {
	var result strings.Builder
	for {
		select {
		case call := <-fake.log:
			result.WriteString(call)
			result.WriteByte(' ')
		default:
			return result.String()
		}
	}
}

func (fake *fakeLanguageSession) record(call string) {
	fake.log <- call
}

func (fake *fakeLanguageSession) Open(path, _ string) error {
	fake.record("open:" + filepath.Base(path))
	return nil
}

func (fake *fakeLanguageSession) Change(path, _ string) error {
	fake.record("change:" + filepath.Base(path))
	return nil
}

func (fake *fakeLanguageSession) CloseDocument(path string) error {
	fake.record("close:" + filepath.Base(path))
	return nil
}

func (fake *fakeLanguageSession) Hover(context.Context, string, gopls.Position) (string, error) {
	return "symbol information", nil
}

func (fake *fakeLanguageSession) Diagnostics() <-chan gopls.PublishedDiagnostics { return fake.diags }
func (fake *fakeLanguageSession) Close() error                                   { return nil }

package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
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

func TestLanguageWorkerReportsStartupAndConnectionFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	for _, test := range []struct {
		name  string
		start func(context.Context, string) (languageSession, error)
		want  string
	}{
		{name: "startup", start: func(context.Context, string) (languageSession, error) { return nil, errors.New("missing executable") }, want: "missing executable"},
		{name: "document open", start: func(context.Context, string) (languageSession, error) {
			return &failingLanguageSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8)}, fail: "open"}, nil
		}, want: "open failed"},
		{name: "diagnostic stream", start: func(context.Context, string) (languageSession, error) {
			diagnostics := make(chan gopls.PublishedDiagnostics)
			close(diagnostics)
			return &fakeLanguageSession{diags: diagnostics, log: make(chan string, 8)}, nil
		}, want: "connection closed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			requests := make(chan languageSnapshot, 1)
			events := make(chan languageEvent, 8)
			done := make(chan struct{})
			requests <- languageSnapshot{path: path, text: "package main\n"}
			go func() { languageWorker(ctx, root, requests, nil, nil, events, test.start); close(done) }()
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("language worker did not stop")
			}
			found := false
			for len(events) > 0 {
				event := <-events
				if event.kind == languageUnavailable && strings.Contains(event.err.Error(), test.want) {
					found = true
				}
			}
			assert.True(t, found)
		})
	}
}

func TestLanguageWorkerForwardsOtherFileDiagnostics(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &fakeLanguageSession{diags: make(chan gopls.PublishedDiagnostics, 1), log: make(chan string, 8)}
	requests := make(chan languageSnapshot, 1)
	events := make(chan languageEvent, 8)
	done := make(chan struct{})
	requests <- languageSnapshot{path: filepath.Join(root, "main.go"), text: "package main\n", seq: 1}
	go func() {
		languageWorker(ctx, root, requests, nil, nil, events, func(context.Context, string) (languageSession, error) { return fake, nil })
		close(done)
	}()
	require.Eventually(t, func() bool { return strings.Contains(fake.calls(), "open:main.go") }, time.Second, time.Millisecond)
	other := filepath.Join(root, "other.go")
	fake.diags <- gopls.PublishedDiagnostics{Path: other, Version: 2}
	found := false
	require.Eventually(t, func() bool {
		for len(events) > 0 {
			event := <-events
			if event.kind == languagePublished && event.published.Path == other {
				found = true
			}
		}
		return found
	}, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("language worker did not stop")
	}
}

func TestLanguageReportsRejectStaleAndClearOnUnavailable(t *testing.T) {
	root := t.TempDir()
	state := &shellState{projectRoot: root}
	path := filepath.Join(root, "other.go")
	problem := gopls.Diagnostic{Message: "bad", Severity: 2}
	state.applyLanguageEvent(languageEvent{kind: languageReady})
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 4, Items: []gopls.Diagnostic{problem}})
	state.applyLanguageDiagnostics(gopls.PublishedDiagnostics{Path: path, Version: 3})
	require.Len(t, state.problems, 1)
	assert.Equal(t, diagnostic.Warning, state.problems[0].Severity)
	state.applyLanguageEvent(languageEvent{kind: languageUnavailable, err: errors.New("stopped")})
	assert.Empty(t, state.problems)
	assert.Equal(t, "gopls unavailable", state.languageSummary())
}

func TestLanguageSummaryForWarningsAndStartup(t *testing.T) {
	state := &shellState{}
	assert.Empty(t, state.languageSummary())
	state.applyLanguageEvent(languageEvent{kind: languageStarting})
	assert.Equal(t, "gopls starting", state.languageSummary())
	state.languageStatus = "ready"
	assert.Equal(t, "gopls ready", state.languageSummary())
	state.languageDiagnostics = []diagnostic.Diagnostic{{Severity: diagnostic.Warning}}
	assert.Equal(t, "gopls 0 error, 1 warning", state.languageSummary())
}

func TestLanguageConnectionPropagatesDocumentFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	other := filepath.Join(filepath.Dir(path), "other.go")
	for _, test := range []struct {
		name    string
		current languageSnapshot
		next    languageSnapshot
		fail    string
		want    string
	}{
		{name: "open", next: languageSnapshot{path: path, text: "new", seq: 1}, fail: "open", want: "open " + path},
		{name: "change", current: languageSnapshot{path: path, text: "old", seq: 1}, next: languageSnapshot{path: path, text: "new", seq: 2}, fail: "change", want: "sync " + path},
		{name: "close", current: languageSnapshot{path: path, text: "old", seq: 1}, next: languageSnapshot{path: other, text: "new", seq: 2}, fail: "close", want: "close " + path},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &failingLanguageSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8)}, fail: test.fail}
			connection := languageConnection{session: fake, current: test.current, version: 1, ctx: context.Background(), events: make(chan languageEvent, 1)}
			err := connection.apply(test.next)
			require.ErrorContains(t, err, test.want)
			assert.Equal(t, test.current, connection.current)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, sendLanguageEvent(ctx, make(chan languageEvent), languageEvent{kind: languageReady}))
}

func TestLanguageConnectionStopsWhenEventDeliveryIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeLanguageSession{log: make(chan string, 8)}
	connection := languageConnection{session: fake, ctx: ctx, events: make(chan languageEvent)}
	err := connection.apply(languageSnapshot{path: filepath.Join(t.TempDir(), "main.go"), text: "package main\n", seq: 1})
	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, fake.calls(), "open:main.go")
}

type failingLanguageSession struct {
	*fakeLanguageSession
	fail string
}

func (fake *failingLanguageSession) Open(string, string) error {
	if fake.fail == "open" {
		return errors.New("open failed")
	}
	return nil
}

func (fake *failingLanguageSession) Change(string, string) error {
	if fake.fail == "change" {
		return errors.New("change failed")
	}
	return nil
}

func (fake *failingLanguageSession) CloseDocument(string) error {
	if fake.fail == "close" {
		return errors.New("close failed")
	}
	return nil
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
		languageWorker(ctx, root, requests, nil, nil, events, func(context.Context, string) (languageSession, error) { return fake, nil })
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

func (fake *fakeLanguageSession) Complete(context.Context, string, gopls.Position) ([]gopls.CompletionItem, error) {
	return []gopls.CompletionItem{{Label: "println"}}, nil
}

func (fake *fakeLanguageSession) Diagnostics() <-chan gopls.PublishedDiagnostics { return fake.diags }
func (fake *fakeLanguageSession) Close() error                                   { return nil }

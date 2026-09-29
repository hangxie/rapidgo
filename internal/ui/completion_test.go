package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestCompletionResultKeepsOnlyCurrentCaret(t *testing.T) {
	state := &shellState{languageStatus: "ready", focus: focusEditor}
	path := filepath.Join(t.TempDir(), "main.go")
	setTestDocument(t, state, path, "package main\nfunc main() { pri }\n")
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 17}, false))
	var queued languageCompletionRequest
	state.enqueueLanguage = func(languageSnapshot) {}
	state.enqueueCompletion = func(request languageCompletionRequest) { queued = request }
	state.requestCompletion()
	assert.Equal(t, path, queued.snapshot.path)
	result := languageCompletionResult{request: queued, items: []gopls.CompletionItem{{Label: "println"}}}
	state.applyLanguageEvent(languageEvent{kind: languageCompleted, completion: result})
	assert.True(t, state.completionVisible)
	state.completionVisible = false
	require.NoError(t, state.buffer.MoveTo(editor.Position{Line: 1, Column: 16}, false))
	state.applyCompletionResult(result)
	assert.False(t, state.completionVisible)
}

func TestApplyCompletionItemWithImportAndUTF16Range(t *testing.T) {
	buffer, err := editor.New("package main\nfunc main() { _ = \"𐐀\"; fmt.Pr }\n")
	require.NoError(t, err)
	item := gopls.CompletionItem{Label: "Println", TextEdit: &gopls.TextEdit{
		Range:   gopls.Range{Start: gopls.Position{Line: 1, Character: 28}, End: gopls.Position{Line: 1, Character: 30}},
		NewText: "Println",
	}, AdditionalTextEdits: []gopls.TextEdit{{Range: gopls.Range{Start: gopls.Position{}, End: gopls.Position{}}, NewText: "import \"fmt\"\n"}}}
	err = applyCompletionItem(buffer, item, editor.Position{Line: 1, Column: 29})
	require.NoError(t, err)
	assert.Contains(t, buffer.Text(), "fmt.Println }")
	assert.Contains(t, buffer.Text(), "import \"fmt\"\n")
	assert.True(t, buffer.Dirty())
	assert.Equal(t, editor.Position{Line: 2, Column: 34}, buffer.Cursor())
	assert.True(t, buffer.Undo())
	assert.Equal(t, "package main\nfunc main() { _ = \"𐐀\"; fmt.Pr }\n", buffer.Text())
}

func TestCompletionFallbackUsesInsertText(t *testing.T) {
	for _, test := range []struct {
		name, input, label, insert, want string
		caret                            int
	}{
		{name: "typed prefix", input: "pri", label: "println", insert: "println", caret: 3, want: "println"},
		{name: "selector prefix", input: "fmt.Pr", label: "Println", insert: "Println", caret: 6, want: "fmt.Println"},
		{name: "empty prefix", input: "", label: "println", insert: "println", caret: 0, want: "println"},
		{name: "label fallback", input: "fo", label: "format", caret: 2, want: "format"},
		{name: "unicode identifier", input: "变量", label: "变量名", caret: 2, want: "变量名"},
		{name: "digits and underscore", input: "foo_2", label: "foo_23", caret: 5, want: "foo_23"},
		{name: "numeric literal", input: "123", label: "println", caret: 3, want: "123println"},
	} {
		t.Run(test.name, func(t *testing.T) {
			buffer, err := editor.New(test.input)
			require.NoError(t, err)
			item := gopls.CompletionItem{Label: test.label, InsertText: test.insert}
			require.NoError(t, applyCompletionItem(buffer, item, editor.Position{Column: test.caret}))
			assert.Equal(t, test.want, buffer.Text())
			assert.Equal(t, editor.Position{Column: graphemeCount(test.want)}, buffer.Cursor())
			assert.True(t, buffer.Undo())
			assert.Equal(t, test.input, buffer.Text())
		})
	}
}

func TestCompletionFallbackRejectsInvalidTextAndCaret(t *testing.T) {
	buffer, err := editor.New("foo")
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		item   gopls.CompletionItem
		cursor editor.Position
		want   string
	}{
		{name: "invalid UTF-8", item: gopls.CompletionItem{InsertText: string([]byte{0xff})}, cursor: editor.Position{}, want: "not valid UTF-8"},
		{name: "invalid caret", item: gopls.CompletionItem{Label: "bar"}, cursor: editor.Position{Column: 8}, want: "out of range"},
		{name: "negative caret", item: gopls.CompletionItem{Label: "bar"}, cursor: editor.Position{Line: -1}, want: "out of range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, applyCompletionItem(buffer, test.item, test.cursor), test.want)
			assert.Equal(t, "foo", buffer.Text())
		})
	}
}

func TestApplyCompletionItemRejectsInvalidRangeWithoutEditing(t *testing.T) {
	buffer, err := editor.New("a𐐀b")
	require.NoError(t, err)
	item := gopls.CompletionItem{Label: "bad", TextEdit: &gopls.TextEdit{Range: gopls.Range{
		Start: gopls.Position{Character: 2}, End: gopls.Position{Character: 3},
	}, NewText: "bad"}}
	require.Error(t, applyCompletionItem(buffer, item, editor.Position{Column: 1}))
	assert.Equal(t, "a𐐀b", buffer.Text())
}

func TestCompletionPickerKeyboard(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{focus: focusEditor, mainFocus: focusEditor, syntax: &syntaxCache{}}
	setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "pri")
	state.completionVisible = true
	require.NoError(t, state.buffer.MoveTo(editor.Position{Column: 3}, false))
	state.completionRequest = languageCompletionRequest{snapshot: languageSnapshot{path: state.document.Path, text: state.buffer.Text()}, cursor: state.buffer.Cursor()}
	state.completionItems = []gopls.CompletionItem{{Label: "print"}, {Label: "println", InsertText: "println"}}
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)))
	assert.Equal(t, 1, state.completionSelected)
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)))
	assert.Equal(t, "println", state.buffer.Text())
	assert.False(t, state.completionVisible)
}

func TestCompletionRejectsOverlappingEdits(t *testing.T) {
	buffer, err := editor.New("Print")
	require.NoError(t, err)
	item := gopls.CompletionItem{Label: "Println", TextEdit: &gopls.TextEdit{Range: gopls.Range{
		Start: gopls.Position{}, End: gopls.Position{Character: 5},
	}, NewText: "Println"}, AdditionalTextEdits: []gopls.TextEdit{{Range: gopls.Range{Start: gopls.Position{Character: 2}, End: gopls.Position{Character: 3}}, NewText: "x"}}}
	require.ErrorContains(t, applyCompletionItem(buffer, item, editor.Position{Column: 5}), "overlap")
	assert.Equal(t, "Print", buffer.Text())
}

func TestCompletionRequestAndCancelShortcuts(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := &shellState{focus: focusEditor, mainFocus: focusEditor, languageStatus: "ready"}
	setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "pri")
	state.enqueueLanguage = func(languageSnapshot) {}
	requests := make([]languageCompletionRequest, 0, 2)
	state.enqueueCompletion = func(request languageCompletionRequest) { requests = append(requests, request) }
	for _, event := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModAlt),
	} {
		assert.False(t, handleKey(screen, state, event))
	}
	require.Len(t, requests, 2)
	assert.True(t, state.completionPending)
	assert.False(t, handleKey(screen, state, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)))
	state.applyCompletionResult(languageCompletionResult{request: requests[1], items: []gopls.CompletionItem{{Label: "println"}}})
	assert.False(t, state.completionVisible)
}

func TestCompletionWorkerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	requests := make(chan languageSnapshot, 1)
	completions := make(chan languageCompletionRequest, 1)
	events := make(chan languageEvent, 8)
	started := make(chan struct{})
	fake := &cancelingCompletionSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8), diags: make(chan gopls.PublishedDiagnostics)}, started: started}
	requests <- languageSnapshot{path: path, text: "package main\n", seq: 1}
	done := make(chan struct{})
	go func() {
		languageWorker(ctx, root, requests, nil, completions, events, func(context.Context, string) (languageSession, error) { return fake, nil })
		close(done)
	}()
	require.Eventually(t, func() bool { return fake.calls() != "" }, time.Second, time.Millisecond)
	completions <- languageCompletionRequest{snapshot: languageSnapshot{path: path, text: "package main\n", seq: 1}}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("completion did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion did not stop after cancellation")
	}
}

func TestCompletionResultOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*shellState)
		items   []gopls.CompletionItem
		err     error
		visible bool
		message string
	}{
		{name: "suggestions", items: []gopls.CompletionItem{{Label: "Println"}}, visible: true, message: "Completion:"},
		{name: "empty", message: "No completions"},
		{name: "server error", err: errors.New("not ready"), message: "gopls completion failed"},
		{name: "moved caret", change: func(state *shellState) { state.buffer.MoveRight(false) }, message: "file or caret changed"},
		{name: "lost focus", change: func(state *shellState) { state.setFocus(focusTree) }, message: "editor lost focus"},
		{name: "dialog opened", change: func(state *shellState) { state.menuOpen = true }, message: "another dialog is open"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &shellState{focus: focusEditor, mainFocus: focusEditor, completionSeq: 1, completionPending: true}
			setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "foo")
			request := languageCompletionRequest{snapshot: languageSnapshot{path: state.document.Path, text: state.buffer.Text()}, cursor: state.buffer.Cursor(), ticket: 1}
			if test.change != nil {
				test.change(state)
			}
			state.applyCompletionResult(languageCompletionResult{request: request, items: test.items, err: test.err})
			assert.Equal(t, test.visible, state.completionVisible)
			assert.Contains(t, state.message, test.message)
			assert.False(t, state.completionPending)
		})
	}
}

func TestCompletionPickerNavigationAndCancel(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 10)
	state := &shellState{completionVisible: true, completionItems: make([]gopls.CompletionItem, 20)}
	for _, test := range []struct {
		key  tcell.Key
		want int
	}{
		{tcell.KeyDown, 1},
		{tcell.KeyPgDn, 5},
		{tcell.KeyEnd, 19},
		{tcell.KeyUp, 18},
		{tcell.KeyPgUp, 14},
		{tcell.KeyHome, 0},
	} {
		state.handleCompletionKey(screen, tcell.NewEventKey(test.key, 0, tcell.ModNone))
		assert.Equal(t, test.want, state.completionSelected)
	}
	state.handleCompletionKey(screen, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	assert.False(t, state.completionVisible)
	assert.Equal(t, "Completion cancelled", state.message)
}

func TestCompletionPickerQuitPreservesUnsavedChanges(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	state := &shellState{completionVisible: true}
	setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "package main\n")
	require.NoError(t, state.buffer.Insert("// edit"))
	assert.False(t, state.handleCompletionKey(screen, tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone)))
	assert.False(t, state.completionVisible)
	assert.Equal(t, confirmQuit, state.confirm)
}

func TestCompletionRequestNeedsGoEditorAndGopls(t *testing.T) {
	state := &shellState{focus: focusEditor}
	state.requestCompletion()
	assert.Contains(t, state.message, "Open a Go file")
	setTestDocument(t, state, filepath.Join(t.TempDir(), "README.md"), "text")
	state.requestCompletion()
	assert.Contains(t, state.message, "Open a Go file")
	setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), "package main")
	state.setFocus(focusTree)
	state.requestCompletion()
	assert.Contains(t, state.message, "Focus the editor")
	state.setFocus(focusEditor)
	state.requestCompletion()
	assert.Contains(t, state.message, "gopls is unavailable")
}

func TestCompletionEditValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		item gopls.CompletionItem
		want string
	}{
		{name: "reversed", item: gopls.CompletionItem{TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{Character: 2}, End: gopls.Position{}}, NewText: "x"}}, want: "reversed"},
		{name: "past end", item: gopls.CompletionItem{TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{Character: 5}, End: gopls.Position{Character: 5}}, NewText: "x"}}, want: "out of range"},
		{name: "not at caret", item: gopls.CompletionItem{TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{}, End: gopls.Position{Character: 1}}, NewText: "x"}}, want: "does not contain"},
		{name: "invalid utf8", item: gopls.CompletionItem{TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{}, End: gopls.Position{Character: 3}}, NewText: string([]byte{0xff})}}, want: "not valid UTF-8"},
		{name: "negative server position", item: gopls.CompletionItem{TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{Line: -1}, End: gopls.Position{Character: 3}}, NewText: "x"}}, want: "out of range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			buffer, err := editor.New("foo")
			require.NoError(t, err)
			err = applyCompletionItem(buffer, test.item, editor.Position{Column: 3})
			require.ErrorContains(t, err, test.want)
			assert.Equal(t, "foo", buffer.Text())
		})
	}
}

func TestCompletionConnectionReportsResultsAndFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	request := languageCompletionRequest{snapshot: languageSnapshot{path: path, text: "package main\n", seq: 1}, ticket: 3}
	for _, test := range []struct {
		name    string
		session languageSession
		want    string
	}{
		{name: "suggestions", session: &fakeLanguageSession{log: make(chan string, 8)}, want: "println"},
		{name: "sync failure", session: &failingLanguageSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8)}, fail: "open"}, want: "open failed"},
		{name: "query failure", session: &errorCompletionSession{fakeLanguageSession: &fakeLanguageSession{log: make(chan string, 8)}}, want: "query failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan languageEvent, 4)
			connection := &languageConnection{session: test.session, ctx: context.Background(), events: events}
			connection.complete(request)
			var result languageCompletionResult
			for len(events) > 0 {
				event := <-events
				if event.kind == languageCompleted {
					result = event.completion
				}
			}
			assert.Equal(t, request, result.request)
			if result.err != nil {
				assert.ErrorContains(t, result.err, test.want)
			} else {
				require.Len(t, result.items, 1)
				assert.Equal(t, test.want, result.items[0].Label)
			}
		})
	}
}

type errorCompletionSession struct{ *fakeLanguageSession }

func (fake *errorCompletionSession) Complete(context.Context, string, gopls.Position) ([]gopls.CompletionItem, error) {
	return nil, errors.New("query failed")
}

func TestCompletionPickerRejectsChangedTextAndBadEdits(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	for _, test := range []struct {
		name, text string
		item       gopls.CompletionItem
		want       string
	}{
		{name: "changed file", text: "changed", item: gopls.CompletionItem{Label: "Println"}, want: "file changed"},
		{name: "invalid edit", text: "foo", item: gopls.CompletionItem{Label: "bad", TextEdit: &gopls.TextEdit{Range: gopls.Range{Start: gopls.Position{Character: 8}, End: gopls.Position{Character: 8}}, NewText: "bad"}}, want: "Cannot apply completion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &shellState{focus: focusEditor, mainFocus: focusEditor, completionVisible: true}
			setTestDocument(t, state, filepath.Join(t.TempDir(), "main.go"), test.text)
			state.completionItems = []gopls.CompletionItem{test.item}
			state.completionRequest.snapshot = languageSnapshot{path: state.document.Path, text: "foo"}
			state.handleCompletionKey(screen, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
			assert.Contains(t, state.message, test.want)
			assert.Equal(t, test.text, state.buffer.Text())
		})
	}
}

type cancelingCompletionSession struct {
	*fakeLanguageSession
	started chan struct{}
}

func (fake *cancelingCompletionSession) Complete(ctx context.Context, _ string, _ gopls.Position) ([]gopls.CompletionItem, error) {
	close(fake.started)
	<-ctx.Done()
	return nil, errors.New("cancelled")
}

package gopls

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeCompletions(t *testing.T) {
	for _, test := range []struct {
		name, input string
		want        int
	}{
		{"null", `null`, 0},
		{"array", `[{"label":"Println","insertText":"Println"}]`, 1},
		{"list", `{"isIncomplete":true,"items":[{"label":"Println","textEdit":{"range":{"start":{"line":1,"character":4},"end":{"line":1,"character":6}},"newText":"Println"},"additionalTextEdits":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"import \"fmt\"\n"}]}]}`, 1},
		{"insert replace", `[{"label":"Println","textEdit":{"insert":{"start":{"line":1,"character":4},"end":{"line":1,"character":6}},"replace":{"start":{"line":1,"character":4},"end":{"line":1,"character":8}},"newText":"Println"}}]`, 1},
		{"snippets skipped", `[{"label":"snippet","insertTextFormat":2,"insertText":"fn(${1:name})"}]`, 0},
		{"whitespace", `  [{"label":"Println"}]  `, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, err := decodeCompletions(json.RawMessage(test.input))
			require.NoError(t, err)
			assert.Len(t, items, test.want)
			if test.name == "list" {
				assert.Equal(t, "Println", items[0].TextEdit.NewText)
				assert.Len(t, items[0].AdditionalTextEdits, 1)
			}
			if test.name == "insert replace" {
				assert.Equal(t, 8, items[0].TextEdit.Range.End.Character)
			}
		})
	}
	_, err := decodeCompletions(json.RawMessage(`{"items":42}`))
	require.ErrorContains(t, err, "decode gopls completion")
	_, err = decodeCompletions(json.RawMessage(`[{"label":"bad","textEdit":{"newText":"bad"}}]`))
	require.ErrorContains(t, err, "missing range")
	_, err = decodeCompletions(json.RawMessage(`[{"label":"bad","textEdit":"bad"}]`))
	require.ErrorContains(t, err, "decode gopls completion edit")
	_, err = decodeCompletions(json.RawMessage(`[{"label":42}]`))
	require.ErrorContains(t, err, "decode gopls completion item")
	_, err = decodeCompletions(json.RawMessage(`[{`))
	require.ErrorContains(t, err, "decode gopls completion")
}

func TestCompleteRequiresOpenDocument(t *testing.T) {
	session := &Session{opened: make(map[string]int)}
	_, err := session.Complete(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
}

func TestCompleteReportsClosedConnection(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	session := &Session{opened: map[string]int{"main.go": 1}, waits: make(map[int]chan reply), closed: closed, output: nopWriteCloser{&bytes.Buffer{}}}
	_, err := session.Complete(context.Background(), "main.go", Position{})
	require.ErrorContains(t, err, "connection closed")
}

func TestCompleteRequestsSuggestionsFromServer(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		initialize, err := readMessage(reader)
		if err == nil {
			err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": initialize.ID, "result": map[string]any{}})
		}
		for range 2 {
			if err != nil {
				break
			}
			_, err = readMessage(reader)
		}
		if err == nil {
			var request wireMessage
			request, err = readMessage(reader)
			if err == nil {
				assert.Equal(t, "textDocument/completion", request.Method)
				err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": []map[string]any{{"label": "Println", "insertText": "Println"}}})
			}
		}
		done <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	root := t.TempDir()
	session, err := Connect(ctx, client, client, root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	path := filepath.Join(root, "main.go")
	require.NoError(t, session.Open(path, "package main\n"))
	items, err := session.Complete(ctx, path, Position{Line: 0, Character: 5})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Println", items[0].Label)
	require.NoError(t, <-done)
}

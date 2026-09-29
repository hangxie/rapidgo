package gopls

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeLocations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a b.go")
	uri := fileURI(path)
	for _, test := range []struct {
		name, raw string
		want      int
		column    int
	}{
		{"null", `null`, 0, 0},
		{"single", `{"uri":"` + uri + `","range":{"start":{"line":1,"character":3},"end":{"line":1,"character":5}}}`, 1, 3},
		{"array", `[{"uri":"` + uri + `","range":{"start":{"line":1,"character":3}}}]`, 1, 3},
		{"link", `[{"targetUri":"` + uri + `","targetRange":{"start":{"line":0,"character":0}},"targetSelectionRange":{"start":{"line":2,"character":7}}}]`, 1, 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, err := decodeLocations(json.RawMessage(test.raw))
			require.NoError(t, err)
			require.Len(t, items, test.want)
			if test.want > 0 {
				assert.Equal(t, path, items[0].Path)
				assert.Equal(t, test.column, items[0].Position.Character)
			}
		})
	}
	for _, raw := range []string{`{`, `{"uri":"https://example.com/a.go","range":{"start":{"line":0,"character":0}}}`, `{"uri":"` + uri + `"}`, `{"uri":"` + uri + `","range":{"start":{"line":-1,"character":0}}}`} {
		_, err := decodeLocations(json.RawMessage(raw))
		require.Error(t, err, raw)
	}
}

func TestNavigationRequestsServer(t *testing.T) {
	for _, test := range []struct {
		name, method string
		query        func(*Session, context.Context, string, Position) ([]Location, error)
	}{
		{"definition", "textDocument/definition", (*Session).Definition},
		{"references", "textDocument/references", (*Session).References},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = server.Close() })
			root := t.TempDir()
			path := filepath.Join(root, "main.go")
			done := make(chan error, 1)
			go func() {
				reader := bufio.NewReader(server)
				initialize, err := readMessage(reader)
				if err == nil {
					err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": initialize.ID, "result": map[string]any{}})
				}
				for range 2 {
					if err == nil {
						_, err = readMessage(reader)
					}
				}
				if err == nil {
					var request wireMessage
					request, err = readMessage(reader)
					if err == nil {
						assert.Equal(t, test.method, request.Method)
						assert.Contains(t, string(request.Params), fileURI(path))
						assert.Contains(t, string(request.Params), `"character":4`)
						if test.name == "references" {
							assert.Contains(t, string(request.Params), `"includeDeclaration":true`)
						}
						err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": []map[string]any{{"uri": fileURI(path), "range": map[string]any{"start": Position{Line: 1, Character: 2}}}}})
					}
				}
				done <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			session, err := Connect(ctx, client, client, root)
			require.NoError(t, err)
			t.Cleanup(func() { _ = session.Close() })
			require.NoError(t, session.Open(path, "package main\n"))
			locations, err := test.query(session, ctx, path, Position{Character: 4})
			require.NoError(t, err)
			assert.Equal(t, []Location{{Path: path, Position: Position{Line: 1, Character: 2}}}, locations)
			require.NoError(t, <-done)
		})
	}
}

func TestNavigationRejectsUnopenedDocument(t *testing.T) {
	session := &Session{opened: map[string]int{}}
	_, err := session.Definition(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
	_, err = session.References(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
}

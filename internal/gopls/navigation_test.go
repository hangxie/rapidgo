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
		{"empty response", ``, 0, 0},
		{"null", `null`, 0, 0},
		{"single", `{"uri":"` + uri + `","range":{"start":{"line":1,"character":3},"end":{"line":1,"character":5}}}`, 1, 3},
		{"array", `[{"uri":"` + uri + `","range":{"start":{"line":1,"character":3}}}]`, 1, 3},
		{"link", `[{"targetUri":"` + uri + `","targetRange":{"start":{"line":0,"character":0}},"targetSelectionRange":{"start":{"line":2,"character":7}}}]`, 1, 7},
		{"link without selection range", `{"targetUri":"` + uri + `","targetRange":{"start":{"line":3,"character":9}}}`, 1, 9},
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
	for _, raw := range []string{
		`{`,
		`[{]`,
		`[42]`,
		`{"uri":"https://example.com/a.go","range":{"start":{"line":0,"character":0}}}`,
		`{"uri":"` + uri + `"}`,
		`{"uri":"` + uri + `","range":{"start":{"line":-1,"character":0}}}`,
		`{"uri":"` + uri + `","range":{"start":{"line":0,"character":-1}}}`,
	} {
		_, err := decodeLocations(json.RawMessage(raw))
		require.Error(t, err, raw)
	}
}

type navigationTestCase struct {
	name, method, wantError string
	query                   func(*Session, context.Context, string, Position) ([]Location, error)
	result                  any
}

func TestNavigationRequestsServer(t *testing.T) {
	for _, test := range []navigationTestCase{
		{name: "definition", method: "textDocument/definition", query: (*Session).Definition},
		{name: "references", method: "textDocument/references", query: (*Session).References},
		{name: "malformed definition", method: "textDocument/definition", query: (*Session).Definition, result: map[string]any{"uri": "file:///missing.go"}, wantError: "decode gopls definition: location has no range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = server.Close() })
			root := t.TempDir()
			path := filepath.Join(root, "main.go")
			done := make(chan error, 1)
			go func() { done <- serveNavigationRequest(t, server, test, path) }()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			session, err := Connect(ctx, client, client, root)
			require.NoError(t, err)
			t.Cleanup(func() { _ = session.Close() })
			require.NoError(t, session.Open(path, "package main\n"))
			locations, err := test.query(session, ctx, path, Position{Character: 4})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Empty(t, locations)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []Location{{Path: path, Position: Position{Line: 1, Character: 2}}}, locations)
			}
			require.NoError(t, <-done)
		})
	}
}

func serveNavigationRequest(t *testing.T, server net.Conn, test navigationTestCase, path string) error {
	t.Helper()
	reader := bufio.NewReader(server)
	initialize, err := readMessage(reader)
	if err != nil {
		return err
	}
	if err := writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": initialize.ID, "result": map[string]any{}}); err != nil {
		return err
	}
	for range 2 {
		if _, err := readMessage(reader); err != nil {
			return err
		}
	}
	request, err := readMessage(reader)
	if err != nil {
		return err
	}
	assert.Equal(t, test.method, request.Method)
	assert.Contains(t, string(request.Params), fileURI(path))
	assert.Contains(t, string(request.Params), `"character":4`)
	if test.name == "references" {
		assert.Contains(t, string(request.Params), `"includeDeclaration":true`)
	}
	result := test.result
	if result == nil {
		result = []map[string]any{{"uri": fileURI(path), "range": map[string]any{"start": Position{Line: 1, Character: 2}}}}
	}
	return writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func TestNavigationRejectsUnopenedDocument(t *testing.T) {
	session := &Session{opened: map[string]int{}}
	_, err := session.Definition(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
	_, err = session.References(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
}

func TestNavigationReportsConnectionError(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	session := &Session{
		opened: map[string]int{"open.go": 1},
		waits:  make(map[int]chan reply),
		closed: closed,
	}
	_, err := session.Definition(context.Background(), "open.go", Position{})
	require.ErrorContains(t, err, "gopls connection closed")
}

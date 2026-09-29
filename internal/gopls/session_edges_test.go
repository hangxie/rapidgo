package gopls

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerRequestResponses(t *testing.T) {
	for _, test := range []struct {
		method string
		params string
		result string
		code   int
	}{
		{method: "workspace/configuration", params: `{"items":[{},{}]}`, result: `[null,null]`},
		{method: "workspace/configuration", params: `{`, code: -32602},
		{method: "workspace/workspaceFolders", params: `{}`, result: `"name"`},
		{method: "client/registerCapability", params: `{}`, result: `null`},
		{method: "unknown/method", params: `{}`, code: -32601},
	} {
		t.Run(test.method+test.params, func(t *testing.T) {
			var output bytes.Buffer
			session := &Session{root: t.TempDir(), output: nopWriteCloser{&output}, closed: make(chan struct{})}
			session.handleServerRequest(message{ID: json.RawMessage(`7`), Method: test.method, Params: json.RawMessage(test.params)})
			response, err := readFrame(bufio.NewReader(&output))
			require.NoError(t, err)
			assert.Equal(t, `7`, string(response.ID))
			if test.code != 0 {
				require.NotNil(t, response.Error)
				assert.Equal(t, test.code, response.Error.Code)
			} else {
				assert.Contains(t, string(response.Result), test.result)
			}
		})
	}
}

func TestPublishedDiagnosticsReplaceFullQueue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	session := &Session{diags: make(chan PublishedDiagnostics, 1)}
	session.publish(json.RawMessage(`{`))
	session.publish(json.RawMessage(`{"uri":"https://example.com/file.go","diagnostics":[]}`))
	assert.Empty(t, session.diags)
	session.publish(json.RawMessage(`{"uri":"` + fileURI(path) + `","version":1,"diagnostics":[]}`))
	session.publish(json.RawMessage(`{"uri":"` + fileURI(path) + `","version":2,"diagnostics":[{"message":"new"}]}`))
	require.Len(t, session.diags, 1)
	event := <-session.diags
	assert.Equal(t, 2, event.Version)
	assert.Equal(t, "new", event.Items[0].Message)
}

func TestSessionClosedAndDocumentErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	closed := make(chan struct{})
	close(closed)
	session := &Session{output: nopWriteCloser{&bytes.Buffer{}}, opened: make(map[string]int), closed: closed}
	err := session.Open(path, "package main\n")
	require.ErrorContains(t, err, "connection closed")
	assert.Empty(t, session.opened, "failed open must be rolled back")
	require.ErrorContains(t, session.Change(path, "changed"), "not open")
	require.ErrorContains(t, session.CloseDocument(path), "not open")
	session.err = errors.New("broken pipe")
	require.ErrorContains(t, session.connectionError(), "broken pipe")
	session.err = io.EOF
	assert.EqualError(t, session.connectionError(), "gopls connection closed")
}

func TestInitializeServerErrorClosesConnection(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	done := make(chan error, 1)
	go func() {
		request, err := readMessage(bufio.NewReader(server))
		if err == nil {
			err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": responseError{Code: -32603, Message: "workspace unavailable"}})
		}
		done <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Connect(ctx, client, client, t.TempDir())
	require.ErrorContains(t, err, "initialize gopls")
	require.ErrorContains(t, err, "workspace unavailable")
	require.NoError(t, <-done)
}

func TestDocumentOpenRejectsDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	var output bytes.Buffer
	session := &Session{opened: make(map[string]int), closed: make(chan struct{}), output: nopWriteCloser{&output}}
	require.NoError(t, session.Open(path, "package main\n"))
	require.ErrorContains(t, session.Open(path, "package main\n"), "already open")
	assert.Equal(t, 1, session.opened[path])
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

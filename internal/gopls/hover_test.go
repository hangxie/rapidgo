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

func TestHoverContents(t *testing.T) {
	for _, test := range []struct {
		input, expected string
	}{
		{`null`, ""},
		{`"symbol"`, "symbol"},
		{`{"kind":"plaintext","value":"func main()"}`, "func main()"},
		{`["func main()",{"language":"go","value":"package main"}]`, "func main()\npackage main"},
	} {
		actual, err := hoverContents(json.RawMessage(test.input))
		require.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}
	_, err := hoverContents(json.RawMessage(`42`))
	require.Error(t, err)
	_, err = hoverContents(json.RawMessage(`["valid",42]`))
	require.Error(t, err)
}

func TestHoverRequiresOpenDocument(t *testing.T) {
	session := &Session{opened: make(map[string]int)}
	_, err := session.Hover(context.Background(), "missing.go", Position{})
	require.ErrorContains(t, err, "document is not open")
}

func TestHoverReportsClosedConnection(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	path := "main.go"
	session := &Session{opened: map[string]int{path: 1}, waits: make(map[int]chan reply), closed: closed, output: nopWriteCloser{&bytes.Buffer{}}}
	_, err := session.Hover(context.Background(), path, Position{})
	require.ErrorContains(t, err, "connection closed")
}

func TestHoverRejectsMalformedServerResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		result any
		want   string
	}{
		{name: "invalid shape", result: "unexpected", want: "decode gopls hover"},
		{name: "invalid contents", result: map[string]any{"contents": 42}, want: "unsupported gopls hover contents"},
	} {
		t.Run(test.name, func(t *testing.T) {
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
					_, err = readMessage(reader) // initialized and didOpen
				}
				if err == nil {
					var hover wireMessage
					hover, err = readMessage(reader)
					if err == nil {
						err = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": hover.ID, "result": test.result})
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
			_, err = session.Hover(ctx, path, Position{})
			require.ErrorContains(t, err, test.want)
			require.NoError(t, <-done)
		})
	}
}

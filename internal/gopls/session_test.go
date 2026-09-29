package gopls

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionInitializeAndSync(t *testing.T) {
	client, server := net.Pipe()
	defer func() { require.NoError(t, server.Close()) }()
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		message, err := readMessage(reader)
		if err != nil {
			done <- err
			return
		}
		if message.Method != "initialize" || !strings.Contains(string(message.Params), `"workspaceFolders"`) {
			done <- fmt.Errorf("unexpected initialize: %s", message.Params)
			return
		}
		if err := writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{"capabilities": map[string]any{"textDocumentSync": 1}}}); err != nil {
			done <- err
			return
		}
		for _, method := range []string{"initialized", "textDocument/didOpen", "textDocument/didChange", "textDocument/didClose"} {
			message, err = readMessage(reader)
			if err != nil {
				done <- err
				return
			}
			if message.Method != method {
				done <- fmt.Errorf("got %s, want %s", message.Method, method)
				return
			}
			if method == "textDocument/didChange" && !strings.Contains(string(message.Params), `"version":2`) {
				done <- fmt.Errorf("change has wrong version: %s", message.Params)
				return
			}
		}
		done <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Connect(ctx, client, client, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, session.Close()) }()
	require.NoError(t, session.Open(path, "package main\n"))
	require.NoError(t, session.Change(path, "package main\nfunc main() {}\n"))
	require.NoError(t, session.CloseDocument(path))
	require.NoError(t, <-done)
}

func TestSessionPublishesDiagnostics(t *testing.T) {
	client, server := net.Pipe()
	defer func() { require.NoError(t, server.Close()) }()
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	go func() {
		reader := bufio.NewReader(server)
		init, err := readMessage(reader)
		if err != nil {
			return
		}
		_ = writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": init.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		_, _ = readMessage(reader) // initialized
		_ = writeMessage(server, map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": map[string]any{
			"uri": fileURI(path), "version": 2, "diagnostics": []any{map[string]any{
				"range":    map[string]any{"start": map[string]int{"line": 0, "character": 4}, "end": map[string]int{"line": 0, "character": 5}},
				"severity": 1, "message": "undefined: x", "source": "compiler",
			}},
		}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Connect(ctx, client, client, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, session.Close()) }()
	select {
	case event := <-session.Diagnostics():
		require.Equal(t, path, event.Path)
		require.Equal(t, 2, event.Version)
		require.Equal(t, "undefined: x", event.Items[0].Message)
	case <-ctx.Done():
		t.Fatal("diagnostic notification not delivered")
	}
}

func TestSessionHover(t *testing.T) {
	client, server := net.Pipe()
	defer func() { require.NoError(t, server.Close()) }()
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		init, err := readMessage(reader)
		if err != nil {
			done <- err
			return
		}
		if err := writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": init.ID, "result": map[string]any{"capabilities": map[string]any{}}}); err != nil {
			done <- err
			return
		}
		for range 2 { // initialized, didOpen
			if _, err := readMessage(reader); err != nil {
				done <- err
				return
			}
		}
		hover, err := readMessage(reader)
		if err != nil {
			done <- err
			return
		}
		if hover.Method != "textDocument/hover" || !strings.Contains(string(hover.Params), `"character":5`) {
			done <- fmt.Errorf("unexpected hover request: %+v", hover)
			return
		}
		done <- writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": hover.ID, "result": map[string]any{"contents": map[string]string{"kind": "plaintext", "value": "func main()"}}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Connect(ctx, client, client, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, session.Close()) }()
	require.NoError(t, session.Open(path, "package main\nfunc main() {}\n"))
	content, err := session.Hover(ctx, path, Position{Line: 1, Character: 5})
	require.NoError(t, err)
	require.Equal(t, "func main()", content)
	require.NoError(t, <-done)
}

func TestConnectCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer func() { require.NoError(t, server.Close()) }()
	ready := make(chan struct{})
	go func() {
		_, _ = readMessage(bufio.NewReader(server))
		close(ready)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	result := make(chan error, 1)
	go func() {
		_, err := Connect(ctx, client, client, root)
		result <- err
	}()
	<-ready
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("Connect did not stop after cancellation")
	}
}

func TestInitializeHandlesServerRequestWithSameID(t *testing.T) {
	client, server := net.Pipe()
	require.NoError(t, server.SetDeadline(time.Now().Add(3*time.Second)))
	defer func() { require.NoError(t, server.Close()) }()
	root := t.TempDir()
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		init, err := readMessage(reader)
		if err != nil {
			done <- err
			return
		}
		if err := writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]string{"section": "gopls"}}}}); err != nil {
			done <- err
			return
		}
		response, err := readMessage(reader)
		if err != nil {
			done <- err
			return
		}
		if string(response.ID) != "1" || string(response.Result) != "[null]" {
			done <- fmt.Errorf("unexpected configuration response: %+v", response)
			return
		}
		if err := writeMessage(server, map[string]any{"jsonrpc": "2.0", "id": init.ID, "result": map[string]any{"capabilities": map[string]any{}}}); err != nil {
			done <- err
			return
		}
		_, err = readMessage(reader) // initialized
		done <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Connect(ctx, client, client, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, session.Close()) }()
	require.NoError(t, <-done)
}

type wireMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

func readMessage(reader *bufio.Reader) (wireMessage, error) {
	var size int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return wireMessage{}, err
		}
		if line == "\r\n" {
			break
		}
		if _, err := fmt.Sscanf(line, "Content-Length: %d", &size); err != nil {
			return wireMessage{}, err
		}
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return wireMessage{}, err
	}
	var message wireMessage
	err := json.Unmarshal(data, &message)
	return message, err
}

func writeMessage(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

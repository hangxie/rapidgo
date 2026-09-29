// Package gopls adapts language-server messages without terminal dependencies.
package gopls

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// Diagnostic is a problem reported in UTF-16 LSP coordinates.
type Diagnostic struct {
	Range struct {
		Start Position `json:"start"`
		End   Position `json:"end"`
	} `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// Position is a zero-based line and UTF-16 character offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// PublishedDiagnostics replaces the diagnostics for one document.
type PublishedDiagnostics struct {
	Path    string
	Version int
	Items   []Diagnostic
}

type reply struct {
	message message
	err     error
}

// Session exchanges LSP messages with one gopls server.
type Session struct {
	root        string
	input       io.ReadCloser
	output      io.WriteCloser
	writes      sync.Mutex
	documentOps sync.Mutex
	state       sync.Mutex
	nextID      int
	waits       map[int]chan reply
	opened      map[string]int
	closed      chan struct{}
	diags       chan PublishedDiagnostics
	err         error
	once        sync.Once
	stopProcess context.CancelFunc
	processDone <-chan error
}

// Connect initializes an LSP connection scoped to root.
func Connect(ctx context.Context, input io.ReadCloser, output io.WriteCloser, root string) (*Session, error) {
	s := &Session{root: root, input: input, output: output, waits: make(map[int]chan reply), opened: make(map[string]int), closed: make(chan struct{}), diags: make(chan PublishedDiagnostics, 64)}
	go s.readLoop()
	uri := fileURI(root)
	params := map[string]any{
		"processId": os.Getpid(), "rootUri": uri,
		"workspaceFolders": []map[string]string{{"uri": uri, "name": root}},
		"capabilities":     map[string]any{"textDocument": map[string]any{"hover": map[string]any{"contentFormat": []string{"plaintext"}}, "publishDiagnostics": map[string]any{"versionSupport": true}, "synchronization": map[string]any{"dynamicRegistration": false}}},
	}
	if _, err := s.request(ctx, "initialize", params); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("initialize gopls: %w", err)
	}
	if err := s.notify("initialized", map[string]any{}); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("notify gopls initialized: %w", err)
	}
	return s, nil
}

func (s *Session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.state.Lock()
	s.nextID++
	id := s.nextID
	wait := make(chan reply, 1)
	s.waits[id] = wait
	s.state.Unlock()
	defer func() {
		s.state.Lock()
		delete(s.waits, id)
		s.state.Unlock()
	}()
	if err := s.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case result := <-wait:
		if result.err != nil {
			return nil, result.err
		}
		if result.message.Error != nil {
			return nil, fmt.Errorf("LSP %s: %s", method, result.message.Error.Message)
		}
		return result.message.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closed:
		return nil, s.connectionError()
	}
}

func (s *Session) notify(method string, params any) error {
	return s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *Session) send(value any) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	select {
	case <-s.closed:
		return s.connectionError()
	default:
	}
	return writeFrame(s.output, value)
}

// Open tells gopls about the current in-memory text of a Go file.
func (s *Session) Open(path, content string) error {
	s.documentOps.Lock()
	defer s.documentOps.Unlock()
	s.state.Lock()
	if _, exists := s.opened[path]; exists {
		s.state.Unlock()
		return fmt.Errorf("document already open: %s", path)
	}
	s.opened[path] = 1
	s.state.Unlock()
	if err := s.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": fileURI(path), "languageId": "go", "version": 1, "text": content}}); err != nil {
		s.state.Lock()
		delete(s.opened, path)
		s.state.Unlock()
		return err
	}
	return nil
}

// Change sends a full-text replacement with a monotonically increasing version.
func (s *Session) Change(path, content string) error {
	s.documentOps.Lock()
	defer s.documentOps.Unlock()
	s.state.Lock()
	version, exists := s.opened[path]
	if !exists {
		s.state.Unlock()
		return fmt.Errorf("document is not open: %s", path)
	}
	version++
	s.opened[path] = version
	s.state.Unlock()
	return s.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": fileURI(path), "version": version}, "contentChanges": []map[string]string{{"text": content}}})
}

// CloseDocument releases an in-memory overlay.
func (s *Session) CloseDocument(path string) error {
	s.documentOps.Lock()
	defer s.documentOps.Unlock()
	s.state.Lock()
	if _, exists := s.opened[path]; !exists {
		s.state.Unlock()
		return fmt.Errorf("document is not open: %s", path)
	}
	delete(s.opened, path)
	s.state.Unlock()
	return s.notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": fileURI(path)}})
}

// Diagnostics receives replacement diagnostic sets without blocking the reader.
func (s *Session) Diagnostics() <-chan PublishedDiagnostics { return s.diags }

// Close stops the connection and unblocks pending requests.
func (s *Session) Close() error {
	s.once.Do(func() {
		if s.stopProcess != nil {
			s.stopProcess()
		}
		_ = s.input.Close()
		_ = s.output.Close()
		if s.processDone != nil {
			<-s.processDone
		}
	})
	return nil
}

func (s *Session) readLoop() {
	defer close(s.closed)
	defer close(s.diags)
	reader := bufio.NewReader(s.input)
	for {
		item, err := readFrame(reader)
		if err != nil {
			s.state.Lock()
			s.err = err
			s.state.Unlock()
			return
		}
		if item.Method == "textDocument/publishDiagnostics" {
			s.publish(item.Params)
			continue
		}
		if item.Method != "" {
			if len(item.ID) != 0 {
				s.handleServerRequest(item)
			}
			continue
		}
		if len(item.ID) == 0 {
			continue
		}
		var id int
		if err := json.Unmarshal(item.ID, &id); err != nil {
			continue
		}
		s.state.Lock()
		wait := s.waits[id]
		s.state.Unlock()
		if wait != nil {
			wait <- reply{message: item}
		}
	}
}

func (s *Session) handleServerRequest(item message) {
	response := map[string]any{"jsonrpc": "2.0", "id": item.ID}
	switch item.Method {
	case "workspace/configuration":
		var params struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(item.Params, &params) != nil {
			response["error"] = responseError{Code: -32602, Message: "invalid configuration request"}
			break
		}
		response["result"] = make([]any, len(params.Items))
	case "workspace/workspaceFolders":
		response["result"] = []map[string]string{{"uri": fileURI(s.root), "name": s.root}}
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability":
		response["result"] = nil
	default:
		response["error"] = responseError{Code: -32601, Message: "method not supported"}
	}
	_ = s.send(response)
}

func (s *Session) publish(raw json.RawMessage) {
	var payload struct {
		URI         string       `json:"uri"`
		Version     int          `json:"version"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	path, err := pathFromURI(payload.URI)
	if err != nil {
		return
	}
	event := PublishedDiagnostics{Path: path, Version: payload.Version, Items: payload.Diagnostics}
	select {
	case s.diags <- event:
	default:
		select {
		case <-s.diags:
		default:
		}
		select {
		case s.diags <- event:
		default:
		}
	}
}

func (s *Session) connectionError() error {
	s.state.Lock()
	defer s.state.Unlock()
	if s.err == nil || errors.Is(s.err, io.EOF) {
		return errors.New("gopls connection closed")
	}
	return fmt.Errorf("gopls connection closed: %w", s.err)
}

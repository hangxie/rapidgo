package gopls

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const maxMessageBytes = 16 << 20

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *responseError  `json:"error"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func readFrame(reader *bufio.Reader) (message, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return message{}, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		key, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		if !ok {
			return message{}, fmt.Errorf("invalid LSP header %q", line)
		}
		if strings.EqualFold(key, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return message{}, fmt.Errorf("invalid LSP content length: %w", err)
			}
		}
	}
	if length < 0 || length > maxMessageBytes {
		return message{}, fmt.Errorf("LSP content length %d outside allowed range", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return message{}, err
	}
	var item message
	if err := json.Unmarshal(data, &item); err != nil {
		return message{}, fmt.Errorf("decode LSP message: %w", err)
	}
	return item, nil
}

func writeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode LSP message: %w", err)
	}
	if len(data) > maxMessageBytes {
		return errors.New("LSP message exceeds size limit")
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return fmt.Errorf("write LSP header: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write LSP body: %w", err)
	}
	return nil
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func pathFromURI(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
		return "", fmt.Errorf("invalid local file URI %q", uri)
	}
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}

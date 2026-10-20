package gopls

import (
	"bufio"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/hangxie/rapidgo/internal/frame"
	"github.com/hangxie/rapidgo/internal/i18n"
)

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
	var item message
	if err := frame.Read(reader, "LSP", &item); err != nil {
		return message{}, err
	}
	return item, nil
}

func writeFrame(writer io.Writer, value any) error { return frame.Write(writer, "LSP", value) }

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
		return "", i18n.Errorf("msg_invalid_local_file_uri_q", uri)
	}
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}

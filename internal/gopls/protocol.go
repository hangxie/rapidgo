package gopls

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
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
			return message{}, i18n.Errorf("msg_invalid_lsp_header_q", line)
		}
		if strings.EqualFold(key, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return message{}, i18n.Errorf("msg_invalid_lsp_content_length_w", err)
			}
		}
	}
	if length < 0 || length > maxMessageBytes {
		return message{}, i18n.Errorf("msg_lsp_content_length_d_outside_allowed_range", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return message{}, err
	}
	var item message
	if err := json.Unmarshal(data, &item); err != nil {
		return message{}, i18n.Errorf("msg_decode_lsp_message_w", err)
	}
	return item, nil
}

func writeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return i18n.Errorf("msg_encode_lsp_message_w", err)
	}
	if len(data) > maxMessageBytes {
		return i18n.Error("msg_lsp_message_exceeds_size_limit")
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return i18n.Errorf("msg_write_lsp_header_w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return i18n.Errorf("msg_write_lsp_body_w", err)
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
		return "", i18n.Errorf("msg_invalid_local_file_uri_q", uri)
	}
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}

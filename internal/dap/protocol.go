package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
)

const maxMessageBytes = 16 << 20

// message is the union of DAP requests, responses, and events.
type message struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	Event      string          `json:"event,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Success    bool            `json:"success,omitempty"`
	Message    string          `json:"message,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// failure extracts the most specific text from an unsuccessful response.
func (m message) failure() string {
	var body struct {
		Error struct {
			Format string `json:"format"`
		} `json:"error"`
	}
	if json.Unmarshal(m.Body, &body) == nil && body.Error.Format != "" {
		return body.Error.Format
	}
	if m.Message != "" {
		return m.Message
	}
	return i18n.Text("msg_dap_request_failed")
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
			return message{}, i18n.Errorf("msg_invalid_dap_header_q", line)
		}
		if strings.EqualFold(key, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return message{}, i18n.Errorf("msg_invalid_dap_content_length_w", err)
			}
		}
	}
	if length < 0 || length > maxMessageBytes {
		return message{}, i18n.Errorf("msg_dap_content_length_d_outside_allowed_range", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return message{}, err
	}
	var item message
	if err := json.Unmarshal(data, &item); err != nil {
		return message{}, i18n.Errorf("msg_decode_dap_message_w", err)
	}
	return item, nil
}

func writeFrame(writer io.Writer, value message) error {
	data, err := json.Marshal(value)
	if err != nil {
		return i18n.Errorf("msg_encode_dap_message_w", err)
	}
	if len(data) > maxMessageBytes {
		return i18n.Error("msg_dap_message_exceeds_size_limit")
	}
	// One write keeps the header and body together on stream connections.
	frame := make([]byte, 0, len(data)+32)
	frame = fmt.Appendf(frame, "Content-Length: %d\r\n\r\n", len(data))
	frame = append(frame, data...)
	if _, err := writer.Write(frame); err != nil {
		return i18n.Errorf("msg_write_dap_message_w", err)
	}
	return nil
}

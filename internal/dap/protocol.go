package dap

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/hangxie/rapidgo/internal/frame"
	"github.com/hangxie/rapidgo/internal/i18n"
)

// message is the union of DAP requests, responses, and events.
type message struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	Event      string          `json:"event,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Success    bool            `json:"success"` // required in every response, including refusals
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
	var item message
	if err := frame.Read(reader, "DAP", &item); err != nil {
		return message{}, err
	}
	return item, nil
}

func writeFrame(writer io.Writer, value message) error { return frame.Write(writer, "DAP", value) }

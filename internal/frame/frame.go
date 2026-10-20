// Package frame reads and writes the Content-Length framed JSON messages that LSP and DAP share.
package frame

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
)

const maxBytes = 16 << 20

// Read decodes the next frame into value; protocol names the protocol in errors.
func Read(reader *bufio.Reader, protocol string, value any) error {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		key, text, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		if !ok {
			return i18n.Errorf("msg_invalid_s_header_q", protocol, line)
		}
		if strings.EqualFold(key, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(text))
			if err != nil {
				return i18n.Errorf("msg_invalid_s_content_length_w", protocol, err)
			}
		}
	}
	if length < 0 || length > maxBytes {
		return i18n.Errorf("msg_s_content_length_d_outside_allowed_range", protocol, length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return i18n.Errorf("msg_decode_s_message_w", protocol, err)
	}
	return nil
}

// Write encodes value as one frame; protocol names the protocol in errors.
func Write(writer io.Writer, protocol string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return i18n.Errorf("msg_encode_s_message_w", protocol, err)
	}
	if len(data) > maxBytes {
		return i18n.Errorf("msg_s_message_exceeds_size_limit", protocol)
	}
	// One write keeps the header and body together on stream connections.
	frame := make([]byte, 0, len(data)+32)
	frame = fmt.Appendf(frame, "Content-Length: %d\r\n\r\n", len(data))
	frame = append(frame, data...)
	written, err := writer.Write(frame)
	if err == nil && written < len(frame) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return i18n.Errorf("msg_write_s_message_w", protocol, err)
	}
	return nil
}

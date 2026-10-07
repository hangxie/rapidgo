package dap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFrame(t *testing.T) {
	for _, test := range []struct {
		name, input, err string
		want             message
	}{
		{name: "event", input: "Content-Length: 41\r\n\r\n{\"seq\":3,\"type\":\"event\",\"event\":\"exited\"}", want: message{Seq: 3, Type: "event", Event: "exited"}},
		{name: "extra header and lowercase key", input: "Content-Type: x\r\ncontent-length: 24\r\n\r\n{\"seq\":1,\"type\":\"event\"}", want: message{Seq: 1, Type: "event"}},
		{name: "bare newlines", input: "Content-Length: 2\n\n{}", want: message{}},
		{name: "missing colon", input: "garbage\r\n\r\n", err: `invalid DAP header "garbage\r\n"`},
		{name: "bad length", input: "Content-Length: x\r\n\r\n", err: "invalid DAP content length"},
		{name: "missing length", input: "\r\n", err: "DAP content length -1 outside allowed range"},
		{name: "oversized", input: "Content-Length: 16777217\r\n\r\n", err: "DAP content length 16777217 outside allowed range"},
		{name: "bad json", input: "Content-Length: 1\r\n\r\n{", err: "decode DAP message"},
		{name: "truncated body", input: "Content-Length: 10\r\n\r\n{}", err: io.ErrUnexpectedEOF.Error()},
		{name: "truncated header", input: "Content-Length: 2", err: io.EOF.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := readFrame(bufio.NewReader(strings.NewReader(test.input)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestWriteFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	sent := message{Seq: 7, Type: "request", Command: "evaluate", Arguments: json.RawMessage(`{"expression":"名前"}`)}
	require.NoError(t, writeFrame(&buffer, sent))
	assert.True(t, strings.HasPrefix(buffer.String(), "Content-Length: "))
	got, err := readFrame(bufio.NewReader(&buffer))
	require.NoError(t, err)
	assert.Equal(t, sent, got)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestWriteFrameErrors(t *testing.T) {
	require.ErrorContains(t, writeFrame(failingWriter{}, message{Type: "request"}), "write DAP message: broken pipe")
	huge := message{Type: "request", Arguments: json.RawMessage(`"` + strings.Repeat("x", maxMessageBytes) + `"`)}
	require.EqualError(t, writeFrame(io.Discard, huge), "DAP message exceeds size limit")
	require.ErrorContains(t, writeFrame(io.Discard, message{Arguments: json.RawMessage(`{`)}), "encode DAP message")
}

func TestFailureText(t *testing.T) {
	for _, test := range []struct {
		name string
		item message
		want string
	}{
		{name: "error format", item: message{Message: "Failed", Body: json.RawMessage(`{"error":{"format":"Failed to launch: detail"}}`)}, want: "Failed to launch: detail"},
		{name: "message", item: message{Message: "No debug session started"}, want: "No debug session started"},
		{name: "fallback", item: message{Body: json.RawMessage(`[]`)}, want: "debug adapter request failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.item.failure())
		})
	}
}

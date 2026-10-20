package dap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFrameNamesProtocol(t *testing.T) {
	got, err := readFrame(bufio.NewReader(strings.NewReader("garbage\r\n\r\n")))
	require.EqualError(t, err, `invalid DAP header "garbage\r\n"`)
	assert.Equal(t, message{}, got)
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

func TestWriteFrameKeepsFailedSuccess(t *testing.T) {
	var buffer bytes.Buffer
	require.NoError(t, writeFrame(&buffer, message{Seq: 2, Type: "response", RequestSeq: 1, Command: "runInTerminal"}))
	assert.Contains(t, buffer.String(), `"success":false`)
}

func TestWriteFrameNamesProtocol(t *testing.T) {
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

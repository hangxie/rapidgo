package frame

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type item struct {
	Seq  int    `json:"seq"`
	Type string `json:"type,omitempty"`
}

func TestRead(t *testing.T) {
	for _, test := range []struct {
		name, input, err string
		want             item
	}{
		{name: "message", input: "Content-Length: 22\r\n\r\n{\"seq\":3,\"type\":\"evt\"}", want: item{Seq: 3, Type: "evt"}},
		{name: "extra header and lowercase key", input: "Content-Type: x\r\ncontent-length: 9\r\n\r\n{\"seq\":1}", want: item{Seq: 1}},
		{name: "bare newlines", input: "Content-Length: 2\n\n{}", want: item{}},
		{name: "missing colon", input: "garbage\r\n\r\n", err: `invalid DAP header "garbage\r\n"`},
		{name: "bad length", input: "Content-Length: x\r\n\r\n", err: "invalid DAP content length"},
		{name: "negative length", input: "Content-Length: -1\r\n\r\n", err: "DAP content length -1 outside allowed range"},
		{name: "missing length", input: "X-Other: value\r\n\r\n", err: "DAP content length -1 outside allowed range"},
		{name: "oversized", input: "Content-Length: 16777217\r\n\r\n", err: "DAP content length 16777217 outside allowed range"},
		{name: "bad json", input: "Content-Length: 1\r\n\r\n{", err: "decode DAP message"},
		{name: "truncated body", input: "Content-Length: 10\r\n\r\n{}", err: io.ErrUnexpectedEOF.Error()},
		{name: "truncated header", input: "Content-Length: 2", err: io.EOF.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got item
			err := Read(bufio.NewReader(strings.NewReader(test.input)), "DAP", &got)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestReadKeepsEOF(t *testing.T) {
	var got item
	require.ErrorIs(t, Read(bufio.NewReader(strings.NewReader("")), "LSP", &got), io.EOF)
}

// countingWriter records how many writes carried the frame.
type countingWriter struct {
	bytes.Buffer
	writes int
}

func (w *countingWriter) Write(data []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(data)
}

func TestWriteRoundTripInOneWrite(t *testing.T) {
	var buffer countingWriter
	require.NoError(t, Write(&buffer, "LSP", map[string]string{"text": "名前"}))
	assert.Equal(t, 1, buffer.writes)
	assert.Equal(t, "Content-Length: 17\r\n\r\n{\"text\":\"名前\"}", buffer.String())
	var got map[string]string
	require.NoError(t, Read(bufio.NewReader(&buffer.Buffer), "LSP", &got))
	assert.Equal(t, map[string]string{"text": "名前"}, got)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// shortWriter accepts one byte less than it is given without reporting an error.
type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestWriteErrors(t *testing.T) {
	require.EqualError(t, Write(shortWriter{}, "DAP", item{}), "write DAP message: short write")
	require.EqualError(t, Write(failingWriter{}, "LSP", item{}), "write LSP message: broken pipe")
	require.EqualError(t, Write(io.Discard, "DAP", strings.Repeat("x", maxBytes)), "DAP message exceeds size limit")
	require.ErrorContains(t, Write(io.Discard, "LSP", make(chan int)), "encode LSP message")
}

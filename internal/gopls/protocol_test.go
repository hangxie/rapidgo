package gopls

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileURIRoundTrip(t *testing.T) {
	path := t.TempDir() + "/two words/日本語.go"
	decoded, err := pathFromURI(fileURI(path))
	require.NoError(t, err)
	require.Equal(t, path, decoded)
	_, err = pathFromURI("https://example.com/code.go")
	require.Error(t, err)
}

func TestReadFrameRejectsInvalidSize(t *testing.T) {
	for _, input := range []string{"Content-Length: -1\r\n\r\n", "Content-Length: 16777217\r\n\r\n", "Content-Length: nope\r\n\r\n"} {
		_, err := readFrame(bufio.NewReader(strings.NewReader(input)))
		require.Error(t, err)
	}
}

func TestReadFrameRejectsBrokenMessages(t *testing.T) {
	for _, input := range []string{
		"Bad Header\r\n\r\n",
		"X-Other: value\r\n\r\n",
		"Content-Length: 4\r\n\r\n{}",
		"Content-Length: 1\r\n\r\nx",
	} {
		_, err := readFrame(bufio.NewReader(strings.NewReader(input)))
		require.Error(t, err, input)
	}
	_, err := readFrame(bufio.NewReader(strings.NewReader("")))
	require.ErrorIs(t, err, io.EOF)
}

func TestWriteFrameReportsSerializationAndTransportFailures(t *testing.T) {
	require.ErrorContains(t, writeFrame(io.Discard, make(chan int)), "encode LSP message")
	require.ErrorContains(t, writeFrame(&failingWriter{failOn: 1}, map[string]string{"ok": "yes"}), "write LSP header")
	require.ErrorContains(t, writeFrame(&failingWriter{failOn: 2}, map[string]string{"ok": "yes"}), "write LSP body")
}

type failingWriter struct {
	failOn int
	writes int
}

func (writer *failingWriter) Write(value []byte) (int, error) {
	writer.writes++
	if writer.writes == writer.failOn {
		return 0, errors.New("transport failed")
	}
	return len(value), nil
}

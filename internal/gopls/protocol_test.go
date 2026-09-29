package gopls

import (
	"bufio"
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

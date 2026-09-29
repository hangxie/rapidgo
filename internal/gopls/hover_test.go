package gopls

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHoverContents(t *testing.T) {
	for _, test := range []struct {
		input, expected string
	}{
		{`null`, ""},
		{`"symbol"`, "symbol"},
		{`{"kind":"plaintext","value":"func main()"}`, "func main()"},
		{`["func main()",{"language":"go","value":"package main"}]`, "func main()\npackage main"},
	} {
		actual, err := hoverContents(json.RawMessage(test.input))
		require.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}
	_, err := hoverContents(json.RawMessage(`42`))
	require.Error(t, err)
}

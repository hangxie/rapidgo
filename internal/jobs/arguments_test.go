package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseArguments(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, input, errorText string
		want                   []string
	}{
		{"empty", " \t", "", nil},
		{"plain", "--count 3", "", []string{"--count", "3"}},
		{"quotes", `--name "two words" '日本 語'`, "", []string{"--name", "two words", "日本 語"}},
		{"empty argument", `"" ''`, "", []string{"", ""}},
		{"adjacent quotes", `pre"fix space"post`, "", []string{"prefix spacepost"}},
		{"escaped space", `one\ two`, "", []string{"one two"}},
		{"single quote literal", `'a\b'`, "", []string{`a\b`}},
		{"unclosed quote", `"oops`, "unclosed quote", nil},
		{"trailing escape", `oops\`, "trailing backslash", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseArguments(test.input)
			assert.Equal(t, test.want, got)
			if test.errorText == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, test.errorText)
			}
		})
	}
}

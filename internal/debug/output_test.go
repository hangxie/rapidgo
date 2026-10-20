package debug

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func texts(events []Event) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.Category+":"+event.Text)
	}
	return result
}

func TestOutputLines(t *testing.T) {
	t.Parallel()

	var lines outputLines
	assert.Empty(t, lines.add("stdout", "par"))
	assert.Equal(t, []string{"stdout:partial", "stdout:"}, texts(lines.add("stdout", "tial\r\n\n")))
	assert.Empty(t, lines.add("stderr", "warn"))
	assert.Equal(t, []string{"stdout:next"}, texts(lines.add("stdout", "next\nend")))
	assert.Equal(t, []string{"stderr:warn", "stdout:end"}, texts(lines.flush()))
	assert.Empty(t, lines.flush())
}

func TestOutputLinesBoundLength(t *testing.T) {
	t.Parallel()

	var lines outputLines
	wide := strings.Repeat("x", maxLineBytes-1) + "世界"
	got := lines.add("stdout", wide+"\n")
	assert.Equal(t, []string{"stdout:" + strings.Repeat("x", maxLineBytes-1), "stdout:世界"}, texts(got))

	partial := lines.add("stdout", strings.Repeat("y", maxLineBytes+5))
	assert.Equal(t, []string{"stdout:" + strings.Repeat("y", maxLineBytes)}, texts(partial), "an unterminated line must not grow without bound")
	assert.Equal(t, []string{"stdout:yyyyy"}, texts(lines.flush()))

	invalid := strings.Repeat("\x80", maxLineBytes+1)
	for _, event := range lines.add("stdout", invalid+"\n") {
		assert.LessOrEqual(t, len(event.Text), maxLineBytes)
	}
	assert.False(t, utf8.ValidString(invalid))
}

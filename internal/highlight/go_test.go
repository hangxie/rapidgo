package highlight

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoClassifiesTokens(t *testing.T) {
	t.Parallel()
	source := "package main\n// 界 comment\nvar name = `raw\ntext`\nvar number = 42i\nvar letter = '界'\n"
	var got []struct {
		text string
		kind Kind
	}
	for _, span := range Go(source) {
		got = append(got, struct {
			text string
			kind Kind
		}{source[span.Start:span.End], span.Kind})
	}
	assert.Equal(t, []struct {
		text string
		kind Kind
	}{
		{"package", Keyword},
		{"// 界 comment", Comment},
		{"var", Keyword},
		{"`raw\ntext`", String},
		{"var", Keyword},
		{"42i", Number},
		{"var", Keyword},
		{"'界'", String},
	}, got)
}

func TestGoKeepsHighlightingIncompleteSource(t *testing.T) {
	t.Parallel()
	source := "var x = \"unfinished\nfunc main() {}\n"
	spans := Go(source)
	assert.NotEmpty(t, spans)
	assert.Equal(t, "var", source[spans[0].Start:spans[0].End])
	assert.Equal(t, Keyword, spans[0].Kind)
	assert.LessOrEqual(t, len(spans), len(source))
}

func TestGoSpansAreOrderedAndWithinSource(t *testing.T) {
	t.Parallel()
	source := "package p\n" + strings.Repeat("var x = 1 // hi\n", 10)
	previous := 0
	for _, span := range Go(source) {
		assert.GreaterOrEqual(t, span.Start, previous)
		assert.Greater(t, span.End, span.Start)
		assert.LessOrEqual(t, span.End, len(source))
		previous = span.End
	}
}

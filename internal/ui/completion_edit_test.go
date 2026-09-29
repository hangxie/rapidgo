package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

func TestCompletionPositionsRespectGraphemeBoundaries(t *testing.T) {
	line := "a𐐀e\u0301b"
	for _, test := range []struct {
		utf16 int
		byte  int
		col   int
	}{
		{0, 0, 0}, {1, 1, 1}, {3, 5, 2}, {5, 8, 3}, {6, 9, 4},
	} {
		position, offset, err := completionPosition([]string{line}, gopls.Position{Character: test.utf16})
		require.NoError(t, err)
		assert.Equal(t, test.byte, offset)
		assert.Equal(t, editor.Position{Column: test.col}, position)
	}
	_, _, err := completionPosition([]string{line}, gopls.Position{Character: 2})
	require.ErrorContains(t, err, "splits a grapheme")
	_, _, err = completionPosition([]string{line}, gopls.Position{Character: 4})
	require.ErrorContains(t, err, "splits a grapheme")
}

func TestCompletionFallbackStopsAtPunctuationAndCaret(t *testing.T) {
	for _, test := range []struct {
		name, input, insert, want string
		caret                     editor.Position
		result                    editor.Position
	}{
		{name: "selector", input: "fmt.", insert: "Println", want: "fmt.Println", caret: editor.Position{Column: 4}, result: editor.Position{Column: 11}},
		{name: "inside identifier", input: "print", insert: "println", want: "println", caret: editor.Position{Column: 3}, result: editor.Position{Column: 7}},
		{name: "inside selector", input: "fmt.Prntln", insert: "Println", want: "fmt.Println", caret: editor.Position{Column: 6}, result: editor.Position{Column: 11}},
		{name: "inside unicode identifier", input: "变量名", insert: "变量值", want: "变量值", caret: editor.Position{Column: 1}, result: editor.Position{Column: 3}},
		{name: "identifier after digits", input: "123abc", insert: "xyz", want: "123xyz", caret: editor.Position{Column: 4}, result: editor.Position{Column: 6}},
		{name: "at identifier start", input: "print", insert: "println", want: "println", caret: editor.Position{}, result: editor.Position{Column: 7}},
		{name: "middle of line", input: "pri tail", insert: "println", want: "println tail", caret: editor.Position{Column: 3}, result: editor.Position{Column: 7}},
		{name: "previous line", input: "package main\npri", insert: "println", want: "package main\nprintln", caret: editor.Position{Line: 1, Column: 3}, result: editor.Position{Line: 1, Column: 7}},
	} {
		t.Run(test.name, func(t *testing.T) {
			buffer, err := editor.New(test.input)
			require.NoError(t, err)
			require.NoError(t, applyCompletionItem(buffer, gopls.CompletionItem{Label: test.insert, InsertText: test.insert}, test.caret))
			assert.Equal(t, test.want, buffer.Text())
			assert.Equal(t, test.result, buffer.Cursor())
			assert.True(t, buffer.Undo())
			assert.Equal(t, test.input, buffer.Text())
		})
	}
}

func TestCompletionEditRejectsMalformedAdditionalRange(t *testing.T) {
	buffer, err := editor.New("foo")
	require.NoError(t, err)
	item := gopls.CompletionItem{Label: "foobar", AdditionalTextEdits: []gopls.TextEdit{{
		Range:   gopls.Range{Start: gopls.Position{Character: 0}, End: gopls.Position{Character: 8}},
		NewText: "import \"fmt\"\n",
	}}}
	require.ErrorIs(t, applyCompletionItem(buffer, item, editor.Position{Column: 3}), editor.ErrPosition)
	assert.Equal(t, "foo", buffer.Text())
}

func TestCompletionEditRejectsInvalidCaret(t *testing.T) {
	buffer, err := editor.New("foo")
	require.NoError(t, err)
	item := gopls.CompletionItem{Label: "foobar", TextEdit: &gopls.TextEdit{
		Range: gopls.Range{Start: gopls.Position{}, End: gopls.Position{Character: 3}}, NewText: "foobar",
	}}
	require.ErrorIs(t, applyCompletionItem(buffer, item, editor.Position{Column: 4}), editor.ErrPosition)
	assert.Equal(t, "foo", buffer.Text())
}

func TestCompletionFallbackRejectsSplitGrapheme(t *testing.T) {
	buffer, err := editor.New("a\u0301")
	require.NoError(t, err)
	err = applyCompletionItem(buffer, gopls.CompletionItem{Label: "abc"}, editor.Position{})
	require.ErrorContains(t, err, "splits a grapheme")
	assert.Equal(t, "a\u0301", buffer.Text())
}

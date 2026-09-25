package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNextWrapsAndSkipsCurrentSelection(t *testing.T) {
	t.Parallel()
	buffer, err := New("one two one")
	require.NoError(t, err)
	match, ok := buffer.FindNext("one")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{}, End: Position{Column: 3}}, match)
	require.NoError(t, buffer.Select(match.Start, match.End))
	match, ok = buffer.FindNext("one")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{Column: 8}, End: Position{Column: 11}}, match)
	require.NoError(t, buffer.Select(match.Start, match.End))
	match, ok = buffer.FindNext("one")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{}, End: Position{Column: 3}, Wrapped: true}, match)
}

func TestFindNextUsesGraphemeBoundaries(t *testing.T) {
	t.Parallel()
	buffer, err := New("e\u0301 e 👩‍💻\n界")
	require.NoError(t, err)
	_, ok := buffer.FindNext("\u0301")
	assert.False(t, ok, "a combining mark inside a grapheme is not selectable")
	match, ok := buffer.FindNext("e")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{Column: 2}, End: Position{Column: 3}}, match)
	match, ok = buffer.FindNext("👩‍💻")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{Column: 4}, End: Position{Column: 5}}, match)
	match, ok = buffer.FindNext("界")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{Line: 1}, End: Position{Line: 1, Column: 1}}, match)
	_, ok = buffer.FindNext("")
	assert.False(t, ok)
}

func TestFindNextDoesNotSplitClusterAtEnd(t *testing.T) {
	t.Parallel()
	buffer, err := New("e\u0301")
	require.NoError(t, err)
	_, ok := buffer.FindNext("e")
	assert.False(t, ok)
	match, ok := buffer.FindNext("e\u0301")
	require.True(t, ok)
	assert.Equal(t, Match{Start: Position{}, End: Position{Column: 1}}, match)
}

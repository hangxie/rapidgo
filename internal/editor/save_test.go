package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySavedTextIsUndoableAndKeepsCaret(t *testing.T) {
	t.Parallel()
	buffer, err := New("package main\nfunc main(){ }\n")
	require.NoError(t, err)
	require.NoError(t, buffer.MoveTo(Position{Line: 1, Column: 5}, false))
	require.NoError(t, buffer.ApplySavedText("package main\nfunc main() {}\n"))
	assert.Equal(t, Position{Line: 1, Column: 5}, buffer.Cursor())
	assert.False(t, buffer.Dirty())
	assert.True(t, buffer.Undo())
	assert.Equal(t, "package main\nfunc main(){ }\n", buffer.Text())
	assert.True(t, buffer.Dirty())
	assert.True(t, buffer.Redo())
	assert.Equal(t, "package main\nfunc main() {}\n", buffer.Text())
	assert.False(t, buffer.Dirty())
}

func TestApplySavedTextUpdatesLineEndingStyle(t *testing.T) {
	t.Parallel()
	buffer, err := New("a\r\nb\r\n")
	require.NoError(t, err)
	require.NoError(t, buffer.ApplySavedText("a\nb\n"))
	assert.Equal(t, "a\nb\n", buffer.SerializedText())
	assert.False(t, buffer.Dirty())
}

func TestSavedRevisionTracksAsyncCheckpoint(t *testing.T) {
	t.Parallel()
	buffer, err := New("a")
	require.NoError(t, err)
	require.NoError(t, buffer.Insert("b"))
	revision := buffer.Revision()
	require.NoError(t, buffer.Insert("c"))
	buffer.MarkSavedRevision(revision)
	assert.True(t, buffer.Dirty())
	assert.True(t, buffer.Undo())
	assert.False(t, buffer.Dirty())
}

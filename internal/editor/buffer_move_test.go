package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositionsUseGraphemeClusters(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\u0301界👩‍💻\nZ")
	for _, want := range []Position{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {1, 1}} {
		buffer.MoveRight(false)
		assert.Equal(t, want, buffer.Cursor())
	}
	buffer.MoveRight(false)
	assert.Equal(t, Position{1, 1}, buffer.Cursor())
	for _, want := range []Position{{1, 0}, {0, 3}, {0, 2}, {0, 1}, {0, 0}} {
		buffer.MoveLeft(false)
		assert.Equal(t, want, buffer.Cursor())
	}
	buffer.MoveLeft(false)
	assert.Equal(t, Position{0, 0}, buffer.Cursor())
}

func TestGraphemeMovementAndDeletion(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		text string
	}{
		{name: "combining", text: "e\u0301"},
		{name: "wide", text: "界"},
		{name: "emoji modifier", text: "👍🏽"},
		{name: "emoji joiner", text: "👩‍💻"},
		{name: "flag", text: "🇺🇸"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			buffer := newBuffer(t, test.text+"X")
			buffer.MoveRight(false)
			assert.Equal(t, Position{0, 1}, buffer.Cursor())
			buffer.MoveLeft(false)
			assert.Equal(t, Position{0, 0}, buffer.Cursor())
			require.True(t, buffer.DeleteForward())
			assert.Equal(t, "X", buffer.Text())
			require.True(t, buffer.Undo())
			assert.Equal(t, test.text+"X", buffer.Text())
		})
	}
}

func TestPositionValidationDoesNotMoveCaret(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "界\n")
	for _, position := range []Position{{-1, 0}, {0, -1}, {0, 2}, {2, 0}} {
		assert.ErrorIs(t, buffer.MoveTo(position, false), ErrPosition)
		assert.Equal(t, Position{}, buffer.Cursor())
	}
	require.NoError(t, buffer.MoveTo(Position{1, 0}, false))
	assert.Equal(t, Position{1, 0}, buffer.Cursor())
	assert.ErrorIs(t, buffer.Select(Position{0, 0}, Position{1, 1}), ErrPosition)
	assert.Equal(t, Position{1, 0}, buffer.Cursor())
}

func TestVerticalMovesKeepPreferredColumn(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "abcd\nx\nwxyz")
	require.NoError(t, buffer.MoveTo(Position{0, 3}, false))
	buffer.MoveDown(false)
	assert.Equal(t, Position{1, 1}, buffer.Cursor())
	buffer.MoveDown(false)
	assert.Equal(t, Position{2, 3}, buffer.Cursor())
	buffer.MoveUp(false)
	buffer.MoveLeft(false)
	buffer.MoveDown(false)
	assert.Equal(t, Position{2, 0}, buffer.Cursor(), "horizontal motion resets the preferred column")
	buffer.MoveHome(false)
	buffer.MoveEnd(false)
	assert.Equal(t, Position{2, 4}, buffer.Cursor())
}

func TestExtendedMovementAndSelectionCollapse(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "abc")
	buffer.MoveRight(true)
	buffer.MoveRight(true)
	start, end, selected := buffer.Selection()
	assert.True(t, selected)
	assert.Equal(t, Position{0, 0}, start)
	assert.Equal(t, Position{0, 2}, end)
	buffer.MoveLeft(false)
	assert.Equal(t, Position{0, 0}, buffer.Cursor())
	assert.False(t, buffer.HasSelection())
	require.NoError(t, buffer.Select(Position{0, 0}, Position{0, 2}))
	buffer.MoveRight(false)
	assert.Equal(t, Position{0, 2}, buffer.Cursor())
	assert.False(t, buffer.HasSelection())
}

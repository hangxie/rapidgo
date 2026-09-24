package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBuffer(t *testing.T, text string) *Buffer {
	t.Helper()
	buffer, err := New(text)
	require.NoError(t, err)
	return buffer
}

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

func TestSelectionAndReplacement(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "one\ntwo\nthree")
	require.NoError(t, buffer.Select(Position{1, 2}, Position{0, 1}))
	start, end, selected := buffer.Selection()
	assert.True(t, selected)
	assert.Equal(t, Position{0, 1}, start)
	assert.Equal(t, Position{1, 2}, end)
	require.NoError(t, buffer.Insert("X\nY"))
	assert.Equal(t, "oX\nYo\nthree", buffer.Text())
	assert.Equal(t, Position{1, 1}, buffer.Cursor())
	assert.False(t, buffer.HasSelection())
	assert.True(t, buffer.Dirty())
	require.True(t, buffer.Undo())
	assert.Equal(t, "one\ntwo\nthree", buffer.Text())
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
	assert.Equal(t, Position{1, 2}, buffer.Anchor())
	require.True(t, buffer.Redo())
	assert.Equal(t, "oX\nYo\nthree", buffer.Text())
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

func TestDeletionAcrossGraphemesAndLines(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\u0301界\nZ")
	require.NoError(t, buffer.MoveTo(Position{0, 2}, false))
	require.True(t, buffer.DeleteBackward())
	assert.Equal(t, "a\u0301\nZ", buffer.Text())
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
	require.True(t, buffer.DeleteForward())
	assert.Equal(t, "a\u0301Z", buffer.Text())
	require.True(t, buffer.DeleteBackward())
	assert.Equal(t, "Z", buffer.Text())
	assert.Equal(t, Position{0, 0}, buffer.Cursor())
	assert.False(t, buffer.DeleteBackward())
	buffer.MoveEnd(false)
	assert.False(t, buffer.DeleteForward())
}

func TestUndoRedoAndSaveCheckpoint(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a")
	assert.False(t, buffer.Dirty())
	assert.False(t, buffer.Undo())
	require.NoError(t, buffer.MoveTo(Position{0, 1}, false))
	require.NoError(t, buffer.Insert("b"))
	buffer.MarkSaved()
	require.NoError(t, buffer.Insert("c"))
	assert.True(t, buffer.Dirty())
	require.True(t, buffer.Undo())
	assert.Equal(t, "ab", buffer.Text())
	assert.False(t, buffer.Dirty())
	require.True(t, buffer.Undo())
	assert.True(t, buffer.Dirty())
	require.True(t, buffer.Redo())
	assert.False(t, buffer.Dirty())
	require.NoError(t, buffer.Insert("X"))
	assert.Equal(t, "abX", buffer.Text())
	assert.False(t, buffer.CanRedo(), "new edits discard the redo branch")
	assert.True(t, buffer.Dirty())
}

func TestInvalidInsertLeavesStateUnchanged(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "abc")
	require.NoError(t, buffer.Select(Position{0, 0}, Position{0, 2}))
	assert.ErrorIs(t, buffer.Insert(string([]byte{0xff})), ErrInvalidUTF8)
	assert.Equal(t, "abc", buffer.Text())
	assert.True(t, buffer.HasSelection())
	assert.False(t, buffer.CanUndo())
	assert.False(t, buffer.Dirty())
	_, err := New(string([]byte{0xff}))
	assert.ErrorIs(t, err, ErrInvalidUTF8)
}

func TestInsertCombiningMarkKeepsCaretOnBoundary(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "aX")
	require.NoError(t, buffer.MoveTo(Position{0, 1}, false))
	require.NoError(t, buffer.Insert("\u0301"))
	assert.Equal(t, "a\u0301X", buffer.Text())
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
	buffer.MoveRight(false)
	assert.Equal(t, Position{0, 2}, buffer.Cursor())
	require.True(t, buffer.Undo())
	assert.Equal(t, "aX", buffer.Text())
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
}

func TestLinesAreDetachedAndPreserveFinalNewline(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\n")
	lines := buffer.Lines()
	assert.Equal(t, []string{"a", ""}, lines)
	lines[0] = "other"
	assert.Equal(t, "a\n", buffer.Text())
}

func TestCRLFIsOneLogicalNewline(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\r\nb")
	assert.Equal(t, "a\nb", buffer.Text())
	assert.Equal(t, "a\r\nb", buffer.SerializedText())
	assert.Equal(t, []string{"a", "b"}, buffer.Lines())
	buffer.MoveEnd(false)
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
	buffer.MoveDown(false)
	assert.Equal(t, Position{1, 1}, buffer.Cursor())
	buffer.MoveUp(false)
	assert.Equal(t, Position{0, 1}, buffer.Cursor())
	assert.False(t, buffer.Dirty(), "normalizing input is not an edit")
}

func TestCRLFDeletionAndUndoRedo(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		delete func(*Buffer) bool
		start  Position
	}{
		{name: "backspace", start: Position{1, 0}, delete: (*Buffer).DeleteBackward},
		{name: "delete", start: Position{0, 1}, delete: (*Buffer).DeleteForward},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			buffer := newBuffer(t, "a\r\nb")
			require.NoError(t, buffer.MoveTo(test.start, false))
			require.True(t, test.delete(buffer))
			assert.Equal(t, "ab", buffer.Text())
			assert.Equal(t, "ab", buffer.SerializedText())
			require.True(t, buffer.Undo())
			assert.Equal(t, "a\r\nb", buffer.SerializedText())
			assert.Equal(t, test.start, buffer.Cursor())
			require.True(t, buffer.Redo())
			assert.Equal(t, "ab", buffer.SerializedText())
		})
	}
}

func TestNewlinesInsertedIntoCRLFBufferUseCRLFOnSerialization(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\r\nb")
	require.NoError(t, buffer.MoveTo(Position{0, 1}, false))
	require.NoError(t, buffer.Insert("\n"))
	assert.Equal(t, "a\n\nb", buffer.Text())
	assert.Equal(t, "a\r\n\r\nb", buffer.SerializedText())
	assert.Equal(t, Position{1, 0}, buffer.Cursor())
	require.True(t, buffer.Undo())
	assert.Equal(t, "a\r\nb", buffer.SerializedText())
	require.True(t, buffer.Redo())
	assert.Equal(t, "a\r\n\r\nb", buffer.SerializedText())
	require.NoError(t, buffer.Insert("x\r\ny"))
	assert.Equal(t, "a\nx\ny\nb", buffer.Text())
	assert.Equal(t, "a\r\nx\r\ny\r\nb", buffer.SerializedText())
}

func TestLFAndMixedLineEndingPolicy(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		input      string
		serialized string
	}{
		{name: "LF", input: "a\nb", serialized: "a\nb"},
		{name: "first LF", input: "a\nb\r\nc", serialized: "a\nb\nc"},
		{name: "first CRLF", input: "a\r\nb\nc", serialized: "a\r\nb\r\nc"},
		{name: "bare CR", input: "a\rb", serialized: "a\rb"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			buffer := newBuffer(t, test.input)
			assert.Equal(t, test.serialized, buffer.SerializedText())
			assert.False(t, buffer.Dirty())
		})
	}
}

func TestBareCRRemainsEditableAcrossSpliceBoundary(t *testing.T) {
	t.Parallel()

	t.Run("insert CR before LF", func(t *testing.T) {
		t.Parallel()
		buffer := newBuffer(t, "a\nb")
		require.NoError(t, buffer.MoveTo(Position{0, 1}, false))
		require.NoError(t, buffer.Insert("\r"))
		assert.Equal(t, "a\r\nb", buffer.Text())
		assert.Equal(t, "a\r\r\nb", buffer.SerializedText())
		assert.True(t, buffer.Dirty())
		reloaded := newBuffer(t, buffer.SerializedText())
		assert.Equal(t, buffer.Text(), reloaded.Text())
		require.True(t, buffer.Undo())
		assert.Equal(t, "a\nb", buffer.Text())
		require.True(t, buffer.Redo())
		assert.Equal(t, "a\r\nb", buffer.Text())
	})

	t.Run("insert LF after bare CR", func(t *testing.T) {
		t.Parallel()
		buffer := newBuffer(t, "a\rb")
		require.NoError(t, buffer.MoveTo(Position{0, 2}, false))
		require.NoError(t, buffer.Insert("\n"))
		assert.Equal(t, "a\r\nb", buffer.Text())
		assert.Equal(t, "a\r\r\nb", buffer.SerializedText())
		reloaded := newBuffer(t, buffer.SerializedText())
		assert.Equal(t, buffer.Text(), reloaded.Text())
		require.True(t, buffer.Undo())
		assert.Equal(t, "a\rb", buffer.Text())
		require.True(t, buffer.Redo())
		assert.Equal(t, "a\r\nb", buffer.Text())
	})

	t.Run("delete between CR and LF", func(t *testing.T) {
		t.Parallel()
		buffer := newBuffer(t, "a\rX\nb")
		require.NoError(t, buffer.MoveTo(Position{0, 3}, false))
		require.True(t, buffer.DeleteBackward())
		assert.Equal(t, "a\r\nb", buffer.Text())
		assert.Equal(t, "a\r\r\nb", buffer.SerializedText())
		require.True(t, buffer.Undo())
		assert.Equal(t, "a\rX\nb", buffer.Text())
		require.True(t, buffer.Redo())
		assert.Equal(t, "a\r\nb", buffer.Text())
	})
}

func TestRepeatedCRBeforeLFPreservesBareCR(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\r\r\nb")
	assert.Equal(t, "a\r\nb", buffer.Text())
	assert.Equal(t, "a\r\r\nb", buffer.SerializedText())
	buffer.MoveEnd(false)
	assert.Equal(t, Position{0, 2}, buffer.Cursor())
	require.True(t, buffer.DeleteBackward())
	assert.Equal(t, "a\nb", buffer.Text())
	require.True(t, buffer.Undo())
	assert.Equal(t, "a\r\r\nb", buffer.SerializedText())
}

func TestSaveReopenSaveKeepsLFStyleAfterEscapedBareCR(t *testing.T) {
	t.Parallel()

	buffer := newBuffer(t, "a\nb\n")
	require.NoError(t, buffer.MoveTo(Position{Line: 0, Column: 1}, false))
	require.NoError(t, buffer.Insert("\r"))
	first := buffer.SerializedText()
	assert.Equal(t, "a\r\r\nb\n", first)
	reloaded := newBuffer(t, first)
	assert.Equal(t, buffer.Text(), reloaded.Text())
	assert.Equal(t, first, reloaded.SerializedText())
}

func TestLineEndingDetectionSkipsAmbiguousBreaks(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		input string
		style string
	}{
		{name: "LF after ambiguous", input: "a\r\r\nb\n", style: "\n"},
		{name: "CRLF after ambiguous", input: "a\r\r\nb\r\n", style: "\r\n"},
		{name: "all ambiguous", input: "a\r\r\n", style: "\r\n"},
		{name: "no newline", input: "a\r", style: "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			buffer := newBuffer(t, test.input)
			assert.Equal(t, test.style, buffer.lineEnding)
			assert.Equal(t, test.input, buffer.SerializedText())
		})
	}
}

func TestSerializationRoundTripAcrossSmallEdits(t *testing.T) {
	t.Parallel()

	inputs := []string{""}
	for range 3 {
		previous := append([]string(nil), inputs...)
		for _, input := range previous {
			for _, char := range []string{"a", "\r", "\n"} {
				inputs = append(inputs, input+char)
			}
		}
	}
	for _, input := range inputs {
		original := newBuffer(t, input)
		for line, text := range original.Lines() {
			for column := range graphemeCount(text) + 1 {
				position := Position{Line: line, Column: column}
				for _, inserted := range []string{"\r", "\n", "\r\n", "\r\r\n", "x"} {
					buffer := newBuffer(t, input)
					require.NoError(t, buffer.MoveTo(position, false))
					require.NoError(t, buffer.Insert(inserted))
					assertSerializedRoundTrip(t, buffer, input, position, "insert "+inserted)
				}
				for _, operation := range []struct {
					name   string
					delete func(*Buffer) bool
				}{
					{name: "backspace", delete: (*Buffer).DeleteBackward},
					{name: "delete", delete: (*Buffer).DeleteForward},
				} {
					buffer := newBuffer(t, input)
					require.NoError(t, buffer.MoveTo(position, false))
					operation.delete(buffer)
					assertSerializedRoundTrip(t, buffer, input, position, operation.name)
				}
			}
		}
	}
}

func assertSerializedRoundTrip(t *testing.T, buffer *Buffer, input string, position Position, operation string) {
	t.Helper()
	reloaded := newBuffer(t, buffer.SerializedText())
	assert.Equal(t, buffer.Text(), reloaded.Text(), "input %q, position %+v, %s", input, position, operation)
	assert.Equal(t, buffer.SerializedText(), reloaded.SerializedText(), "input %q, position %+v, %s", input, position, operation)
	if buffer.CanUndo() {
		modified := buffer.Text()
		require.True(t, buffer.Undo())
		assert.Equal(t, newBuffer(t, input).Text(), buffer.Text())
		require.True(t, buffer.Redo())
		assert.Equal(t, modified, buffer.Text())
	}
}

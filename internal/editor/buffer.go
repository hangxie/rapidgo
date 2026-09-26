// Package editor owns text and editing state independently of the terminal UI.
package editor

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

var (
	ErrInvalidUTF8 = errors.New("editor text is not valid UTF-8")
	ErrPosition    = errors.New("editor position is out of range")
)

// Position uses zero-based lines and grapheme-cluster columns.
type Position struct {
	Line   int
	Column int
}

type view struct {
	cursor int // UTF-8 byte offset, always at a grapheme boundary
	anchor int
	goal   int // preferred grapheme column for consecutive vertical moves
}

type edit struct {
	start, beforeID, afterID int
	removed, inserted        string
	before, after            view
}

// Buffer stores UTF-8 text with LF as the logical line break.
type Buffer struct {
	text              string
	lineEnding        string
	view              view
	undo, redo        []edit
	currentID, nextID int
	savedID           int
}

func New(text string) (*Buffer, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}
	return &Buffer{
		text:       strings.ReplaceAll(text, "\r\n", "\n"),
		lineEnding: detectLineEnding(text),
		view:       view{goal: -1},
	}, nil
}

// Only an unambiguous break names the file's style; CRLF is the fallback.
func detectLineEnding(text string) string {
	ambiguous := false
	for index := 0; index < len(text); index++ {
		if text[index] != '\n' {
			continue
		}
		if index == 0 || text[index-1] != '\r' {
			return "\n"
		}
		if index > 1 && text[index-2] == '\r' {
			ambiguous = true
			continue
		}
		return "\r\n"
	}
	if ambiguous {
		return "\r\n"
	}
	return "\n"
}

// Text returns editable text: LF is a line break and CR is a character.
func (b *Buffer) Text() string { return b.text }

// SerializedText restores the detected line-ending style for writing.
func (b *Buffer) SerializedText() string {
	var serialized strings.Builder
	serialized.Grow(len(b.text) + strings.Count(b.text, "\n"))
	for index := 0; index < len(b.text); index++ {
		if b.text[index] != '\n' {
			serialized.WriteByte(b.text[index])
			continue
		}
		if index > 0 && b.text[index-1] == '\r' {
			serialized.WriteString("\r\n")
		} else {
			serialized.WriteString(b.lineEnding)
		}
	}
	return serialized.String()
}

// Lines returns a detached snapshot, including an empty final line after LF.
func (b *Buffer) Lines() []string { return strings.Split(b.text, "\n") }

func (b *Buffer) Cursor() Position { return positionAt(b.text, b.view.cursor) }

func (b *Buffer) Anchor() Position { return positionAt(b.text, b.view.anchor) }

func (b *Buffer) HasSelection() bool { return b.view.cursor != b.view.anchor }

func (b *Buffer) Selection() (Position, Position, bool) {
	start, end := b.selectionOffsets()
	return positionAt(b.text, start), positionAt(b.text, end), start != end
}

// MoveTo positions the caret, optionally extending the current selection.
func (b *Buffer) MoveTo(position Position, extend bool) error {
	offset, err := offsetAt(b.text, position)
	if err != nil {
		return err
	}
	b.move(offset, extend)
	b.view.goal = -1
	return nil
}

func (b *Buffer) Select(anchor, caret Position) error {
	start, err := offsetAt(b.text, anchor)
	if err != nil {
		return err
	}
	end, err := offsetAt(b.text, caret)
	if err != nil {
		return err
	}
	b.view.anchor, b.view.cursor, b.view.goal = start, end, -1
	return nil
}

func (b *Buffer) MoveLeft(extend bool) {
	if b.HasSelection() && !extend {
		start, _ := b.selectionOffsets()
		b.move(start, false)
	} else {
		b.move(previousBoundary(b.text, b.view.cursor), extend)
	}
	b.view.goal = -1
}

func (b *Buffer) MoveRight(extend bool) {
	if b.HasSelection() && !extend {
		_, end := b.selectionOffsets()
		b.move(end, false)
	} else {
		b.move(nextBoundary(b.text, b.view.cursor), extend)
	}
	b.view.goal = -1
}

func (b *Buffer) MoveHome(extend bool) {
	start := strings.LastIndexByte(b.text[:b.view.cursor], '\n') + 1
	b.move(start, extend)
	b.view.goal = -1
}

func (b *Buffer) MoveEnd(extend bool) {
	end := strings.IndexByte(b.text[b.view.cursor:], '\n')
	if end < 0 {
		b.move(len(b.text), extend)
	} else {
		b.move(b.view.cursor+end, extend)
	}
	b.view.goal = -1
}

func (b *Buffer) MoveUp(extend bool) { b.moveVertical(-1, extend) }

func (b *Buffer) MoveDown(extend bool) { b.moveVertical(1, extend) }

func (b *Buffer) Insert(text string) error {
	if !utf8.ValidString(text) {
		return ErrInvalidUTF8
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	start, end := b.selectionOffsets()
	if start != end || text != "" {
		b.replace(start, end, text)
	}
	return nil
}

func (b *Buffer) DeleteBackward() bool {
	start, end := b.selectionOffsets()
	if start == end {
		start = previousBoundary(b.text, start)
	}
	if start == end {
		return false
	}
	b.replace(start, end, "")
	return true
}

func (b *Buffer) DeleteForward() bool {
	start, end := b.selectionOffsets()
	if start == end {
		end = nextBoundary(b.text, end)
	}
	if start == end {
		return false
	}
	b.replace(start, end, "")
	return true
}

func (b *Buffer) CanUndo() bool { return len(b.undo) > 0 }

func (b *Buffer) CanRedo() bool { return len(b.redo) > 0 }

func (b *Buffer) Undo() bool {
	if !b.CanUndo() {
		return false
	}
	last := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.splice(last.start, last.start+len(last.inserted), last.removed)
	b.view = last.before
	b.currentID = last.beforeID
	b.redo = append(b.redo, last)
	return true
}

func (b *Buffer) Redo() bool {
	if !b.CanRedo() {
		return false
	}
	last := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.splice(last.start, last.start+len(last.removed), last.inserted)
	b.view = last.after
	b.currentID = last.afterID
	b.undo = append(b.undo, last)
	return true
}

// MarkSaved sets the clean checkpoint without clearing undo history.
func (b *Buffer) MarkSaved() { b.savedID = b.currentID }

// Revision identifies the current edit-history state for an asynchronous save.
func (b *Buffer) Revision() int { return b.currentID }

// MarkSavedRevision records a written state, even if editing continued.
func (b *Buffer) MarkSavedRevision(revision int) { b.savedID = revision }

func (b *Buffer) Dirty() bool { return b.currentID != b.savedID }

// ApplySavedText takes formatter output as one undoable, clean edit.
func (b *Buffer) ApplySavedText(raw string) error {
	saved, err := New(raw)
	if err != nil {
		return err
	}
	if b.text != saved.text {
		position := b.Cursor()
		b.replace(0, len(b.text), saved.text)
		lines := b.Lines()
		position.Line = min(position.Line, len(lines)-1)
		position.Column = min(position.Column, graphemeCount(lines[position.Line]))
		_ = b.MoveTo(position, false)
		b.undo[len(b.undo)-1].after = b.view
	}
	b.lineEnding = saved.lineEnding
	b.MarkSaved()
	return nil
}

func (b *Buffer) move(offset int, extend bool) {
	b.view.cursor = offset
	if !extend {
		b.view.anchor = offset
	}
}

func (b *Buffer) moveVertical(delta int, extend bool) {
	current := b.Cursor()
	lines := b.Lines()
	target := current.Line + delta
	if target < 0 || target >= len(lines) {
		return
	}
	if b.view.goal < 0 {
		b.view.goal = current.Column
	}
	column := min(b.view.goal, graphemeCount(lines[target]))
	offset, _ := offsetAt(b.text, Position{Line: target, Column: column})
	b.move(offset, extend)
}

func (b *Buffer) selectionOffsets() (int, int) {
	if b.view.cursor < b.view.anchor {
		return b.view.cursor, b.view.anchor
	}
	return b.view.anchor, b.view.cursor
}

func (b *Buffer) replace(start, end int, inserted string) {
	change := edit{
		start: start, beforeID: b.currentID, afterID: b.nextID + 1,
		removed: strings.Clone(b.text[start:end]), inserted: inserted, before: b.view,
	}
	b.splice(start, end, inserted)
	b.view.cursor = forwardBoundary(b.text, start+len(inserted))
	b.view.anchor = b.view.cursor
	b.view.goal = -1
	change.after = b.view
	b.undo = append(b.undo, change)
	b.redo = nil
	b.nextID, b.currentID = change.afterID, change.afterID
}

func (b *Buffer) splice(start, end int, text string) {
	b.text = b.text[:start] + text + b.text[end:]
}

func offsetAt(text string, position Position) (int, error) {
	if position.Line < 0 || position.Column < 0 {
		return 0, ErrPosition
	}
	start := 0
	for range position.Line {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			return 0, ErrPosition
		}
		start += end + 1
	}
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += start
	}
	if position.Column == 0 {
		return start, nil
	}
	graphemes := uniseg.NewGraphemes(text[start:end])
	for column := 1; graphemes.Next(); column++ {
		if column == position.Column {
			_, to := graphemes.Positions()
			return start + to, nil
		}
	}
	return 0, ErrPosition
}

func positionAt(text string, offset int) Position {
	line := strings.Count(text[:offset], "\n")
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	return Position{Line: line, Column: graphemeCount(text[start:offset])}
}

func graphemeCount(text string) int {
	count := 0
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		count++
	}
	return count
}

func previousBoundary(text string, offset int) int {
	if offset == 0 {
		return 0
	}
	if text[offset-1] == '\n' {
		return offset - 1
	}
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	previous := start
	graphemes := uniseg.NewGraphemes(text[start:offset])
	for graphemes.Next() {
		from, _ := graphemes.Positions()
		previous = start + from
	}
	return previous
}

func nextBoundary(text string, offset int) int {
	if offset == len(text) {
		return offset
	}
	if text[offset] == '\n' {
		return offset + 1
	}
	end := strings.IndexByte(text[offset:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += offset
	}
	graphemes := uniseg.NewGraphemes(text[offset:end])
	if graphemes.Next() {
		_, to := graphemes.Positions()
		return offset + to
	}
	return offset
}

func forwardBoundary(text string, offset int) int {
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	if start == offset {
		return offset
	}
	end := strings.IndexByte(text[offset:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += offset
	}
	graphemes := uniseg.NewGraphemes(text[start:end])
	for graphemes.Next() {
		_, to := graphemes.Positions()
		if start+to >= offset {
			return start + to
		}
	}
	return end
}

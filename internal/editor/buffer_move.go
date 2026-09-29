package editor

import (
	"strings"

	"github.com/rivo/uniseg"
)

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

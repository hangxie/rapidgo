package editor

// NewView shares text, undo history, and save state with an independent caret.
func (b *Buffer) NewView() *Buffer {
	b.nextView++
	other := &Buffer{document: b.document, view: b.view, id: b.nextView}
	b.views[other] = struct{}{}
	return other
}

// CloseView releases a view when its window is removed.
func (b *Buffer) CloseView() { delete(b.views, b) }

func (b *Buffer) updateViews(start, end, inserted int) {
	for other := range b.views {
		other.view.cursor = adjustedOffset(b.text, other.view.cursor, start, end, inserted)
		other.view.anchor = adjustedOffset(b.text, other.view.anchor, start, end, inserted)
		other.view.goal = -1
	}
}

func adjustedOffset(text string, offset, start, end, inserted int) int {
	if offset >= end {
		offset += inserted - (end - start)
	} else if offset > start {
		offset = start + inserted
	}
	return forwardBoundary(text, min(len(text), max(0, offset)))
}

func (b *Buffer) snapshotViews() map[*Buffer][2]Position {
	positions := make(map[*Buffer][2]Position, len(b.views))
	for other := range b.views {
		if other != b {
			positions[other] = [2]Position{other.Anchor(), other.Cursor()}
		}
	}
	return positions
}

func (b *Buffer) restoreViews(positions map[*Buffer][2]Position) {
	lines := b.Lines()
	clamp := func(position Position) Position {
		position.Line = min(position.Line, len(lines)-1)
		position.Column = min(position.Column, graphemeCount(lines[position.Line]))
		return position
	}
	for other, view := range positions {
		_ = other.Select(clamp(view[0]), clamp(view[1]))
	}
}

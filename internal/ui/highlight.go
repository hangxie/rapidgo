package ui

import (
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/highlight"
)

// syntaxCache belongs to the presentation layer. The buffer knows nothing
// about syntax, and its revision invalidates spans after edits or undo/redo.
type syntaxCache struct {
	buffer   *editor.Buffer
	revision int
	spans    []highlight.Span
}

func (cache *syntaxCache) spansFor(buffer *editor.Buffer) []highlight.Span {
	if cache == nil {
		return highlight.Go(buffer.Text())
	}
	if cache.buffer != buffer || cache.revision != buffer.Revision() {
		cache.buffer = buffer
		cache.revision = buffer.Revision()
		cache.spans = highlight.Go(buffer.Text())
	}
	return cache.spans
}

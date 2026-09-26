package ui

import (
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/highlight"
)

// syntaxCache keys spans by buffer revision, which edits invalidate.
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

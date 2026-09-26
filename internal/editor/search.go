package editor

import (
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Match is a literal search result at grapheme-cluster boundaries.
type Match struct {
	Start, End Position
	Wrapped    bool
}

// FindNext starts after the selection, or at the caret, and wraps once.
func (b *Buffer) FindNext(query string) (Match, bool) {
	if query == "" || !utf8.ValidString(query) {
		return Match{}, false
	}
	origin := b.view.cursor
	if b.HasSelection() {
		_, origin = b.selectionOffsets()
	}
	type candidate struct{ start, end int }
	var pending []candidate
	var before, after *candidate
	boundary := 0
	clusters := uniseg.NewGraphemes(b.text)
	for {
		for len(pending) > 0 && pending[0].end <= boundary {
			found := pending[0]
			pending = pending[1:]
			if found.end != boundary {
				continue
			}
			if found.start >= origin {
				if after == nil {
					after = &found
				}
			} else if before == nil {
				before = &found
			}
		}
		if after != nil {
			break
		}
		if strings.HasPrefix(b.text[boundary:], query) {
			pending = append(pending, candidate{boundary, boundary + len(query)})
		}
		if !clusters.Next() {
			break
		}
		_, boundary = clusters.Positions()
	}
	chosen, wrapped := after, false
	if chosen == nil {
		chosen, wrapped = before, true
	}
	if chosen == nil {
		return Match{}, false
	}
	return Match{Start: positionAt(b.text, chosen.start), End: positionAt(b.text, chosen.end), Wrapped: wrapped}, true
}

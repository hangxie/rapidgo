package debug

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// maxLineBytes bounds one output line, matching job output.
const maxLineBytes = 64 << 10

// outputLines splits adapter output chunks into lines per category.
type outputLines struct {
	partial map[string]string
}

// add returns the complete lines in text, keeping a trailing partial line for later.
func (o *outputLines) add(category, text string) []Event {
	if o.partial == nil {
		o.partial = make(map[string]string)
	}
	pending := o.partial[category] + text
	var lines []Event
	for {
		end := strings.IndexByte(pending, '\n')
		if end < 0 {
			break
		}
		lines = appendLine(lines, category, strings.TrimSuffix(pending[:end], "\r"))
		pending = pending[end+1:]
	}
	for len(pending) > maxLineBytes {
		cut := lineCut(pending)
		lines = append(lines, Event{Kind: Output, Category: category, Text: pending[:cut]})
		pending = pending[cut:]
	}
	o.partial[category] = pending
	return lines
}

// flush returns unterminated lines in category order once output has ended.
func (o *outputLines) flush() []Event {
	var lines []Event
	categories := make([]string, 0, len(o.partial))
	for category := range o.partial {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	for _, category := range categories {
		if pending := o.partial[category]; pending != "" {
			lines = appendLine(lines, category, pending)
		}
	}
	o.partial = nil
	return lines
}

func appendLine(lines []Event, category, line string) []Event {
	for len(line) > maxLineBytes {
		cut := lineCut(line)
		lines = append(lines, Event{Kind: Output, Category: category, Text: line[:cut]})
		line = line[cut:]
	}
	return append(lines, Event{Kind: Output, Category: category, Text: line})
}

// lineCut finds the last rune boundary within the line limit.
func lineCut(text string) int {
	cut := maxLineBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	if cut == 0 {
		return maxLineBytes
	}
	return cut
}

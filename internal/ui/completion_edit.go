package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/gopls"
)

type completionEdit struct {
	start, end         editor.Position
	startByte, endByte int
	text               string
	main               bool
}

func applyCompletionItem(buffer *editor.Buffer, item gopls.CompletionItem, cursor editor.Position) error {
	lines := buffer.Lines()
	edits, err := prepareCompletionEdits(lines, item, cursor)
	if err != nil {
		return err
	}
	if len(edits) == 1 {
		if err := buffer.Select(edits[0].start, edits[0].end); err != nil {
			return err
		}
		return buffer.Insert(edits[0].text)
	}
	updated, caretByte := mergeCompletionEdits(buffer.Text(), edits)
	position := completionTextPosition(updated, caretByte)
	last := editor.Position{Line: len(lines) - 1, Column: graphemeCount(lines[len(lines)-1])}
	if err := buffer.Select(editor.Position{}, last); err != nil {
		return err
	}
	if err := buffer.Insert(updated); err != nil {
		return err
	}
	return buffer.MoveTo(position, false)
}

func prepareCompletionEdits(lines []string, item gopls.CompletionItem, cursor editor.Position) ([]completionEdit, error) {
	edits := make([]completionEdit, 0, len(item.AdditionalTextEdits)+1)
	var main completionEdit
	var err error
	if item.TextEdit != nil {
		main, err = prepareTextEdit(lines, *item.TextEdit, true)
	} else {
		main, err = prepareFallbackEdit(lines, item, cursor)
	}
	if err != nil {
		return nil, err
	}
	edits = append(edits, main)
	for _, edit := range item.AdditionalTextEdits {
		prepared, err := prepareTextEdit(lines, edit, false)
		if err != nil {
			return nil, err
		}
		edits = append(edits, prepared)
	}
	cursorByte, err := completionByteOffset(lines, cursor)
	if err != nil {
		return nil, err
	}
	if cursorByte < main.startByte || cursorByte > main.endByte {
		return nil, fmt.Errorf("completion edit does not contain the caret")
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].startByte < edits[j].startByte })
	for index := 1; index < len(edits); index++ {
		previous, next := edits[index-1], edits[index]
		if previous.endByte > next.startByte || previous.startByte == next.startByte || (previous.endByte == next.startByte && (previous.startByte == previous.endByte || next.startByte == next.endByte)) {
			return nil, fmt.Errorf("completion edits overlap")
		}
	}
	return edits, nil
}

func prepareTextEdit(lines []string, edit gopls.TextEdit, main bool) (completionEdit, error) {
	if !utf8.ValidString(edit.NewText) {
		return completionEdit{}, editor.ErrInvalidUTF8
	}
	start, startByte, err := completionPosition(lines, edit.Range.Start)
	if err != nil {
		return completionEdit{}, err
	}
	end, endByte, err := completionPosition(lines, edit.Range.End)
	if err != nil {
		return completionEdit{}, err
	}
	if endByte < startByte {
		return completionEdit{}, fmt.Errorf("completion edit has reversed range")
	}
	return completionEdit{start: start, end: end, startByte: startByte, endByte: endByte, text: strings.ReplaceAll(edit.NewText, "\r\n", "\n"), main: main}, nil
}

func prepareFallbackEdit(lines []string, item gopls.CompletionItem, cursor editor.Position) (completionEdit, error) {
	text := item.InsertText
	if text == "" {
		text = item.Label
	}
	if !utf8.ValidString(text) {
		return completionEdit{}, editor.ErrInvalidUTF8
	}
	place, err := completionByteOffset(lines, cursor)
	if err != nil {
		return completionEdit{}, err
	}
	lineBase := 0
	for index := 0; index < cursor.Line; index++ {
		lineBase += len(lines[index]) + 1
	}
	line := lines[cursor.Line]
	caretInLine := place - lineBase
	startInLine, endInLine := completionIdentifierRange(line, caretInLine)
	start := editor.Position{Line: cursor.Line, Column: graphemeCount(line[:startInLine])}
	end := editor.Position{Line: cursor.Line, Column: graphemeCount(line[:endInLine])}
	startByte, err := completionByteOffset(lines, start)
	if err != nil {
		return completionEdit{}, fmt.Errorf("completion range start: %w", err)
	}
	if startByte != lineBase+startInLine {
		return completionEdit{}, fmt.Errorf("completion range start splits a grapheme")
	}
	endByte, err := completionByteOffset(lines, end)
	if err != nil {
		return completionEdit{}, fmt.Errorf("completion range end: %w", err)
	}
	if endByte != lineBase+endInLine {
		return completionEdit{}, fmt.Errorf("completion range end splits a grapheme")
	}
	return completionEdit{start: start, end: end, startByte: startByte, endByte: endByte, text: strings.ReplaceAll(text, "\r\n", "\n"), main: true}, nil
}

// completionIdentifierRange selects the Go identifier around the caret.
func completionIdentifierRange(line string, caret int) (int, int) {
	start := caret
	for start > 0 {
		value, size := utf8.DecodeLastRuneInString(line[:start])
		if !completionIdentifierContinue(value) {
			break
		}
		start -= size
	}
	for start < caret {
		first, size := utf8.DecodeRuneInString(line[start:caret])
		if completionIdentifierStart(first) {
			break
		}
		start += size
	}
	end := caret
	for end < len(line) {
		value, size := utf8.DecodeRuneInString(line[end:])
		if !completionIdentifierContinue(value) || (start == caret && end == caret && !completionIdentifierStart(value)) {
			break
		}
		end += size
	}
	return start, end
}

func completionIdentifierStart(value rune) bool { return value == '_' || unicode.IsLetter(value) }

func completionIdentifierContinue(value rune) bool {
	return completionIdentifierStart(value) || unicode.IsDigit(value)
}

func mergeCompletionEdits(original string, edits []completionEdit) (string, int) {
	var result strings.Builder
	result.Grow(len(original))
	previous, caretByte := 0, 0
	for _, edit := range edits {
		result.WriteString(original[previous:edit.startByte])
		result.WriteString(edit.text)
		if edit.main {
			caretByte = result.Len()
		}
		previous = edit.endByte
	}
	result.WriteString(original[previous:])
	return result.String(), caretByte
}

func completionPosition(lines []string, position gopls.Position) (editor.Position, int, error) {
	if position.Line < 0 || position.Line >= len(lines) || position.Character < 0 {
		return editor.Position{}, 0, editor.ErrPosition
	}
	line := lines[position.Line]
	units, column, lineByte := 0, 0, 0
	clusters := uniseg.NewGraphemes(line)
	for clusters.Next() {
		if units == position.Character {
			break
		}
		for _, value := range clusters.Str() {
			units += utf16.RuneLen(value)
		}
		column++
		_, lineByte = clusters.Positions()
		if units > position.Character {
			return editor.Position{}, 0, fmt.Errorf("completion position splits a grapheme")
		}
	}
	if units != position.Character {
		return editor.Position{}, 0, editor.ErrPosition
	}
	offset := lineByte
	for index := 0; index < position.Line; index++ {
		offset += len(lines[index]) + 1
	}
	return editor.Position{Line: position.Line, Column: column}, offset, nil
}

func completionByteOffset(lines []string, position editor.Position) (int, error) {
	if position.Line < 0 || position.Line >= len(lines) || position.Column < 0 {
		return 0, editor.ErrPosition
	}
	offset := 0
	for index := 0; index < position.Line; index++ {
		offset += len(lines[index]) + 1
	}
	clusters := uniseg.NewGraphemes(lines[position.Line])
	for index := 0; index < position.Column; index++ {
		if !clusters.Next() {
			return 0, editor.ErrPosition
		}
		_, end := clusters.Positions()
		if index == position.Column-1 {
			offset += end
		}
	}
	return offset, nil
}

func completionTextPosition(value string, offset int) editor.Position {
	position := editor.Position{}
	for index, line := range strings.Split(value, "\n") {
		if offset <= len(line) {
			position.Line = index
			clusters := uniseg.NewGraphemes(line)
			for clusters.Next() {
				_, end := clusters.Positions()
				if end > offset {
					break
				}
				position.Column++
			}
			return position
		}
		offset -= len(line) + 1
	}
	return position
}

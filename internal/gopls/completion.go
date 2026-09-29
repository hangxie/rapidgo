package gopls

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// Range uses zero-based lines and UTF-16 character offsets.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit replaces one document range with plain text.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CompletionItem describes one plain-text suggestion and its edits.
type CompletionItem struct {
	Label               string
	Detail              string
	InsertText          string
	TextEdit            *TextEdit
	AdditionalTextEdits []TextEdit
}

// Complete requests suggestions for a position in an open document.
func (s *Session) Complete(ctx context.Context, path string, position Position) ([]CompletionItem, error) {
	s.state.Lock()
	_, open := s.opened[path]
	s.state.Unlock()
	if !open {
		return nil, fmt.Errorf("document is not open: %s", path)
	}
	result, err := s.request(ctx, "textDocument/completion", map[string]any{
		"textDocument": map[string]string{"uri": fileURI(path)}, "position": position,
	})
	if err != nil {
		return nil, err
	}
	return decodeCompletions(result)
}

func decodeCompletions(raw json.RawMessage) ([]CompletionItem, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var entries []json.RawMessage
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("decode gopls completion: %w", err)
		}
	} else {
		var list struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("decode gopls completion: %w", err)
		}
		entries = list.Items
	}
	items := make([]CompletionItem, 0, len(entries))
	for _, entry := range entries {
		var wire struct {
			Label               string          `json:"label"`
			Detail              string          `json:"detail"`
			InsertText          string          `json:"insertText"`
			InsertTextFormat    int             `json:"insertTextFormat"`
			TextEdit            json.RawMessage `json:"textEdit"`
			AdditionalTextEdits []TextEdit      `json:"additionalTextEdits"`
		}
		if err := json.Unmarshal(entry, &wire); err != nil {
			return nil, fmt.Errorf("decode gopls completion item: %w", err)
		}
		if wire.Label == "" || wire.InsertTextFormat == 2 {
			continue
		}
		item := CompletionItem{Label: wire.Label, Detail: wire.Detail, InsertText: wire.InsertText, AdditionalTextEdits: wire.AdditionalTextEdits}
		if len(wire.TextEdit) > 0 && string(wire.TextEdit) != "null" {
			var edit struct {
				Range   *Range `json:"range"`
				Replace *Range `json:"replace"`
				Insert  *Range `json:"insert"`
				NewText string `json:"newText"`
			}
			if err := json.Unmarshal(wire.TextEdit, &edit); err != nil {
				return nil, fmt.Errorf("decode gopls completion edit: %w", err)
			}
			target := edit.Range
			if target == nil {
				target = edit.Replace
			}
			if target == nil {
				target = edit.Insert
			}
			if target == nil {
				return nil, fmt.Errorf("decode gopls completion edit: missing range")
			}
			item.TextEdit = &TextEdit{Range: *target, NewText: edit.NewText}
		}
		items = append(items, item)
	}
	return items, nil
}

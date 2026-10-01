package gopls

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// Hover returns plain-text information for a position in an open document.
func (s *Session) Hover(ctx context.Context, path string, position Position) (string, error) {
	s.state.Lock()
	_, open := s.opened[path]
	s.state.Unlock()
	if !open {
		return "", i18n.Errorf("msg_document_is_not_open_s", path)
	}
	result, err := s.request(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]string{"uri": fileURI(path)}, "position": position,
	})
	if err != nil {
		return "", err
	}
	if len(result) == 0 || string(result) == "null" {
		return "", nil
	}
	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return "", i18n.Errorf("msg_decode_gopls_hover_w", err)
	}
	return hoverContents(hover.Contents)
}

func hoverContents(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value, nil
	}
	var markup struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &markup) == nil {
		return markup.Value, nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) == nil {
		var text []string
		for _, part := range parts {
			value, err := hoverContents(part)
			if err != nil {
				return "", err
			}
			if value != "" {
				text = append(text, value)
			}
		}
		return strings.Join(text, "\n"), nil
	}
	return "", i18n.Error("msg_unsupported_gopls_hover_contents")
}

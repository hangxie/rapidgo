package gopls

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// Location is a local source position reported by gopls.
type Location struct {
	Path     string
	Position Position
}

// Definition returns declaration locations for a position in an open document.
func (s *Session) Definition(ctx context.Context, path string, position Position) ([]Location, error) {
	return s.locations(ctx, path, position, "textDocument/definition", nil)
}

// References returns uses and declarations for a position in an open document.
func (s *Session) References(ctx context.Context, path string, position Position) ([]Location, error) {
	return s.locations(ctx, path, position, "textDocument/references", map[string]bool{"includeDeclaration": true})
}

func (s *Session) locations(ctx context.Context, path string, position Position, method string, referenceContext any) ([]Location, error) {
	s.state.Lock()
	_, open := s.opened[path]
	s.state.Unlock()
	if !open {
		return nil, i18n.Errorf("msg_document_is_not_open_s", path)
	}
	params := map[string]any{"textDocument": map[string]string{"uri": fileURI(path)}, "position": position}
	if referenceContext != nil {
		params["context"] = referenceContext
	}
	result, err := s.request(ctx, method, params)
	if err != nil {
		return nil, err
	}
	locations, err := decodeLocations(result)
	if err != nil {
		return nil, i18n.Errorf("msg_decode_gopls_s_w", strings.TrimPrefix(method, "textDocument/"), err)
	}
	return locations, nil
}

func decodeLocations(raw json.RawMessage) ([]Location, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var values []json.RawMessage
	if len(strings.TrimSpace(string(raw))) > 0 && strings.TrimSpace(string(raw))[0] == '[' {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	} else {
		values = []json.RawMessage{raw}
	}
	locations := make([]Location, 0, len(values))
	for _, value := range values {
		var item struct {
			URI                  string `json:"uri"`
			Range                *Range `json:"range"`
			TargetURI            string `json:"targetUri"`
			TargetRange          *Range `json:"targetRange"`
			TargetSelectionRange *Range `json:"targetSelectionRange"`
		}
		if err := json.Unmarshal(value, &item); err != nil {
			return nil, err
		}
		uri, sourceRange := item.URI, item.Range
		if item.TargetURI != "" {
			uri, sourceRange = item.TargetURI, item.TargetSelectionRange
			if sourceRange == nil {
				sourceRange = item.TargetRange
			}
		}
		if sourceRange == nil {
			return nil, i18n.Error("msg_location_has_no_range")
		}
		path, err := pathFromURI(uri)
		if err != nil {
			return nil, err
		}
		if sourceRange.Start.Line < 0 || sourceRange.Start.Character < 0 {
			return nil, i18n.Error("msg_location_has_negative_position")
		}
		locations = append(locations, Location{Path: path, Position: sourceRange.Start})
	}
	return locations, nil
}

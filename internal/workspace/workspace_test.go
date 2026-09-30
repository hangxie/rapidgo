package workspace

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

func TestWindows(t *testing.T) {
	w := &Workspace{}
	b, _ := editor.New("界\nhello")
	first := w.Open(&project.Document{Path: "a.go"}, b)
	first.Scroll = 3
	second := w.Duplicate()
	_ = second.Buffer.Insert("x")
	if first.Buffer.Text() != second.Buffer.Text() {
		t.Fatal("documents diverged")
	}
	second.Scroll = 7
	w.Next(-1)
	if w.Current() != first || first.Scroll != 3 {
		t.Fatal("window state lost")
	}
	w.Open(&project.Document{Path: "b.go"}, b)
	if !w.Activate("a.go") || w.Current() != first {
		t.Fatal("open document not found")
	}
}

func TestLayouts(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {20, 8}, {1, 1}, {0, 0}} {
		for _, mode := range []Mode{Tile, Cascade} {
			w := &Workspace{}
			for range 5 {
				b, _ := editor.New("")
				w.Open(&project.Document{}, b)
			}
			w.Arrange(mode, size[0], size[1])
			w.Adjust(999, 999, 999, 999, size[0], size[1])
			w.Resize(size[0]/2, size[1]/2)
			for _, win := range w.Windows {
				r := win.Rect
				if r.X < 0 || r.Y < 0 || r.X+r.Width > size[0]/2 || r.Y+r.Height > size[1]/2 {
					t.Fatalf("out of bounds: %+v", r)
				}
			}
		}
	}
}

func TestOpenSameDocumentSharesBuffer(t *testing.T) {
	w := &Workspace{}
	first, _ := editor.New("original")
	w.Open(&project.Document{Path: "a.go"}, first)
	stale, _ := editor.New("stale disk contents")
	win := w.Open(&project.Document{Path: "a.go"}, stale)
	_ = win.Buffer.Insert("x")
	if first.Text() != win.Buffer.Text() || win.Document.Text == "stale disk contents" {
		t.Fatal("second window used a separate document")
	}
}

func TestDuplicateCopiesActiveView(t *testing.T) {
	for _, test := range []struct {
		name           string
		anchor, cursor editor.Position
	}{
		{name: "caret", anchor: editor.Position{Line: 1, Column: 3}, cursor: editor.Position{Line: 1, Column: 3}},
		{name: "selection", anchor: editor.Position{Line: 1, Column: 1}, cursor: editor.Position{Line: 1, Column: 5}},
		{name: "reverse selection", anchor: editor.Position{Line: 1, Column: 5}, cursor: editor.Position{Line: 1, Column: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := &Workspace{}
			buffer, err := editor.New("界abc\nsecond\nthird")
			require.NoError(t, err)
			first := w.Open(&project.Document{Path: "foo.go"}, buffer)
			require.NoError(t, first.Buffer.Select(editor.Position{}, editor.Position{Column: 1}))
			second := w.Duplicate()
			require.NoError(t, second.Buffer.Select(test.anchor, test.cursor))
			second.Scroll, second.Column = 1, 4
			third := w.Duplicate()
			require.Equal(t, test.cursor, third.Buffer.Cursor(), "duplicate must copy the active caret")
			require.Equal(t, test.anchor, third.Buffer.Anchor(), "duplicate must copy the active selection")
			require.Equal(t, second.Scroll, third.Scroll)
			require.Equal(t, second.Column, third.Column)
			require.Same(t, third, w.Current())
			require.Same(t, second.Document, third.Document)
			require.NotSame(t, second.Buffer, third.Buffer)
			require.NoError(t, third.Buffer.MoveTo(editor.Position{Line: 2}, false))
			require.Equal(t, test.cursor, second.Buffer.Cursor(), "movement must preserve the source caret")
			require.Equal(t, test.anchor, second.Buffer.Anchor(), "movement must preserve the source selection")
			require.NoError(t, third.Buffer.Insert("x"))
			require.Equal(t, third.Buffer.Text(), first.Buffer.Text())
			require.Equal(t, third.Buffer.Text(), second.Buffer.Text())
		})
	}
}

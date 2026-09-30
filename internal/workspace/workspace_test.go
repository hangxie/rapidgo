package workspace

import (
	"fmt"
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

func TestCloseCurrentPreservesWindowOrderAndSharedDocument(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	firstBuffer, err := editor.New("first")
	require.NoError(t, err)
	first := w.Open(&project.Document{Path: "a.go"}, firstBuffer)
	duplicate := w.Duplicate()
	lastBuffer, err := editor.New("last")
	require.NoError(t, err)
	last := w.Open(&project.Document{Path: "b.go"}, lastBuffer)
	require.True(t, w.Activate("a.go"))
	require.Same(t, first, w.CloseCurrent())
	require.Equal(t, []*Window{duplicate, last}, w.Windows)
	require.Same(t, last, w.Current())
	require.Equal(t, []string{"a.go", "b.go"}, windowPaths(w))
	require.NoError(t, duplicate.Buffer.Insert("x"))
	require.Equal(t, "xfirst", duplicate.Buffer.Text())
	w.Next(-1)
	require.Same(t, duplicate, w.CloseCurrent())
	require.Same(t, last, w.Current())
	require.Same(t, last, w.CloseCurrent())
	require.Nil(t, w.Current())
	require.Nil(t, w.WindowAt(0))
	require.Nil(t, w.CloseCurrent())
}

func TestCloseCurrentRetilesRemainingWindows(t *testing.T) {
	w := &Workspace{}
	w.Resize(90, 24)
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	w.Arrange(Tile, 90, 24)
	last := w.Current()
	require.Same(t, last, w.CloseCurrent())
	require.Equal(t, Rect{Width: 45, Height: 24}, w.Windows[0].Rect)
	require.Equal(t, Rect{X: 45, Width: 45, Height: 24}, w.Windows[1].Rect)
}

func TestCloseCurrentRestoresLoneCascadedWindow(t *testing.T) {
	w := &Workspace{}
	w.Resize(90, 24)
	for _, path := range []string{"a.go", "b.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	w.Arrange(Cascade, 90, 24)
	w.CloseCurrent()
	require.Equal(t, Rect{Width: 90, Height: 24}, w.Current().Rect)
}

func TestCloseAllWindowsThenResizeAndReopen(t *testing.T) {
	for _, mode := range []Mode{Tile, Cascade, Manual} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			w := &Workspace{}
			buffer, err := editor.New("界")
			require.NoError(t, err)
			w.Open(&project.Document{Path: "a.go"}, buffer)
			w.Arrange(mode, 90, 24)
			w.CloseCurrent()
			for _, size := range [][2]int{{0, 0}, {1, 1}, {90, 24}} {
				require.NotPanics(t, func() { w.Resize(size[0], size[1]) })
			}
			buffer, err = editor.New("reopened")
			require.NoError(t, err)
			win := w.Open(&project.Document{Path: "a.go"}, buffer)
			require.Same(t, win, w.Current())
			require.Same(t, win, w.WindowAt(0))
			require.Equal(t, Rect{Width: 90, Height: 24}, win.Rect)
		})
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

func TestEmptyWorkspaceAndMissingWindow(t *testing.T) {
	w := &Workspace{}
	require.Nil(t, w.Current())
	require.Nil(t, w.WindowAt(-1))
	require.Nil(t, w.WindowAt(0))
	require.Nil(t, w.Duplicate())
	require.False(t, w.Zoom())
	require.False(t, w.Activate("missing.go"))
	w.Next(1)
	require.Empty(t, w.Windows)
	w.Arrange(Cascade, 20, 8)

	buffer, err := editor.New("a")
	require.NoError(t, err)
	w.Open(&project.Document{Path: "a.go"}, buffer)
	require.False(t, w.Activate("missing.go"))
}

func TestTilePartitionsNarrowArea(t *testing.T) {
	w := &Workspace{}
	for range 2 {
		buffer, err := editor.New("")
		require.NoError(t, err)
		w.Open(&project.Document{}, buffer)
	}
	w.Arrange(Tile, 15, 5)
	require.Equal(t, Rect{Width: 7, Height: 5}, w.Windows[0].Rect)
	require.Equal(t, Rect{X: 7, Width: 8, Height: 5}, w.Windows[1].Rect)
	w.Arrange(Tile, 15, 20)
	require.Equal(t, Rect{Width: 15, Height: 10}, w.Windows[0].Rect)
	require.Equal(t, Rect{Y: 10, Width: 15, Height: 10}, w.Windows[1].Rect)
}

func TestTileDoesNotOverlapOnTinyTerminal(t *testing.T) {
	w := &Workspace{}
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		buffer, err := editor.New("")
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	w.Arrange(Tile, 1, 1)
	for i, win := range w.Windows {
		require.GreaterOrEqual(t, win.Rect.Width, 0)
		require.GreaterOrEqual(t, win.Rect.Height, 0)
		for j := 0; j < i; j++ {
			require.False(t, rectanglesOverlap(win.Rect, w.Windows[j].Rect))
		}
	}
	require.True(t, w.Activate("a.go"))
	require.Greater(t, w.Current().Rect.Width*w.Current().Rect.Height, 0)
	w.Arrange(Tile, 1, 1)
	require.Greater(t, w.Current().Rect.Width*w.Current().Rect.Height, 0)
	w.Next(1)
	require.Greater(t, w.Current().Rect.Width*w.Current().Rect.Height, 0)
	w.Resize(2, 1)
	w.Resize(1, 1)
	require.Greater(t, w.Current().Rect.Width*w.Current().Rect.Height, 0)
}

func TestNewWindowsOverlapWithoutMovingEarlierWindows(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	firstBuffer, err := editor.New("first")
	require.NoError(t, err)
	first := w.Open(&project.Document{Path: "a.go"}, firstBuffer)
	firstRect := first.Rect
	secondBuffer, err := editor.New("second")
	require.NoError(t, err)
	second := w.Open(&project.Document{Path: "b.go"}, secondBuffer)
	require.Equal(t, firstRect, first.Rect)
	require.Less(t, second.Rect.X, first.Rect.X+first.Rect.Width)
	require.Less(t, second.Rect.Y, first.Rect.Y+first.Rect.Height)
	require.Greater(t, second.Rect.X+second.Rect.Width, first.Rect.X)
	require.Greater(t, second.Rect.Y+second.Rect.Height, first.Rect.Y)
	require.NotEqual(t, first.Rect, second.Rect)
}

func TestSingleWindowFillsWorkAreaUntilMoved(t *testing.T) {
	for _, sizedBeforeOpen := range []bool{false, true} {
		t.Run(map[bool]string{false: "size after open", true: "size before open"}[sizedBeforeOpen], func(t *testing.T) {
			w := &Workspace{}
			if sizedBeforeOpen {
				w.Resize(100, 30)
			}
			buffer, err := editor.New("hello")
			require.NoError(t, err)
			win := w.Open(&project.Document{Path: "a.go"}, buffer)
			w.Resize(100, 30)
			require.Equal(t, Rect{Width: 100, Height: 30}, win.Rect)
			w.Resize(80, 25)
			require.Equal(t, Rect{Width: 80, Height: 25}, win.Rect)
			w.Adjust(1, 1, -10, -4, 80, 25)
			manual := win.Rect
			w.Resize(90, 28)
			require.Equal(t, manual, win.Rect)
		})
	}
}

func TestOpenBeforeTerminalSizeStillCascades(t *testing.T) {
	w := &Workspace{}
	for _, path := range []string{"a.go", "b.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	w.Resize(20, 8)
	require.True(t, rectanglesOverlap(w.Windows[0].Rect, w.Windows[1].Rect))
	require.NotEqual(t, w.Windows[0].Rect, w.Windows[1].Rect)
	for _, win := range w.Windows {
		require.LessOrEqual(t, win.Rect.X+win.Rect.Width, 20)
		require.LessOrEqual(t, win.Rect.Y+win.Rect.Height, 8)
	}
}

func TestWindowZOrderFollowsSelection(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	require.Equal(t, "c.go", w.WindowAt(2).Document.Path)
	require.True(t, w.Activate("a.go"))
	require.Equal(t, []string{"b.go", "c.go", "a.go"}, windowPaths(w))
	w.Next(1)
	require.Equal(t, []string{"c.go", "a.go", "b.go"}, windowPaths(w))
	w.Next(-1)
	require.Equal(t, []string{"c.go", "b.go", "a.go"}, windowPaths(w))
	w.Duplicate()
	require.Same(t, w.Current(), w.WindowAt(3))
}

func windowPaths(w *Workspace) []string {
	paths := make([]string, len(w.Windows))
	for i := range paths {
		paths[i] = w.WindowAt(i).Document.Path
	}
	return paths
}

func TestCascadeTileAndNewWindowGeometry(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	w.Arrange(Cascade, 100, 30)
	for i := 1; i < len(w.Windows); i++ {
		require.Greater(t, w.WindowAt(i).Rect.X, w.WindowAt(i-1).Rect.X)
		require.Greater(t, w.WindowAt(i).Rect.Y, w.WindowAt(i-1).Rect.Y)
		require.True(t, rectanglesOverlap(w.WindowAt(i).Rect, w.WindowAt(i-1).Rect))
	}
	w.Arrange(Tile, 100, 30)
	for i := range w.Windows {
		for j := 0; j < i; j++ {
			require.False(t, rectanglesOverlap(w.Windows[i].Rect, w.Windows[j].Rect))
		}
	}
	previous := w.Windows[0].Rect
	buffer, err := editor.New("d")
	require.NoError(t, err)
	newWindow := w.Open(&project.Document{Path: "d.go"}, buffer)
	require.Equal(t, previous, w.Windows[0].Rect)
	require.True(t, rectanglesOverlap(newWindow.Rect, previous))
	w.Arrange(Tile, 100, 30)
	tiled := w.Current().Rect
	w.Duplicate()
	require.Equal(t, tiled, w.Windows[3].Rect)
}

func rectanglesOverlap(a, b Rect) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func TestMoveResizeAndZoomPreserveOtherWindows(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	for _, path := range []string{"a.go", "b.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	require.True(t, w.Activate("a.go"))
	other := w.Windows[1].Rect
	w.Adjust(2, 1, -3, -2, 100, 30)
	saved := w.Current().Rect
	require.Equal(t, other, w.Windows[1].Rect)
	require.True(t, w.Zoom())
	require.Equal(t, Rect{Width: 100, Height: 30}, w.Current().Rect)
	require.Equal(t, other, w.Windows[1].Rect)
	w.Resize(80, 25)
	require.Equal(t, Rect{Width: 80, Height: 25}, w.Current().Rect)
	require.False(t, w.Zoom())
	require.Equal(t, clamp(saved, 80, 25), w.Current().Rect)
	require.True(t, w.Zoom())
	w.Adjust(1, 0, -1, 0, 80, 25)
	require.NotEqual(t, Rect{Width: 80, Height: 25}, w.Current().Rect)
}

func TestSwitchingWindowRestoresZoomedChild(t *testing.T) {
	w := &Workspace{}
	w.Resize(100, 30)
	for _, path := range []string{"a.go", "b.go"} {
		buffer, err := editor.New(path)
		require.NoError(t, err)
		w.Open(&project.Document{Path: path}, buffer)
	}
	require.True(t, w.Activate("a.go"))
	before := w.Current().Rect
	require.True(t, w.Zoom())
	require.True(t, w.Activate("b.go"))
	require.Equal(t, before, w.Windows[0].Rect)
	require.NotEqual(t, Rect{Width: 100, Height: 30}, w.Windows[1].Rect)
	second := w.Current().Rect
	require.True(t, w.Zoom())
	w.Next(1)
	require.Equal(t, second, w.Windows[1].Rect)
}

func TestSelectWindowByIndex(t *testing.T) {
	w := &Workspace{}
	require.False(t, w.Select(0))
	w.Resize(100, 30)
	buffer, err := editor.New("界")
	require.NoError(t, err)
	first := w.Open(&project.Document{Path: "a.go"}, buffer)
	second := w.Duplicate()
	before := second.Rect
	require.True(t, w.Zoom())
	require.True(t, w.Select(0))
	require.Equal(t, before, second.Rect)
	require.Same(t, first, w.Current())
	require.Same(t, first, w.WindowAt(1))
	for _, index := range []int{-1, 2} {
		require.False(t, w.Select(index))
		require.Same(t, first, w.Current())
	}
	require.True(t, w.Select(1))
	require.Same(t, second, w.Current())
}

// Package workspace owns editor windows independently of terminal rendering.
package workspace

import (
	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

// Rect locates a window relative to the editor work area.
type Rect struct{ X, Y, Width, Height int }

// Window owns a document view and its viewport.
type Window struct {
	Document       *project.Document
	Buffer         *editor.Buffer
	Scroll, Column int
	Rect           Rect
	normal         Rect
	zoomed         bool
}

// Mode selects automatic window arrangement.
type Mode uint8

const (
	Cascade Mode = iota
	Tile
	Manual
)

// Workspace keeps window order and the active window.
type Workspace struct {
	Windows       []*Window
	Active        int
	zOrder        []int
	mode          Mode
	width, height int
}

func (w *Workspace) Current() *Window {
	if len(w.Windows) == 0 {
		return nil
	}
	return w.Windows[w.Active]
}

// WindowAt returns a window by back-to-front layer.
func (w *Workspace) WindowAt(layer int) *Window {
	if layer < 0 || layer >= len(w.zOrder) {
		return nil
	}
	return w.Windows[w.zOrder[layer]]
}

func (w *Workspace) Open(document *project.Document, buffer *editor.Buffer) *Window {
	w.restoreCurrent()
	for _, existing := range w.Windows {
		if document.Path != "" && existing.Document.Path == document.Path {
			document, buffer = existing.Document, existing.Buffer.NewView()
			break
		}
	}
	win := &Window{Document: document, Buffer: buffer, Rect: w.newWindowRect()}
	w.Windows = append(w.Windows, win)
	w.Active = len(w.Windows) - 1
	w.zOrder = append(w.zOrder, w.Active)
	if w.mode == Tile {
		w.mode = Manual
	}
	return win
}

func (w *Workspace) Activate(path string) bool {
	for i, win := range w.Windows {
		if win.Document.Path == path {
			if i != w.Active {
				w.restoreCurrent()
			}
			w.Active = i
			w.raise(i)
			w.ensureActiveVisible()
			return true
		}
	}
	return false
}

func (w *Workspace) Duplicate() *Window {
	current := w.Current()
	if current == nil {
		return nil
	}
	w.restoreCurrent()
	win := &Window{
		Document: current.Document,
		Buffer:   current.Buffer.NewView(),
		Scroll:   current.Scroll,
		Column:   current.Column,
		Rect:     w.newWindowRect(),
	}
	w.Windows = append(w.Windows, win)
	w.Active = len(w.Windows) - 1
	w.zOrder = append(w.zOrder, w.Active)
	if w.mode == Tile {
		w.mode = Manual
	}
	return win
}

func (w *Workspace) Next(delta int) {
	if len(w.Windows) > 0 {
		next := (w.Active + delta%len(w.Windows) + len(w.Windows)) % len(w.Windows)
		if next != w.Active {
			w.restoreCurrent()
		}
		w.Active = next
		w.raise(w.Active)
		w.ensureActiveVisible()
	}
}

func (w *Workspace) ensureActiveVisible() {
	active := w.Current()
	if w.mode != Tile || (active.Rect.Width > 0 && active.Rect.Height > 0) {
		return
	}
	for _, win := range w.Windows {
		if win.Rect.Width > 0 && win.Rect.Height > 0 {
			active.Rect, win.Rect = win.Rect, active.Rect
			return
		}
	}
}

func (w *Workspace) restoreCurrent() {
	if win := w.Current(); win != nil && win.zoomed {
		win.Rect = clamp(win.normal, w.width, w.height)
		win.zoomed = false
	}
}

func (w *Workspace) raise(index int) {
	for layer, value := range w.zOrder {
		if value == index {
			copy(w.zOrder[layer:], w.zOrder[layer+1:])
			w.zOrder[len(w.zOrder)-1] = index
			return
		}
	}
}

func (w *Workspace) newWindowRect() Rect {
	if len(w.Windows) == 0 {
		return Rect{Width: w.width, Height: w.height}
	}
	return w.defaultRect(len(w.Windows))
}

func (w *Workspace) defaultRect(index int) Rect {
	width := w.width - min(12, w.width/5)
	height := w.height - min(4, w.height/5)
	x := (2 * index) % (w.width - width + 1)
	y := index % (w.height - height + 1)
	return clamp(Rect{X: x, Y: y, Width: width, Height: height}, w.width, w.height)
}

func (w *Workspace) Arrange(mode Mode, width, height int) {
	w.mode = mode
	w.width, w.height = max(0, width), max(0, height)
	n := len(w.Windows)
	if n == 0 {
		return
	}
	if mode == Tile {
		w.tile()
		w.ensureActiveVisible()
		return
	}
	if mode == Cascade {
		offset := min(n-1, max(0, min(w.width-12, w.height-3)))
		for layer := range w.zOrder {
			win := w.WindowAt(layer)
			win.zoomed = false
			win.Rect = clamp(Rect{X: min(layer, offset), Y: min(layer, offset), Width: w.width - offset, Height: w.height - offset}, w.width, w.height)
		}
	}
}

func (w *Workspace) tile() {
	n := len(w.Windows)
	horizontal := w.width >= w.height*2 || w.height < n
	for i, win := range w.Windows {
		win.zoomed = false
		if horizontal {
			left, right := i*w.width/n, (i+1)*w.width/n
			win.Rect = Rect{X: left, Width: right - left, Height: w.height}
		} else {
			top, bottom := i*w.height/n, (i+1)*w.height/n
			win.Rect = Rect{Y: top, Width: w.width, Height: bottom - top}
		}
	}
}

func (w *Workspace) Resize(width, height int) {
	width, height = max(0, width), max(0, height)
	if w.width == width && w.height == height {
		return
	}
	w.width, w.height = width, height
	if w.mode == Tile {
		w.tile()
		w.ensureActiveVisible()
		return
	}
	if w.mode == Cascade && len(w.Windows) == 1 && !w.Windows[0].zoomed {
		w.Windows[0].Rect = Rect{Width: width, Height: height}
		return
	}
	for index, win := range w.Windows {
		if win.zoomed {
			win.normal = clamp(win.normal, width, height)
			win.Rect = Rect{Width: width, Height: height}
		} else if win.Rect.Width == 0 && win.Rect.Height == 0 && width > 0 && height > 0 {
			win.Rect = w.defaultRect(index)
		} else {
			win.Rect = clamp(win.Rect, width, height)
		}
	}
}

// Zoom toggles the active window between its saved rectangle and the work area.
func (w *Workspace) Zoom() bool {
	win := w.Current()
	if win == nil {
		return false
	}
	if win.zoomed {
		w.restoreCurrent()
		return false
	}
	win.normal = win.Rect
	win.Rect = Rect{Width: w.width, Height: w.height}
	win.zoomed = true
	w.mode = Manual
	return true
}

func (w *Workspace) Adjust(dx, dy, dw, dh, width, height int) {
	w.Resize(width, height)
	if win := w.Current(); win != nil {
		if win.zoomed {
			w.Zoom()
		}
		w.mode = Manual
		r := win.Rect
		r.X += dx
		r.Y += dy
		r.Width += dw
		r.Height += dh
		win.Rect = clamp(r, w.width, w.height)
	}
}

func clamp(r Rect, width, height int) Rect {
	r.Width = min(width, max(min(12, width), r.Width))
	r.Height = min(height, max(min(3, height), r.Height))
	r.X = min(max(0, r.X), width-r.Width)
	r.Y = min(max(0, r.Y), height-r.Height)
	return r
}

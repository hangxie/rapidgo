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
}

// Mode selects automatic window arrangement.
type Mode uint8

const (
	Tile Mode = iota
	Cascade
	Manual
)

// Workspace keeps window order and the active window.
type Workspace struct {
	Windows       []*Window
	Active        int
	mode          Mode
	width, height int
}

func (w *Workspace) Current() *Window {
	if len(w.Windows) == 0 {
		return nil
	}
	return w.Windows[w.Active]
}

func (w *Workspace) Open(document *project.Document, buffer *editor.Buffer) *Window {
	for _, existing := range w.Windows {
		if document.Path != "" && existing.Document.Path == document.Path {
			document, buffer = existing.Document, existing.Buffer.NewView()
			break
		}
	}
	win := &Window{Document: document, Buffer: buffer, Rect: Rect{Width: w.width, Height: w.height}}
	w.Windows = append(w.Windows, win)
	w.Active = len(w.Windows) - 1
	w.Arrange(w.mode, w.width, w.height)
	return win
}

func (w *Workspace) Activate(path string) bool {
	for i, win := range w.Windows {
		if win.Document.Path == path {
			w.Active = i
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
	win := &Window{
		Document: current.Document,
		Buffer:   current.Buffer.NewView(),
		Scroll:   current.Scroll,
		Column:   current.Column,
		Rect:     Rect{Width: w.width, Height: w.height},
	}
	w.Windows = append(w.Windows, win)
	w.Active = len(w.Windows) - 1
	w.Arrange(w.mode, w.width, w.height)
	return win
}

func (w *Workspace) Next(delta int) {
	if len(w.Windows) > 0 {
		w.Active = (w.Active + delta%len(w.Windows) + len(w.Windows)) % len(w.Windows)
	}
}

func (w *Workspace) Arrange(mode Mode, width, height int) {
	w.mode = mode
	w.width, w.height = max(0, width), max(0, height)
	n := len(w.Windows)
	for i, win := range w.Windows {
		switch mode {
		case Tile:
			if width >= n*20 && width >= height*2 {
				left, right := i*w.width/n, (i+1)*w.width/n
				win.Rect = Rect{X: left, Width: right - left, Height: w.height}
			} else if height >= n*4 {
				top, bottom := i*w.height/n, (i+1)*w.height/n
				win.Rect = Rect{Y: top, Width: w.width, Height: bottom - top}
			} else {
				win.Rect = Rect{Width: w.width, Height: w.height}
			}
		case Cascade:
			offset := i % max(1, min(max(1, w.width-20), max(1, w.height-4)))
			win.Rect = Rect{X: offset, Y: offset, Width: w.width - min(n-1, max(0, w.width-20)), Height: w.height - min(n-1, max(0, w.height-4))}
		}
		win.Rect = clamp(win.Rect, w.width, w.height)
	}
}

func (w *Workspace) Resize(width, height int) {
	if w.width == max(0, width) && w.height == max(0, height) {
		return
	}
	w.Arrange(w.mode, width, height)
}

func (w *Workspace) Adjust(dx, dy, dw, dh, width, height int) {
	w.Resize(width, height)
	if win := w.Current(); win != nil {
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

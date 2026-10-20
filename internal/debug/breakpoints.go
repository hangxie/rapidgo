// Package debug holds debugger state independently of the terminal UI.
package debug

import (
	"path/filepath"
	"slices"

	"github.com/hangxie/rapidgo/internal/dap"
)

// Breakpoint is a user-placed line breakpoint with a one-based line.
type Breakpoint struct {
	Path         string
	Line         int
	Condition    string
	HitCondition string
	Disabled     bool
	Verified     bool   // the adapter accepted it in the running session
	Message      string // the adapter's reason when it did not
}

// Breakpoints is the set of line breakpoints, keyed by cleaned absolute path; it is not safe for concurrent use.
type Breakpoints struct {
	files map[string][]Breakpoint // sorted by line, at most one per line
}

func NewBreakpoints() *Breakpoints {
	return &Breakpoints{files: make(map[string][]Breakpoint)}
}

// Toggle adds a breakpoint at line or removes the one there, reporting whether one now exists.
func (b *Breakpoints) Toggle(path string, line int) bool {
	if line < 1 {
		return false
	}
	path = filepath.Clean(path)
	if b.Remove(path, line) {
		return false
	}
	file := b.files[path]
	index, _ := find(file, line)
	b.files[path] = slices.Insert(file, index, Breakpoint{Path: path, Line: line})
	return true
}

func (b *Breakpoints) At(path string, line int) (Breakpoint, bool) {
	file := b.files[filepath.Clean(path)]
	if index, ok := find(file, line); ok {
		return file[index], true
	}
	return Breakpoint{}, false
}

func (b *Breakpoints) Remove(path string, line int) bool {
	path = filepath.Clean(path)
	file := b.files[path]
	index, ok := find(file, line)
	if !ok {
		return false
	}
	b.store(path, slices.Delete(file, index, index+1))
	return true
}

// SetCondition replaces the expression and hit-count conditions of an existing breakpoint.
func (b *Breakpoints) SetCondition(path string, line int, condition, hitCondition string) bool {
	return b.update(path, line, func(breakpoint *Breakpoint) {
		breakpoint.Condition, breakpoint.HitCondition = condition, hitCondition
	})
}

// SetDisabled keeps a breakpoint without sending it to the adapter.
func (b *Breakpoints) SetDisabled(path string, line int, disabled bool) bool {
	return b.update(path, line, func(breakpoint *Breakpoint) {
		breakpoint.Disabled = disabled
		if disabled {
			breakpoint.Verified, breakpoint.Message = false, ""
		}
	})
}

func (b *Breakpoints) InFile(path string) []Breakpoint {
	return slices.Clone(b.files[filepath.Clean(path)])
}

// All returns every breakpoint ordered by path and line.
func (b *Breakpoints) All() []Breakpoint {
	var all []Breakpoint
	for _, path := range b.Paths() {
		all = append(all, b.files[path]...)
	}
	return all
}

// Paths returns the sorted files that have breakpoints.
func (b *Breakpoints) Paths() []string {
	paths := make([]string, 0, len(b.files))
	for path := range b.files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Requests lists the enabled breakpoints in path for a setBreakpoints request; an empty list clears them.
func (b *Breakpoints) Requests(path string) []dap.SourceBreakpoint {
	requests := []dap.SourceBreakpoint{}
	for _, breakpoint := range b.files[filepath.Clean(path)] {
		if !breakpoint.Disabled {
			requests = append(requests, dap.SourceBreakpoint{
				Line: breakpoint.Line, Condition: breakpoint.Condition, HitCondition: breakpoint.HitCondition,
			})
		}
	}
	return requests
}

// Resolve records the adapter's answer to requested, matching results by position and lines by the request.
func (b *Breakpoints) Resolve(path string, requested []dap.SourceBreakpoint, results []dap.Breakpoint) {
	path = filepath.Clean(path)
	file := b.files[path]
	for index := range file {
		file[index].Verified, file[index].Message = false, ""
	}
	// The set may have changed while the request was in flight; a later request reports those lines.
	for index, request := range requested[:min(len(requested), len(results))] {
		if at, ok := find(file, request.Line); ok {
			file[at].Verified, file[at].Message = results[index].Verified, results[index].Message
		}
	}
}

// ClearVerification forgets adapter results, such as when a session ends.
func (b *Breakpoints) ClearVerification() {
	for _, file := range b.files {
		for index := range file {
			file[index].Verified, file[index].Message = false, ""
		}
	}
}

func (b *Breakpoints) update(path string, line int, change func(*Breakpoint)) bool {
	file := b.files[filepath.Clean(path)]
	index, ok := find(file, line)
	if ok {
		change(&file[index])
	}
	return ok
}

func (b *Breakpoints) store(path string, file []Breakpoint) {
	if len(file) == 0 {
		delete(b.files, path)
		return
	}
	b.files[path] = file
}

func find(file []Breakpoint, line int) (int, bool) {
	return slices.BinarySearchFunc(file, line, func(breakpoint Breakpoint, line int) int {
		return breakpoint.Line - line
	})
}

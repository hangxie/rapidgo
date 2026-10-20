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
	files    map[string][]entry // sorted by line, at most one per line
	revision uint64
}

// entry tags a breakpoint with a revision that changes whenever the adapter would see it differently.
type entry struct {
	Breakpoint
	revision uint64
}

func NewBreakpoints() *Breakpoints {
	return &Breakpoints{files: make(map[string][]entry)}
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
	b.files[path] = slices.Insert(file, index, entry{Breakpoint{Path: path, Line: line}, b.next()})
	return true
}

func (b *Breakpoints) At(path string, line int) (Breakpoint, bool) {
	file := b.files[filepath.Clean(path)]
	if index, ok := find(file, line); ok {
		return file[index].Breakpoint, true
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
	return appendBreakpoints(nil, b.files[filepath.Clean(path)])
}

// All returns every breakpoint ordered by path and line.
func (b *Breakpoints) All() []Breakpoint {
	var all []Breakpoint
	for _, path := range b.Paths() {
		all = appendBreakpoints(all, b.files[path])
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

// Request is a snapshot of one file's enabled breakpoints for a setBreakpoints request.
type Request struct {
	Path        string
	Breakpoints []dap.SourceBreakpoint // empty clears the file's breakpoints
	revisions   []uint64
}

func (b *Breakpoints) Request(path string) Request {
	request := Request{Path: filepath.Clean(path), Breakpoints: []dap.SourceBreakpoint{}}
	for _, breakpoint := range b.files[request.Path] {
		if !breakpoint.Disabled {
			request.Breakpoints = append(request.Breakpoints, dap.SourceBreakpoint{
				Line: breakpoint.Line, Condition: breakpoint.Condition, HitCondition: breakpoint.HitCondition,
			})
			request.revisions = append(request.revisions, breakpoint.revision)
		}
	}
	return request
}

// Resolve records the adapter's answer to request for breakpoints that have not changed since.
func (b *Breakpoints) Resolve(request Request, results []dap.Breakpoint) {
	file := b.files[request.Path]
	results = results[:min(len(results), len(request.revisions))]
	for index := range file {
		file[index].Verified, file[index].Message = false, ""
		// Revisions are unique, so a removed, recreated, edited, or moved breakpoint matches nothing.
		if at := slices.Index(request.revisions, file[index].revision); at >= 0 && at < len(results) {
			file[index].Verified, file[index].Message = results[at].Verified, results[at].Message
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
		change(&file[index].Breakpoint)
		file[index].revision = b.next()
	}
	return ok
}

func (b *Breakpoints) next() uint64 {
	b.revision++
	return b.revision
}

func (b *Breakpoints) store(path string, file []entry) {
	if len(file) == 0 {
		delete(b.files, path)
		return
	}
	b.files[path] = file
}

func find(file []entry, line int) (int, bool) {
	return slices.BinarySearchFunc(file, line, func(breakpoint entry, line int) int {
		return breakpoint.Line - line
	})
}

func appendBreakpoints(breakpoints []Breakpoint, file []entry) []Breakpoint {
	for _, breakpoint := range file {
		breakpoints = append(breakpoints, breakpoint.Breakpoint)
	}
	return breakpoints
}

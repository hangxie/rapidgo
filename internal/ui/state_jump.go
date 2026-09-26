package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/editor"
)

// resumeJump completes a jump that was waiting for the package listing.
func (state *shellState) resumeJump() {
	problem := state.pendingJump
	state.pendingJump = nil
	if problem != nil {
		state.jumpTo(*problem)
	}
}

// jumpToSelected opens the file a selected diagnostic names. Resolving a test
// path needs the package listing, so the jump may wait for it.
func (state *shellState) jumpToSelected() {
	view := state.activeView()
	if view == nil || view.selected >= len(view.lines) {
		return
	}
	problem := view.lines[view.selected].problem
	if problem == nil {
		state.message = "No problem on this line"
		return
	}
	state.jumpTo(*problem)
}

func (state *shellState) jumpTo(problem diagnostic.Diagnostic) {
	path, ok := state.resolvePath(problem)
	if !ok {
		if state.needsPackages(problem) {
			state.pendingJump = &problem
			state.withPackages(runIntentJump)
			return
		}
		state.message = "Cannot locate " + problem.Path
		return
	}
	state.openAt(path, problem.Line, problem.Column)
}

// needsPackages reports whether resolving this path waits on `go list`, which
// is how a package-relative test path becomes a directory.
func (state *shellState) needsPackages(problem diagnostic.Diagnostic) bool {
	return !state.packagesLoaded && problem.Package != "" && !filepath.IsAbs(problem.Path)
}

// resolvePath turns a reported path into one on disk. Compiler paths are
// relative to the project root; test paths are relative to their package's
// directory, which the listing supplies.
func (state *shellState) resolvePath(problem diagnostic.Diagnostic) (string, bool) {
	if problem.Path == "" {
		return "", false
	}
	if filepath.IsAbs(problem.Path) {
		return problem.Path, exists(problem.Path)
	}
	clean := filepath.FromSlash(strings.TrimPrefix(problem.Path, "./"))
	roots := []string{state.projectRoot}
	if directory, ok := state.packageDir(problem.Package); ok {
		// Test output names a file relative to its package, so that directory
		// wins over a file of the same name in the project root.
		if problem.Source == diagnostic.SourceTest {
			roots = []string{directory, state.projectRoot}
		} else {
			roots = append(roots, directory)
		}
	}
	for _, base := range roots {
		if candidate := filepath.Join(base, clean); exists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func (state *shellState) packageDir(importPath string) (string, bool) {
	for _, listed := range state.packages {
		if listed.ImportPath == importPath {
			return listed.Dir, true
		}
	}
	return "", false
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// openAt shows a file at a source position, loading it first when it is not
// already open. A position past the end of the file is clamped to it.
func (state *shellState) openAt(path string, line, column int) {
	state.pendingPosition = &jumpTarget{line: max(0, line-1), byteColumn: column}
	if state.document != nil && state.document.Path == path {
		state.applyPendingPosition()
		state.focusSeq++
		state.setFocus(focusEditor)
		return
	}
	if state.saving {
		state.message = "Save in progress; wait before opening another file"
		state.pendingPosition = nil
		return
	}
	if state.buffer != nil && state.buffer.Dirty() {
		state.openSeq++
		state.opening = false
		state.confirm = confirmOpen
		state.pendingPath = path
		state.message = "Unsaved changes: press D to discard and open " + path + ", Esc to cancel"
		return
	}
	state.focusSeq++
	state.queueOpen(path, false)
}

// jumpTarget is where a diagnostic pointed. The column stays as the tool
// reported it, a 1-based byte count, until the file is loaded and its text can
// turn that into the grapheme column the editor uses.
type jumpTarget struct {
	line       int // zero-based
	byteColumn int // one-based, zero when the tool gave none
}

// applyPendingPosition moves the caret to a jump target, clamping a line or
// column the file does not have and saying so.
func (state *shellState) applyPendingPosition() {
	target := state.pendingPosition
	state.pendingPosition = nil
	if target == nil || state.buffer == nil || state.document == nil {
		return
	}
	lines := state.buffer.Lines()
	at := editor.Position{Line: target.line}
	outside := false
	if at.Line >= len(lines) {
		at.Line, outside = len(lines)-1, true
	}
	at.Column, outside = graphemeColumn(lines[at.Line], target.byteColumn, outside)
	if err := state.buffer.MoveTo(at, false); err != nil {
		state.message = err.Error()
		return
	}
	state.message = describePosition(state.document.Path, at, outside)
}

// graphemeColumn converts a 1-based byte column into the grapheme-cluster
// column the editor uses. Go's token.Position counts bytes, so a multibyte
// character earlier in the line would otherwise push the caret too far right.
// A column inside a cluster rounds down to its start.
func graphemeColumn(line string, byteColumn int, outside bool) (int, bool) {
	offset := byteColumn - 1
	if offset <= 0 {
		return 0, outside
	}
	if offset > len(line) {
		outside = true
	}
	column := 0
	clusters := uniseg.NewGraphemes(line)
	for clusters.Next() {
		start, end := clusters.Positions()
		if offset <= start || offset < end {
			return column, outside
		}
		column++
	}
	return column, outside
}

func describePosition(path string, at editor.Position, outside bool) string {
	shown := path + ":" + strconv.Itoa(at.Line+1) + ":" + strconv.Itoa(at.Column+1)
	if outside {
		return shown + " (the reported position is past the end of the file)"
	}
	return shown
}

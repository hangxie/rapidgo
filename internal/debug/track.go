package debug

import (
	"path/filepath"
	"strings"
)

// Track moves the breakpoints in path to follow an edit from before to after, reporting whether any line changed.
func (b *Breakpoints) Track(path, before, after string) bool {
	path = filepath.Clean(path)
	file := b.files[path]
	if len(file) == 0 || before == after {
		return false
	}
	change := lineChangeBetween(before, after)
	moved := make([]entry, 0, len(file))
	changed := false
	for _, breakpoint := range file {
		line := change.move(breakpoint.Line)
		changed = changed || line != breakpoint.Line
		// Lines only collapse onto the change's first line, so a breakpoint already there wins.
		if len(moved) > 0 && moved[len(moved)-1].Line == line {
			continue
		}
		if line != breakpoint.Line {
			breakpoint.Line, breakpoint.revision = line, b.next()
		}
		moved = append(moved, breakpoint)
	}
	b.files[path] = moved
	return changed
}

// lineChange is one contiguous replacement described by one-based lines.
type lineChange struct {
	start       int  // line where the replaced text begins
	atLineStart bool // the replaced text begins at column zero
	pure        bool // nothing was removed
	removed     int  // line breaks removed
	inserted    int  // line breaks inserted
}

// lineChangeBetween finds the smallest single replacement that turns before into after.
func lineChangeBetween(before, after string) lineChange {
	prefix := 0
	for prefix < min(len(before), len(after)) && before[prefix] == after[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < min(len(before), len(after))-prefix && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	removed := before[prefix : len(before)-suffix]
	return lineChange{
		start:       1 + strings.Count(before[:prefix], "\n"),
		atLineStart: prefix == 0 || before[prefix-1] == '\n',
		pure:        removed == "",
		removed:     strings.Count(removed, "\n"),
		inserted:    strings.Count(after[prefix:len(after)-suffix], "\n"),
	}
}

func (c lineChange) move(line int) int {
	end := c.start + c.removed
	switch {
	case line < c.start:
		return line
	case line == c.start:
		// Inserting before a line's first character pushes the whole line down.
		if c.atLineStart && c.pure {
			return line + c.inserted
		}
		return line
	case line <= end:
		return c.start
	default:
		return line + c.inserted - c.removed
	}
}

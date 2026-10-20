package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrack(t *testing.T) {
	t.Parallel()

	const text = "a\nb\nc\nd\n"
	for _, test := range []struct {
		name    string
		before  string
		after   string
		lines   []int
		want    []int
		changed bool
	}{
		{name: "unchanged text", before: text, after: text, lines: []int{2}, want: []int{2}},
		{name: "edit within a line", before: text, after: "a\nbX\nc\nd\n", lines: []int{2, 3}, want: []int{2, 3}},
		{name: "new line after a breakpoint", before: text, after: "a\n\nb\nc\nd\n", lines: []int{1, 2, 3}, want: []int{1, 3, 4}, changed: true},
		{name: "lines pasted above a breakpoint", before: text, after: "a\nx\ny\nb\nc\nd\n", lines: []int{2}, want: []int{4}, changed: true},
		{name: "line split after its start", before: "a\nbb\nc\n", after: "a\nb\nb\nc\n", lines: []int{2, 3}, want: []int{2, 4}, changed: true},
		{name: "lines joined", before: text, after: "a\nbc\nd\n", lines: []int{3, 4}, want: []int{2, 3}, changed: true},
		{name: "joined breakpoints merge", before: text, after: "a\nbc\nd\n", lines: []int{2, 3}, want: []int{2}, changed: true},
		{name: "whole lines deleted", before: text, after: "a\nd\n", lines: []int{1, 3, 4}, want: []int{1, 2}, changed: true},
		{name: "everything deleted", before: text, after: "", lines: []int{1, 4}, want: []int{1}, changed: true},
		{name: "multi-line replacement", before: text, after: "a\nx\ny\nz\nd\n", lines: []int{3, 4}, want: []int{2, 5}, changed: true},
		{name: "UTF-8 text", before: "é\nb\n", after: "é世\n界\nb\n", lines: []int{1, 2}, want: []int{1, 3}, changed: true},
		// One replacement spans both regions, so breakpoints between them collapse onto its first line.
		{name: "separate regions", before: "a\nb\nc\nd\ne\n", after: "A\nb\nc\nd\nE\n", lines: []int{3, 4}, want: []int{1}, changed: true},
		{name: "undo restores lines", before: "a\n\nb\nc\nd\n", after: text, lines: []int{1, 3, 4}, want: []int{1, 2, 3}, changed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			set := NewBreakpoints()
			for _, line := range test.lines {
				set.Toggle("/p/main.go", line)
			}
			assert.Equal(t, test.changed, set.Track("/p/main.go", test.before, test.after))
			assert.Equal(t, test.want, lines(set.InFile("/p/main.go")))
		})
	}
}

func TestTrackKeepsSettingsAndOtherFiles(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 2)
	set.Toggle("/p/main.go", 3)
	set.Toggle("/p/other.go", 3)
	set.SetCondition("/p/main.go", 2, "i > 2", "")
	set.SetDisabled("/p/main.go", 3, true)

	assert.True(t, set.Track("/p/main.go", "a\nb\nc\nd\n", "a\nbc\nd\n"))
	assert.Equal(t, []Breakpoint{
		{Path: "/p/main.go", Line: 2, Condition: "i > 2"},
		{Path: "/p/other.go", Line: 3},
	}, set.All())

	assert.False(t, set.Track("/p/none.go", "a\n", "\na\n"))
	assert.Empty(t, set.InFile("/p/none.go"))
}

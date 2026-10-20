package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hangxie/rapidgo/internal/dap"
)

func lines(breakpoints []Breakpoint) []int {
	result := make([]int, 0, len(breakpoints))
	for _, breakpoint := range breakpoints {
		result = append(result, breakpoint.Line)
	}
	return result
}

func TestToggleAddsAndRemoves(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	assert.True(t, set.Toggle("/p/main.go", 9))
	assert.True(t, set.Toggle("/p/main.go", 4))
	assert.True(t, set.Toggle("/p/./b.go", 1))
	assert.Equal(t, []Breakpoint{
		{Path: "/p/b.go", Line: 1},
		{Path: "/p/main.go", Line: 4},
		{Path: "/p/main.go", Line: 9},
	}, set.All())
	assert.Equal(t, []string{"/p/b.go", "/p/main.go"}, set.Paths())

	assert.False(t, set.Toggle("/p/b.go", 1))
	assert.Equal(t, []string{"/p/main.go"}, set.Paths())
	assert.Empty(t, set.InFile("/p/b.go"))
	assert.Equal(t, []int{4, 9}, lines(set.InFile("/p/main.go")))
}

func TestInvalidLinesAreIgnored(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	assert.False(t, set.Toggle("/p/main.go", 0))
	assert.False(t, set.Toggle("/p/main.go", -3))
	assert.Empty(t, set.All())
}

func TestAtAndRemove(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	got, ok := set.At("/p/main.go", 4)
	assert.True(t, ok)
	assert.Equal(t, Breakpoint{Path: "/p/main.go", Line: 4}, got)
	_, ok = set.At("/p/main.go", 5)
	assert.False(t, ok)

	assert.False(t, set.Remove("/p/main.go", 5))
	assert.True(t, set.Remove("/p/main.go", 4))
	assert.False(t, set.Remove("/p/main.go", 4))
	assert.Empty(t, set.Paths())
}

func TestReturnedSlicesAreDetached(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	set.InFile("/p/main.go")[0].Line = 40
	set.All()[0].Condition = "x"
	assert.Equal(t, []Breakpoint{{Path: "/p/main.go", Line: 4}}, set.All())
}

func TestConditionsAndDisabling(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	assert.False(t, set.SetCondition("/p/main.go", 4, "i > 2", ""))
	assert.False(t, set.SetDisabled("/p/main.go", 4, true))

	set.Toggle("/p/main.go", 4)
	set.Toggle("/p/main.go", 9)
	set.Toggle("/p/main.go", 12)
	assert.True(t, set.SetCondition("/p/main.go", 4, "i > 2", "3"))
	assert.True(t, set.SetDisabled("/p/main.go", 9, true))

	got, _ := set.At("/p/main.go", 4)
	assert.Equal(t, Breakpoint{Path: "/p/main.go", Line: 4, Condition: "i > 2", HitCondition: "3"}, got)
	assert.Equal(t, []dap.SourceBreakpoint{
		{Line: 4, Condition: "i > 2", HitCondition: "3"},
		{Line: 12},
	}, set.Requests("/p/main.go"))
}

func TestRequestsForFileWithoutBreakpointsIsEmpty(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	requests := set.Requests("/p/main.go")
	assert.NotNil(t, requests, "an empty request clears the adapter's breakpoints")
	assert.Empty(t, requests)
}

func TestDisablingClearsVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	set.Resolve("/p/main.go", []dap.SourceBreakpoint{{Line: 4}}, []dap.Breakpoint{{Verified: true}})
	set.SetDisabled("/p/main.go", 4, true)
	got, _ := set.At("/p/main.go", 4)
	assert.False(t, got.Verified)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		requested []dap.SourceBreakpoint
		results   []dap.Breakpoint
		want      []Breakpoint
	}{
		{
			name:      "pairs results with requested lines",
			requested: []dap.SourceBreakpoint{{Line: 4}, {Line: 9}},
			results:   []dap.Breakpoint{{Verified: true, Line: 4}, {Line: 9, Message: "no code at line"}},
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9, Message: "no code at line"},
			},
		},
		{
			name:      "skips lines removed since the request",
			requested: []dap.SourceBreakpoint{{Line: 2}, {Line: 9}},
			results:   []dap.Breakpoint{{Verified: true}, {Verified: true}},
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4},
				{Path: "/p/main.go", Line: 9, Verified: true},
			},
		},
		{
			name:      "ignores missing and extra results",
			requested: []dap.SourceBreakpoint{{Line: 4}},
			results:   []dap.Breakpoint{{Verified: true}, {Verified: true}},
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9},
			},
		},
		{
			name:      "a failed request leaves breakpoints unverified",
			requested: []dap.SourceBreakpoint{{Line: 4}, {Line: 9}},
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4},
				{Path: "/p/main.go", Line: 9},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			set := NewBreakpoints()
			set.Toggle("/p/main.go", 4)
			set.Toggle("/p/main.go", 9)
			set.Resolve("/p/main.go", test.requested, test.results)
			assert.Equal(t, test.want, set.All())
		})
	}
}

func TestResolveReplacesEarlierVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	set.Resolve("/p/main.go", []dap.SourceBreakpoint{{Line: 4}}, []dap.Breakpoint{{Verified: true}})
	set.Resolve("/p/main.go", []dap.SourceBreakpoint{{Line: 4}}, []dap.Breakpoint{{Message: "stale binary"}})
	got, _ := set.At("/p/main.go", 4)
	assert.Equal(t, Breakpoint{Path: "/p/main.go", Line: 4, Message: "stale binary"}, got)
}

func TestClearVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/a.go", 1)
	set.Toggle("/p/b.go", 2)
	set.Resolve("/p/a.go", []dap.SourceBreakpoint{{Line: 1}}, []dap.Breakpoint{{Verified: true}})
	set.Resolve("/p/b.go", []dap.SourceBreakpoint{{Line: 2}}, []dap.Breakpoint{{Message: "no code"}})
	set.ClearVerification()
	assert.Equal(t, []Breakpoint{{Path: "/p/a.go", Line: 1}, {Path: "/p/b.go", Line: 2}}, set.All())
}

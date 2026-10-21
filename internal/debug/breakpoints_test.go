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
	request := set.Request("/p/./main.go")
	assert.Equal(t, "/p/main.go", request.Path)
	assert.Equal(t, []dap.SourceBreakpoint{
		{Line: 4, Condition: "i > 2", HitCondition: "3"},
		{Line: 12},
	}, request.Breakpoints)
}

func TestRequestForFileWithoutBreakpointsIsEmpty(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	request := set.Request("/p/main.go")
	assert.NotNil(t, request.Breakpoints, "an empty request clears the adapter's breakpoints")
	assert.Empty(t, request.Breakpoints)
}

func TestDisablingClearsVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	set.Resolve(set.Request("/p/main.go"), []dap.Breakpoint{{Verified: true}})
	set.SetDisabled("/p/main.go", 4, true)
	got, _ := set.At("/p/main.go", 4)
	assert.False(t, got.Verified)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	verified := []dap.Breakpoint{{Verified: true}, {Verified: true}}
	for _, test := range []struct {
		name    string
		change  func(*Breakpoints) // runs while the request is in flight
		results []dap.Breakpoint
		want    []Breakpoint
	}{
		{
			name:    "pairs results with requested breakpoints",
			results: []dap.Breakpoint{{Verified: true}, {Message: "no code at line"}},
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9, Message: "no code at line"},
			},
		},
		{
			name:    "ignores missing results",
			results: verified[:1],
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}, {Path: "/p/main.go", Line: 9}},
		},
		{
			name:    "ignores extra results",
			results: append(verified, dap.Breakpoint{Verified: true}),
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}, {Path: "/p/main.go", Line: 9, Verified: true}},
		},
		{
			name: "a failed request leaves breakpoints unverified",
			want: []Breakpoint{{Path: "/p/main.go", Line: 4}, {Path: "/p/main.go", Line: 9}},
		},
		{
			name:    "skips a breakpoint removed in flight",
			change:  func(set *Breakpoints) { set.Remove("/p/main.go", 9) },
			results: verified,
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}},
		},
		{
			name: "skips a breakpoint recreated in flight",
			change: func(set *Breakpoints) {
				set.Toggle("/p/main.go", 9)
				set.Toggle("/p/main.go", 9)
			},
			results: verified,
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}, {Path: "/p/main.go", Line: 9}},
		},
		{
			name:    "skips a breakpoint whose condition changed in flight",
			change:  func(set *Breakpoints) { set.SetCondition("/p/main.go", 9, "i > 2", "") },
			results: verified,
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9, Condition: "i > 2"},
			},
		},
		{
			name:    "skips a breakpoint disabled in flight",
			change:  func(set *Breakpoints) { set.SetDisabled("/p/main.go", 9, true) },
			results: verified,
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9, Disabled: true},
			},
		},
		{
			name: "skips a breakpoint disabled and enabled in flight",
			change: func(set *Breakpoints) {
				set.SetDisabled("/p/main.go", 9, true)
				set.SetDisabled("/p/main.go", 9, false)
			},
			results: verified,
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}, {Path: "/p/main.go", Line: 9}},
		},
		{
			name:    "skips a breakpoint moved by an edit in flight",
			change:  func(set *Breakpoints) { set.Track("/p/main.go", "1\n2\n3\n4\n5\n", "1\n2\n3\n4\nx\n5\n") },
			results: verified,
			want:    []Breakpoint{{Path: "/p/main.go", Line: 4, Verified: true}, {Path: "/p/main.go", Line: 10}},
		},
		{
			name:    "keeps breakpoints untouched in flight",
			change:  func(set *Breakpoints) { set.Toggle("/p/main.go", 20) },
			results: verified,
			want: []Breakpoint{
				{Path: "/p/main.go", Line: 4, Verified: true},
				{Path: "/p/main.go", Line: 9, Verified: true},
				{Path: "/p/main.go", Line: 20},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			set := NewBreakpoints()
			set.Toggle("/p/main.go", 4)
			set.Toggle("/p/main.go", 9)
			request := set.Request("/p/main.go")
			if test.change != nil {
				test.change(set)
			}
			set.Resolve(request, test.results)
			assert.Equal(t, test.want, set.All())
		})
	}
}

func TestResolveReplacesEarlierVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 4)
	set.Resolve(set.Request("/p/main.go"), []dap.Breakpoint{{Verified: true}})
	set.Resolve(set.Request("/p/main.go"), []dap.Breakpoint{{Message: "stale binary"}})
	got, _ := set.At("/p/main.go", 4)
	assert.Equal(t, Breakpoint{Path: "/p/main.go", Line: 4, Message: "stale binary"}, got)
}

func TestClearVerification(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/a.go", 1)
	set.Toggle("/p/b.go", 2)
	set.Resolve(set.Request("/p/a.go"), []dap.Breakpoint{{Verified: true}})
	set.Resolve(set.Request("/p/b.go"), []dap.Breakpoint{{Message: "no code"}})
	set.ClearVerification()
	assert.Equal(t, []Breakpoint{{Path: "/p/a.go", Line: 1}, {Path: "/p/b.go", Line: 2}}, set.All())
}

func TestRunToAddsAnUnconditionalBreakpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*Breakpoints)
		want  []dap.SourceBreakpoint
	}{
		{"empty file", func(*Breakpoints) {}, []dap.SourceBreakpoint{{Line: 5}}},
		{"between others", func(set *Breakpoints) {
			set.Toggle("/p/main.go", 3)
			set.Toggle("/p/main.go", 7)
		}, []dap.SourceBreakpoint{{Line: 3}, {Line: 5}, {Line: 7}}},
		{"already there", func(set *Breakpoints) { set.Toggle("/p/main.go", 5) }, []dap.SourceBreakpoint{{Line: 5}}},
		{"disabled there", func(set *Breakpoints) {
			set.Toggle("/p/main.go", 5)
			set.SetDisabled("/p/main.go", 5, true)
		}, []dap.SourceBreakpoint{{Line: 5}}},
		{"conditional there", func(set *Breakpoints) {
			set.Toggle("/p/main.go", 5)
			set.SetCondition("/p/main.go", 5, "i > 2", "")
		}, []dap.SourceBreakpoint{{Line: 5}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			set := NewBreakpoints()
			test.setup(set)
			request := set.RunTo("/p/./main.go", 5)
			assert.Equal(t, "/p/main.go", request.Path)
			assert.Equal(t, test.want, request.Breakpoints)
		})
	}
}

func TestRunToResultsLeaveTheTemporaryLineOut(t *testing.T) {
	t.Parallel()

	set := NewBreakpoints()
	set.Toggle("/p/main.go", 3)
	set.Toggle("/p/main.go", 5)
	set.SetCondition("/p/main.go", 5, "i > 2", "")
	set.Toggle("/p/main.go", 7)
	request := set.RunTo("/p/main.go", 5)
	set.Resolve(request, []dap.Breakpoint{{Verified: true}, {Verified: true}, {Verified: true}})
	var verified []bool
	for _, breakpoint := range set.InFile("/p/main.go") {
		verified = append(verified, breakpoint.Verified)
	}
	assert.Equal(t, []bool{true, false, true}, verified, "the conditional breakpoint was not sent")
}

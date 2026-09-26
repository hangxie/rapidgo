package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverityString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "info", Info.String())
	assert.Equal(t, "warning", Warning.String())
	assert.Equal(t, "error", Error.String())
	assert.Equal(t, "unknown", Severity(200).String())
}

// Each case is one line fed to a fresh parser, so these cover shape alone.
func TestParseLine(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		line string
		want *Diagnostic
	}{
		{
			name: "relative path with column",
			line: "internal/sub/sub.go:4:9: cannot use \"no\" as int value",
			want: &Diagnostic{Path: "internal/sub/sub.go", Line: 4, Column: 9, Severity: Error, Source: SourceCompile, Message: `cannot use "no" as int value`},
		},
		{
			name: "dot slash path",
			line: "./main.go:7:2: undefined: missing",
			want: &Diagnostic{Path: "./main.go", Line: 7, Column: 2, Severity: Error, Source: SourceCompile, Message: "undefined: missing"},
		},
		{
			name: "absolute path",
			line: "/home/dev/project/main.go:12:1: syntax error",
			want: &Diagnostic{Path: "/home/dev/project/main.go", Line: 12, Column: 1, Severity: Error, Source: SourceCompile, Message: "syntax error"},
		},
		{
			name: "windows path keeps its drive letter",
			line: `C:\work\project\main.go:3:5: undefined: x`,
			want: &Diagnostic{Path: `C:\work\project\main.go`, Line: 3, Column: 5, Severity: Error, Source: SourceCompile, Message: "undefined: x"},
		},
		{
			name: "no column",
			line: "main.go:9: missing return",
			want: &Diagnostic{Path: "main.go", Line: 9, Severity: Error, Source: SourceCompile, Message: "missing return"},
		},
		{
			name: "vet prefix",
			line: "vet: ./main.go:6:2: fmt.Printf format %d has arg s of wrong type string",
			want: &Diagnostic{Path: "./main.go", Line: 6, Column: 2, Severity: Warning, Source: SourceVet, Message: "fmt.Printf format %d has arg s of wrong type string"},
		},
		{
			name: "utf-8 path and message",
			line: "internal/界/文件.go:2:1: 未定义: 変数",
			want: &Diagnostic{Path: "internal/界/文件.go", Line: 2, Column: 1, Severity: Error, Source: SourceCompile, Message: "未定义: 変数"},
		},
		{name: "location with no message", line: "main.go:1:1: "},
		{name: "empty line"},
		{name: "plain text", line: "building..."},
		{name: "package header", line: "# example.com/m"},
		{name: "non-numeric line number", line: "main.go:abc: broken"},
		{name: "no message separator", line: "main.go:12:3:no space"},
		{name: "not a go file", line: "config.yaml:3:1: bad key"},
		{name: "timestamp from a program's log", line: "2026/09/26 03:07:07 starting up"},
		{name: "test verdict", line: "FAIL\texample.com/m/internal/sub\t0.177s"},
		{name: "package summary", line: "ok  \texample.com/m\t0.003s"},
		{name: "no test files", line: "?   \texample.com/m/internal/x\t[no test files]"},
		{name: "fail marker alone", line: "FAIL"},
		{name: "cannot load package", line: "can't load package: package .: no Go files"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parser := New(Tool)
			got, ok := parser.Line(test.line)
			if test.want == nil {
				assert.False(t, ok, "%q should not be a diagnostic", test.line)
				assert.Equal(t, Diagnostic{}, got)
				return
			}
			require.True(t, ok, "%q should be a diagnostic", test.line)
			assert.Equal(t, *test.want, got)
		})
	}
}

// go build names the package once, above the errors in it.
func TestParseAttachesThePackageHeader(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `# diagsample/internal/sub
internal/sub/sub.go:4:9: cannot use "no" (untyped string constant) as int value in return statement
# diagsample
./main.go:7:2: undefined: missing
`)
	require.Len(t, found, 2)
	assert.Equal(t, "diagsample/internal/sub", found[0].Package)
	assert.Equal(t, "internal/sub/sub.go", found[0].Path)
	assert.Equal(t, "diagsample", found[1].Package)
	assert.Equal(t, "./main.go", found[1].Path)
}

// go vet wraps the package name in brackets in one of its headers.
func TestParseAcceptsTheBracketedVetHeader(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, "# [diagsample]\nvet: ./main.go:7:2: undefined: missing\n")
	require.Len(t, found, 1)
	assert.Equal(t, "diagsample", found[0].Package)
	assert.Equal(t, SourceVet, found[0].Source)
	assert.Equal(t, Warning, found[0].Severity)
}

// Located lines inside a failing test are its evidence; elsewhere they are output.
func TestParseTestOutput(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `--- FAIL: TestAdd (0.00s)
    sub_test.go:6: about to check
    sub_test.go:8: Add(1, 2) = -1, want 3
--- FAIL: TestSub (0.00s)
    --- FAIL: TestSub/nested (0.00s)
        sub_test.go:14: boom 7
FAIL
FAIL	diagtest/internal/sub	0.177s
--- PASS: TestOK (0.00s)
    sub_test.go:19: fine
`)
	require.Len(t, found, 4)
	for _, reported := range found[:3] {
		assert.Equal(t, SourceTest, reported.Source)
		assert.Equal(t, Error, reported.Severity)
		assert.Zero(t, reported.Column, "test output carries no column")
		assert.Equal(t, "sub_test.go", reported.Path, "paths are relative to the package directory")
	}
	assert.Equal(t, 6, found[0].Line)
	assert.Equal(t, "about to check", found[0].Message)
	assert.Equal(t, 14, found[2].Line)

	// The passing test's log is output, not a failure, and the verdict line
	// before it cleared the package.
	assert.Equal(t, Info, found[3].Severity)
	assert.Equal(t, "fine", found[3].Message)
	assert.Empty(t, found[3].Package)
}

// A compile failure during go test is reported the same way go build does.
func TestParseTestCompileFailure(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `# diagtest/internal/sub [diagtest/internal/sub.test]
internal/sub/sub_test.go:5:2: undefined: Helper
FAIL	diagtest/internal/sub [build failed]
`)
	require.Len(t, found, 1)
	assert.Equal(t, SourceCompile, found[0].Source)
	assert.Equal(t, Error, found[0].Severity)
	assert.Equal(t, "internal/sub/sub_test.go", found[0].Path)
	assert.Equal(t, "diagtest/internal/sub", found[0].Package)
}

// A verdict line ends the package, so later errors are not misattributed.
func TestPackageDoesNotLeakPastAVerdict(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, "# example.com/m\n./a.go:1:1: first\nok  \texample.com/m\t0.1s\n./b.go:2:2: second\n")
	require.Len(t, found, 2)
	assert.Equal(t, "example.com/m", found[0].Package)
	assert.Empty(t, found[1].Package, "the verdict cleared the package")
}

func TestParseEmptyOutput(t *testing.T) {
	t.Parallel()

	assert.Empty(t, Parse(Tool, ""))
	assert.Empty(t, Parse(Tool, "\n\n\n"))
}

// A subtest's result governs only the lines indented inside it, as -v shows.
func TestSubtestResultDoesNotGovernTheParent(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `--- FAIL: TestParent (0.00s)
    parent_test.go:10: first failure
    --- PASS: TestParent/child (0.00s)
        parent_test.go:20: child log
    parent_test.go:30: second failure
`)
	require.Len(t, found, 3)
	assert.Equal(t, Error, found[0].Severity, "the parent failed")
	assert.Equal(t, Info, found[1].Severity, "inside a subtest that passed")
	assert.Equal(t, Error, found[2].Severity, "the parent again, not the subtest")
}

// The same nesting in the output a plain `go test ./...` produces.
func TestFailingSubtestThenMoreParentOutput(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `--- FAIL: TestParent (0.00s)
    a_test.go:6: first failure
    --- FAIL: TestParent/child (0.00s)
        a_test.go:8: child failure
        a_test.go:9: child log
    a_test.go:11: second failure
FAIL
FAIL	nested	0.132s
`)
	require.Len(t, found, 4)
	for index, reported := range found {
		assert.Equal(t, Error, reported.Severity, "line %d", index)
	}
	assert.Equal(t, 11, found[3].Line)
}

// A deeply nested marker closes every level it ended.
func TestNestingClosesSeveralLevels(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `--- PASS: TestA (0.00s)
    --- FAIL: TestA/b (0.00s)
        --- FAIL: TestA/b/c (0.00s)
            a_test.go:1: deepest
    a_test.go:2: back at TestA
`)
	require.Len(t, found, 2)
	assert.Equal(t, Error, found[0].Severity)
	assert.Equal(t, Info, found[1].Severity, "TestA passed")
}

// log.Lshortfile writes "main.go:7: msg", which go run must not record.
func TestProgramOutputIsNotADiagnostic(t *testing.T) {
	t.Parallel()

	assert.Empty(t, Parse(Program, "main.go:7: server started\n"))
	assert.Empty(t, Parse(Program, "/home/dev/app/main.go:42: listening on :8080\n"))

	// The same lines from go build are diagnostics.
	require.Len(t, Parse(Tool, "main.go:7: server started\n"), 1)
}

// go run still reports a compile failure, which arrives under a header.
func TestRunCompileFailureIsStillADiagnostic(t *testing.T) {
	t.Parallel()

	found := Parse(Program, `# command-line-arguments
./main.go:7:2: undefined: missing
`)
	require.Len(t, found, 1)
	assert.Equal(t, "./main.go", found[0].Path)
	assert.Equal(t, "command-line-arguments", found[0].Package)
	assert.Equal(t, Error, found[0].Severity)
}

// A compiler error's continuation lines must not end the toolchain's block.
func TestRunMultilineCompileFailure(t *testing.T) {
	t.Parallel()

	found := Parse(Program, "# multi2\n"+
		"./main.go:6:8: not enough arguments in call to t\n"+
		"\thave (number)\n"+
		"\twant (int, int)\n"+
		"./main.go:7:14: cannot use \"no\" (untyped string constant) as int value\n")
	require.Len(t, found, 2)
	assert.Equal(t, 6, found[0].Line)
	assert.Equal(t, 7, found[1].Line)
	for _, reported := range found {
		assert.Equal(t, "multi2", reported.Package)
		assert.Equal(t, Error, reported.Severity)
	}
}

// A module preamble must not stop the header after it being recognized.
func TestRunPreambleBeforeTheHeader(t *testing.T) {
	t.Parallel()

	found := Parse(Program, `go: downloading example.com/dep v1.2.3
# command-line-arguments
./main.go:7:2: undefined: missing
`)
	require.Len(t, found, 1)
	assert.Equal(t, "command-line-arguments", found[0].Package)
}

// A header means the program never started, so the whole run is the toolchain's.
func TestHeaderHoldsForTheRestOfTheRun(t *testing.T) {
	t.Parallel()

	found := Parse(Program, `# example.com/m/internal/sub
internal/sub/sub.go:4:9: cannot use "no" as int value
# example.com/m
./main.go:7:2: undefined: missing
`)
	require.Len(t, found, 2)
	assert.Equal(t, "example.com/m/internal/sub", found[0].Package)
	assert.Equal(t, "example.com/m", found[1].Package)
}

// go test names the test binary's package in a bracketed suffix.
func TestPackageHeaderWithATestBinarySuffix(t *testing.T) {
	t.Parallel()

	found := Parse(Tool, `# diagtest/internal/sub [diagtest/internal/sub.test]
internal/sub/sub_test.go:5:2: undefined: Helper
`)
	require.Len(t, found, 1)
	assert.Equal(t, "diagtest/internal/sub", found[0].Package)
	assert.Equal(t, SourceCompile, found[0].Source)
}

// A verdict line names the package the test output above it belongs to.
func TestTakeVerdict(t *testing.T) {
	t.Parallel()

	parser := New(Tool)
	assert.Empty(t, parser.TakeVerdict())

	_, ok := parser.Line("FAIL\texample.com/m/internal/sub\t0.177s")
	assert.False(t, ok)
	assert.Equal(t, "example.com/m/internal/sub", parser.TakeVerdict())
	assert.Empty(t, parser.TakeVerdict(), "taking it clears it")

	_, ok = parser.Line("ok  \texample.com/m\t0.003s")
	assert.False(t, ok)
	assert.Equal(t, "example.com/m", parser.TakeVerdict())

	// The bare marker at the end of a run names nothing.
	_, ok = parser.Line("FAIL")
	assert.False(t, ok)
	assert.Empty(t, parser.TakeVerdict())
}

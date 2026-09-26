package diagnostic

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// "# pkg", vet's "# [pkg]", and the test binary's "# pkg [pkg.test]".
	packageHeader = regexp.MustCompile(`^# (?:\[([^\[\]\s]+)]|([^\[\]\s]+))(?: \[[^\[\]]*])?$`)
	// A .go path, a line, an optional column, and a message. Requiring .go
	// keeps timestamps and other "12:34:" text out.
	located = regexp.MustCompile(`^(.*\.go):(\d+)(?::(\d+))?: (.*)$`)
	// "--- FAIL: TestX (0.00s)", indented one level per subtest.
	testResult = regexp.MustCompile(`^(\s*)--- (FAIL|PASS|SKIP|BENCH): `)
	// The per-package verdict that ends a package's test output.
	testVerdict = regexp.MustCompile(`^(ok|FAIL|\?)\s`)
)

// Origin says who writes a command's output.
type Origin uint8

const (
	// Tool output comes only from the Go toolchain, as go build and go test
	// produce.
	Tool Origin = iota
	// Program output also carries whatever the program prints, as go run does.
	Program
)

// testFrame is one open "--- RESULT" marker and the column it started at.
type testFrame struct {
	indent  int
	failing bool
}

// Parser converts output lines into diagnostics. It is stateful because Go
// names a package, and marks a test, once above the lines it covers, so feed
// it lines in order. The zero value parses toolchain output.
type Parser struct {
	origin    Origin
	pkg       string
	frames    []testFrame
	sawHeader bool
}

// New returns a parser for one run of one command.
func New(origin Origin) Parser { return Parser{origin: origin} }

// Line converts one output line. It reports false for anything that is not a
// located problem, which the caller keeps as plain output.
func (p *Parser) Line(text string) (Diagnostic, bool) {
	line := strings.TrimRight(text, " \t")
	if header := packageHeader.FindStringSubmatch(line); header != nil {
		p.pkg = header[1] + header[2] // Exactly one alternative matched.
		p.frames, p.sawHeader = nil, true
		return Diagnostic{}, false
	}
	if result := testResult.FindStringSubmatch(line); result != nil {
		p.enterTest(len(result[1]), result[2] == "FAIL")
		return Diagnostic{}, false
	}
	if testVerdict.MatchString(line) {
		p.pkg, p.frames = "", nil
		return Diagnostic{}, false
	}

	body := strings.TrimLeft(line, " \t")
	indent := len(line) - len(body)
	source := SourceCompile
	if rest, found := strings.CutPrefix(body, "vet: "); found {
		body, source = rest, SourceVet
	} else if indent > 0 {
		source = SourceTest // Only test output indents its located lines.
	}
	match := located.FindStringSubmatch(body)
	if match == nil {
		return Diagnostic{}, false
	}
	// Without a package header a go run line is the program's own, such as the
	// "main.go:7: msg" log.Lshortfile writes. A header means compilation
	// failed and the program never started, so it holds for the whole run.
	if p.origin == Program && !p.sawHeader {
		return Diagnostic{}, false
	}
	return Diagnostic{
		Path:     match[1],
		Line:     number(match[2]),
		Column:   number(match[3]),
		Severity: p.severity(source, indent),
		Source:   source,
		Package:  p.pkg,
		Message:  match[4],
	}, true
}

// enterTest opens a marker, closing any that ended at or inside its column.
func (p *Parser) enterTest(indent int, failing bool) {
	p.closeTo(indent)
	p.frames = append(p.frames, testFrame{indent: indent, failing: failing})
}

// closeTo drops the markers at or deeper than indent, which have ended.
func (p *Parser) closeTo(indent int) {
	for len(p.frames) > 0 && p.frames[len(p.frames)-1].indent >= indent {
		p.frames = p.frames[:len(p.frames)-1]
	}
}

// severity grades a located line. Plain test output cannot tell a t.Log from a
// t.Errorf, so anything inside a failing test counts as evidence of it.
func (p *Parser) severity(source string, indent int) Severity {
	if source == SourceVet {
		return Warning
	}
	if source != SourceTest {
		return Error
	}
	// A line belongs to the innermost marker that started left of it, so a
	// parent's later output is not graded by a subtest that has ended.
	p.closeTo(indent)
	if len(p.frames) > 0 && p.frames[len(p.frames)-1].failing {
		return Error
	}
	return Info
}

func number(text string) int {
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return value
}

// Parse converts a whole block of output at once.
func Parse(origin Origin, output string) []Diagnostic {
	parser := New(origin)
	var found []Diagnostic
	for _, line := range strings.Split(output, "\n") {
		if reported, ok := parser.Line(line); ok {
			found = append(found, reported)
		}
	}
	return found
}

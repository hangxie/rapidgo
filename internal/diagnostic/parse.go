package diagnostic

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// "# pkg", vet's "# [pkg]", and the test binary's "# pkg [pkg.test]".
	packageHeader = regexp.MustCompile(`^# (?:\[([^\[\]\s]+)]|([^\[\]\s]+))(?: \[[^\[\]]*])?$`)
	// A .go path, a line, an optional column, and a message.
	located = regexp.MustCompile(`^(.*\.go):(\d+)(?::(\d+))?: (.*)$`)
	// "--- FAIL: TestX (0.00s)", indented one level per subtest.
	testResult = regexp.MustCompile(`^(\s*)--- (FAIL|PASS|SKIP|BENCH): `)
	// The per-package verdict that ends a package's test output.
	testVerdict = regexp.MustCompile(`^(ok|FAIL|\?)\s`)
)

// Origin says who writes a command's output.
type Origin uint8

const (
	// Tool output comes only from the Go toolchain, as go build and go test do.
	Tool Origin = iota
	// Program output also carries whatever the program prints, as go run does.
	Program
)

// testFrame is one open "--- RESULT" marker and the column it started at.
type testFrame struct {
	indent  int
	failing bool
}

// Parser converts output lines, in order, into diagnostics.
type Parser struct {
	origin    Origin
	pkg       string
	frames    []testFrame
	sawHeader bool
	verdict   string
}

// New returns a parser for one run of one command.
func New(origin Origin) Parser { return Parser{origin: origin} }

// Line converts one output line, reporting false for anything unlocated.
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
		// "FAIL\texample.com/m/internal/sub\t0.177s" names the package the
		// test output above belongs to.
		if fields := strings.Fields(line); len(fields) >= 2 {
			p.verdict = fields[1]
		}
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
	if !p.toolchainWrote() {
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

// toolchainWrote reports whether a located line is the toolchain's own.
func (p *Parser) toolchainWrote() bool { return p.origin == Tool || p.sawHeader }

// TakeVerdict returns and forgets the package a verdict line named.
func (p *Parser) TakeVerdict() string {
	named := p.verdict
	p.verdict = ""
	return named
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

// severity grades a located line by its source and enclosing test.
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

// Package diagnostic turns Go command output into located problems.
package diagnostic

// Severity is how much a diagnostic matters.
type Severity uint8

const (
	// Info is located output that is not a failure, such as a passing test's log.
	Info Severity = iota
	// Warning is a static-analysis finding, which is what go vet reports.
	Warning
	// Error is a compiler error or the evidence of a failing test.
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return "unknown"
}

// Sources name the tool a diagnostic came from.
const (
	SourceCompile = "compile"
	SourceVet     = "vet"
	SourceTest    = "test"
)

// Diagnostic is one located problem, with the path as the tool wrote it.
type Diagnostic struct {
	Path     string
	Line     int // 1-based, zero when the tool gave none
	Column   int // 1-based byte count, zero when the tool gave none
	Severity Severity
	Source   string
	Package  string // import path, when the output named one
	Message  string
}

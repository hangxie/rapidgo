package jobs

import (
	"encoding/json"
	"strings"
)

// testOutput converts go test JSON output while preserving unexpected lines.
func (m *Manager) testOutput(id uint64, stream Stream, line string) {
	var record struct {
		Action     string
		Package    string
		ImportPath string
		Test       string
		Output     string
		OutputType string
	}
	if stream != Stdout || json.Unmarshal([]byte(line), &record) != nil || record.Action == "" {
		m.emit(Event{ID: id, Kind: Test, Type: Output, Line: line, Stream: stream})
		return
	}
	switch record.Action {
	case "fail":
		if record.Test != "" {
			m.emit(Event{ID: id, Kind: Test, Type: TestFailed, TestPackage: record.Package, TestName: record.Test})
		}
		return
	case "start", "run", "pause", "cont", "pass", "skip", "bench", "build-start", "build-fail", "build-pass":
		return
	case "output", "build-output":
	default:
		m.emit(Event{ID: id, Kind: Test, Type: Output, Line: line, Stream: stream})
		return
	}
	if record.Output == "" {
		return
	}
	if record.ImportPath != "" {
		record.Package = record.ImportPath
	}
	for _, part := range strings.Split(strings.TrimSuffix(record.Output, "\n"), "\n") {
		m.emit(Event{
			ID: id, Kind: Test, Type: Output, Line: strings.TrimSuffix(part, "\r"), Stream: stream,
			TestPackage: record.Package, TestName: record.Test, TestOutputType: record.OutputType, StructuredTest: true,
		})
	}
}

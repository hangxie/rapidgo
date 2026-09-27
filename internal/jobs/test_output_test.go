package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructuredTestOutput(t *testing.T) {
	t.Parallel()
	manager := NewManager(t.TempDir())
	defer manager.Close()
	for _, line := range []string{
		`{"Action":"run","Package":"example/m","Test":"TestOne"}`,
		`{"Action":"output","Package":"example/m","Test":"TestOne","Output":"    f_test.go:4: log\n"}`,
		`{"Action":"output","Package":"example/m","Test":"TestOne","Output":"    f_test.go:5: error\n","OutputType":"error"}`,
		`{"Action":"build-output","ImportPath":"example/m","Output":"./f.go:2: broken\n"}`,
		`{"Action":"fail","Package":"example/m","Test":"TestOne"}`,
		`not json`,
	} {
		manager.testOutput(1, Stdout, line)
	}
	var got []Event
	for range 5 {
		got = append(got, <-manager.Events())
	}
	assert.Equal(t, []string{"    f_test.go:4: log", "    f_test.go:5: error", "./f.go:2: broken", "not json"}, []string{got[0].Line, got[1].Line, got[2].Line, got[4].Line})
	assert.True(t, got[0].StructuredTest)
	assert.Equal(t, "example/m", got[0].TestPackage)
	assert.Equal(t, "TestOne", got[0].TestName)
	assert.Equal(t, "error", got[1].TestOutputType)
	assert.Equal(t, "example/m", got[2].TestPackage)
	assert.Equal(t, TestFailed, got[3].Type)
	assert.Equal(t, "TestOne", got[3].TestName)
	assert.False(t, got[4].StructuredTest)
}

func TestTestOutputPreservesUnexpectedRecords(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		stream Stream
		line   string
	}{
		{name: "stderr JSON", stream: Stderr, line: `{"Action":"output","Output":"message\n"}`},
		{name: "malformed JSON", stream: Stdout, line: `{"Action":`},
		{name: "missing action", stream: Stdout, line: `{"Output":"message\n"}`},
		{name: "unknown action", stream: Stdout, line: `{"Action":"future","Output":"message\n"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manager := NewManager(t.TempDir())
			defer manager.Close()
			manager.testOutput(7, test.stream, test.line)
			got := <-manager.Events()
			assert.Equal(t, Event{ID: 7, Kind: Test, Type: Output, Line: test.line, Stream: test.stream}, got)
		})
	}
}

func TestTestOutputSplitsLinesAndSkipsEmptyOutput(t *testing.T) {
	t.Parallel()

	manager := NewManager(t.TempDir())
	defer manager.Close()
	manager.testOutput(9, Stdout, `{"Action":"output","Package":"fallback","ImportPath":"example.com/m","Test":"TestOne","Output":"first\r\nsecond\n","OutputType":"error"}`)
	manager.testOutput(9, Stdout, `{"Action":"output","Output":""}`)
	first := <-manager.Events()
	second := <-manager.Events()
	assert.Equal(t, "first", first.Line)
	assert.Equal(t, "second", second.Line)
	for _, event := range []Event{first, second} {
		assert.Equal(t, "example.com/m", event.TestPackage)
		assert.Equal(t, "TestOne", event.TestName)
		assert.Equal(t, "error", event.TestOutputType)
		assert.True(t, event.StructuredTest)
	}
	select {
	case event := <-manager.Events():
		t.Fatalf("unexpected event: %+v", event)
	default:
	}
}

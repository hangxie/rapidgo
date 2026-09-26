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
		`{"Action":"output","Package":"example/m","Output":"    f_test.go:4: log\n"}`,
		`{"Action":"output","Package":"example/m","Output":"    f_test.go:5: error\n","OutputType":"error"}`,
		`{"Action":"build-output","ImportPath":"example/m","Output":"./f.go:2: broken\n"}`,
		`{"Action":"fail","Package":"example/m"}`,
		`not json`,
	} {
		manager.testOutput(1, Stdout, line)
	}
	var got []Event
	for range 4 {
		got = append(got, <-manager.Events())
	}
	assert.Equal(t, []string{"    f_test.go:4: log", "    f_test.go:5: error", "./f.go:2: broken", "not json"}, []string{got[0].Line, got[1].Line, got[2].Line, got[3].Line})
	assert.True(t, got[0].StructuredTest)
	assert.Equal(t, "example/m", got[0].TestPackage)
	assert.Equal(t, "error", got[1].TestOutputType)
	assert.Equal(t, "example/m", got[2].TestPackage)
	assert.False(t, got[3].StructuredTest)
}

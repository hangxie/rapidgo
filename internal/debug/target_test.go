package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testSource = `package demo

import "testing"

// TestAdd checks addition.
func TestAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("math")
	}
}

func BenchmarkAdd(b *testing.B) {}

func ExampleAdd() {
	// Output:
}

func FuzzAdd(f *testing.F) {}

func Testify(t *testing.T) {}

func Test(t *testing.T) {}

func Test_snake(t *testing.T) {}

func TestÜber(t *testing.T) {}

func (s suite) TestMethod(t *testing.T) {}

func helper() {}

func TestMain(m *testing.M) {}

func TestHelper(x int) {}

func ExampleQuiet() {
	println("x")
}

func ExampleUnordered() {
	// Unordered output:
}

func TestResult(t *testing.T) int { return 0 }
`

func TestFindTest(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		path string
		line int
		want Test
		ok   bool
	}{
		{name: "func line", path: "/p/demo_test.go", line: 6, want: Test{Dir: "/p", Name: "TestAdd"}, ok: true},
		{name: "body", path: "/p/demo_test.go", line: 8, want: Test{Dir: "/p", Name: "TestAdd"}, ok: true},
		{name: "closing brace", path: "/p/demo_test.go", line: 10, want: Test{Dir: "/p", Name: "TestAdd"}, ok: true},
		{name: "doc comment", path: "/p/demo_test.go", line: 5},
		{name: "between functions", path: "/p/demo_test.go", line: 11},
		{name: "benchmark", path: "/p/demo_test.go", line: 12, want: Test{Dir: "/p", Name: "BenchmarkAdd"}, ok: true},
		{name: "example", path: "/p/demo_test.go", line: 15, want: Test{Dir: "/p", Name: "ExampleAdd"}, ok: true},
		{name: "fuzz", path: "/p/demo_test.go", line: 18, want: Test{Dir: "/p", Name: "FuzzAdd"}, ok: true},
		{name: "lowercase after prefix", path: "/p/demo_test.go", line: 20},
		{name: "bare prefix", path: "/p/demo_test.go", line: 22, want: Test{Dir: "/p", Name: "Test"}, ok: true},
		{name: "underscore after prefix", path: "/p/demo_test.go", line: 24, want: Test{Dir: "/p", Name: "Test_snake"}, ok: true},
		{name: "non-ASCII uppercase", path: "/p/demo_test.go", line: 26, want: Test{Dir: "/p", Name: "TestÜber"}, ok: true},
		{name: "method", path: "/p/demo_test.go", line: 28},
		{name: "helper", path: "/p/demo_test.go", line: 30},
		{name: "not a test file", path: "/p/demo.go", line: 6},
		{name: "TestMain", path: "/p/demo_test.go", line: 32},
		{name: "wrong parameter", path: "/p/demo_test.go", line: 34},
		{name: "example without output", path: "/p/demo_test.go", line: 37},
		{name: "unordered output", path: "/p/demo_test.go", line: 41, want: Test{Dir: "/p", Name: "ExampleUnordered"}, ok: true},
		{name: "test with a result", path: "/p/demo_test.go", line: 44},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := FindTest(test.path, testSource, test.line)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestFindTestWithDotImportedTesting(t *testing.T) {
	t.Parallel()

	source := "package demo\n\nimport . \"testing\"\n\nfunc TestAdd(t *T) {}\n\nfunc BenchmarkAdd(b *B) {}\n\nfunc TestWrong(t *B) {}\n"
	got, ok := FindTest("/p/demo_test.go", source, 5)
	assert.True(t, ok)
	assert.Equal(t, Test{Dir: "/p", Name: "TestAdd"}, got)
	got, ok = FindTest("/p/demo_test.go", source, 7)
	assert.True(t, ok)
	assert.Equal(t, Test{Dir: "/p", Name: "BenchmarkAdd"}, got)
	_, ok = FindTest("/p/demo_test.go", source, 9)
	assert.False(t, ok)
}

func TestFindTestInBrokenSource(t *testing.T) {
	t.Parallel()

	source := "package demo\n\nfunc TestEdit(t *testing.T) {\n\tif x {\n}\n\nfunc TestNext(t *testing.T) {}\n"
	got, ok := FindTest("/p/demo_test.go", source, 4)
	assert.True(t, ok, "an unsaved syntax error elsewhere should not hide the test")
	assert.Equal(t, "TestEdit", got.Name)
	_, ok = FindTest("/p/demo_test.go", "not go", 1)
	assert.False(t, ok)
}

func TestTestArgs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"-test.run", "^TestAdd$"}, Test{Name: "TestAdd"}.Args())
	assert.Equal(t, []string{"-test.run", "^FuzzAdd$"}, Test{Name: "FuzzAdd"}.Args())
	assert.Equal(t, []string{"-test.run", "^ExampleAdd$"}, Test{Name: "ExampleAdd"}.Args())
	assert.Equal(t, []string{"-test.run", "^$", "-test.bench", "^BenchmarkAdd$"}, Test{Name: "BenchmarkAdd"}.Args())
}

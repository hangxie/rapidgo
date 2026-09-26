package jobs

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		request Request
		kind    string
		args    []string
		command string
	}{
		{"build", Request{Kind: Build}, "build", []string{"build", "./..."}, "go build ./..."},
		{"test", Request{Kind: Test}, "test", []string{"test", "./..."}, "go test ./..."},
		{"run root", Request{Kind: Run}, "run", []string{"run", "."}, "go run ."},
		{"run package", Request{Kind: Run, Target: "./cmd/rapidgo"}, "run", []string{"run", "./cmd/rapidgo"}, "go run ./cmd/rapidgo"},
		{"unknown", Request{Kind: kindCount}, "unknown", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.kind, test.request.Kind.String())
			assert.Equal(t, test.args, test.request.Args())
			assert.Equal(t, test.command, test.request.Command())
		})
	}
	// A target is ignored by the kinds that always cover the whole module.
	assert.Equal(t, "go build ./...", Request{Kind: Build, Target: "./cmd/x"}.Command())
}

func TestDecodePackages(t *testing.T) {
	t.Parallel()

	const listing = `{"Dir":"/p","ImportPath":"example.com/m","Name":"m"}
{"Dir":"/p/cmd/worker","ImportPath":"example.com/m/cmd/worker","Name":"main"}
{"Dir":"/p/cmd/admin","ImportPath":"example.com/m/cmd/admin","Name":"main"}
{"Dir":"/p/internal/ui","ImportPath":"example.com/m/internal/ui","Name":"ui"}
{"ImportPath":"example.com/m/broken","Name":"main"}
`
	packages, err := decodePackages([]byte(listing))
	require.NoError(t, err)
	// Sorted by import path; the entry without a directory is dropped.
	require.Len(t, packages, 4)
	assert.Equal(t, "example.com/m", packages[0].ImportPath)
	assert.Equal(t, "example.com/m/cmd/admin", packages[1].ImportPath)
	assert.Equal(t, "/p/cmd/worker", packages[2].Dir)
	assert.Equal(t, "ui", packages[3].Name, "packages that cannot be run are kept for resolving paths")

	empty, err := decodePackages(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	_, err = decodePackages([]byte("{not json}"))
	assert.ErrorContains(t, err, "read go list output")
}

func TestListError(t *testing.T) {
	t.Parallel()

	assert.ErrorContains(t, listError(assert.AnError, ""), "go list: ")
	err := listError(assert.AnError, "go.mod file not found\nmore detail\n")
	assert.ErrorContains(t, err, "go list: go.mod file not found: ")
	assert.NotContains(t, err.Error(), "more detail")
}

func TestStateAndStream(t *testing.T) {
	t.Parallel()

	for state, name := range map[State]string{
		Pending: "pending", Running: "running", Succeeded: "succeeded",
		Failed: "failed", Cancelled: "cancelled", State(200): "unknown",
	} {
		assert.Equal(t, name, state.String())
	}
	assert.False(t, Pending.Done())
	assert.False(t, Running.Done())
	assert.True(t, Succeeded.Done())
	assert.True(t, Failed.Done())
	assert.True(t, Cancelled.Done())
	assert.Equal(t, "stdout", Stdout.String())
	assert.Equal(t, "stderr", Stderr.String())
}

func TestLineWriter(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		writes []string
		want   []string
	}{
		{"empty", nil, nil},
		{"single line", []string{"one\n"}, []string{"one"}},
		{"split write", []string{"on", "e\ntw", "o\n"}, []string{"one", "two"}},
		{"trailing line", []string{"one\ntwo"}, []string{"one", "two"}},
		{"blank lines", []string{"\n\na\n"}, []string{"", "", "a"}},
		{"carriage returns", []string{"one\r\ntwo\r\n"}, []string{"one", "two"}},
		{"utf8", []string{"日本語 ✓\n"}, []string{"日本語 ✓"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var lines []string
			writer := &lineWriter{emit: func(line string) { lines = append(lines, line) }}
			for _, write := range test.writes {
				count, err := writer.Write([]byte(write))
				require.NoError(t, err)
				assert.Equal(t, len(write), count)
			}
			writer.flush()
			writer.flush() // Flushing twice must not repeat the last line.
			assert.Equal(t, test.want, lines)
		})
	}
}

func TestLineWriterSplitsUnboundedLine(t *testing.T) {
	t.Parallel()

	var lines []string
	writer := &lineWriter{emit: func(line string) { lines = append(lines, line) }}
	count, err := writer.Write([]byte(strings.Repeat("x", maxLineBytes+5)))
	require.NoError(t, err)
	assert.Equal(t, maxLineBytes+5, count)
	require.Len(t, lines, 1)
	assert.Len(t, lines[0], maxLineBytes)
	writer.flush()
	require.Len(t, lines, 2)
	assert.Equal(t, strings.Repeat("x", 5), lines[1])
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ name, output, want string }{
		{"standard", "go version go1.26.0 linux/amd64\n", "go1.26.0"},
		{"extra lines", "go version go1.26.0 darwin/arm64\nnoise\n", "go1.26.0"},
		{"devel", "go version devel go1.27-abc darwin/arm64", "devel"},
		{"unexpected", "something else\n", "something else"},
		{"empty", "", ""},
		{"short", "go version\n", "go version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, parseVersion(test.output))
		})
	}
}

func TestToolchainDescribe(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "go not found", Toolchain{}.Describe())
	assert.Equal(t, "/usr/bin/go", Toolchain{Path: "/usr/bin/go"}.Describe())
	assert.Equal(t, "go1.26.0 (/usr/bin/go)", Toolchain{Path: "/usr/bin/go", Version: "go1.26.0"}.Describe())
}

func TestDetectMissingExecutable(t *testing.T) {
	t.Parallel()

	_, err := detect(t.Context(), func(string) (string, error) { return "", assert.AnError })
	assert.ErrorContains(t, err, "locate go executable")
}

func TestDetectVersionFailure(t *testing.T) {
	t.Parallel()

	missing := t.TempDir() + "/not-a-go-executable"
	tool, err := detect(t.Context(), func(string) (string, error) { return missing, nil })
	assert.Equal(t, missing, tool.Path)
	assert.ErrorContains(t, err, "go version")
}

func TestDetectInstalledGo(t *testing.T) {
	t.Parallel()

	tool, err := Detect(context.Background())
	require.NoError(t, err, "the test environment must provide a go executable")
	assert.NotEmpty(t, tool.Path)
	assert.True(t, strings.HasPrefix(tool.Version, "go"), "unexpected version %q", tool.Version)
}

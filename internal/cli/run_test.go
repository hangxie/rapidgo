package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/buildinfo"
	"github.com/hangxie/rapidgo/internal/i18n"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantOutput string
		wantError  string
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0, wantOutput: "Usage: rapidgo"},
		{name: "too many paths", args: []string{"one", "two"}, wantCode: 2, wantError: "unexpected argument two"},
		{name: "unknown option", args: []string{"--unknown"}, wantCode: 2, wantError: "unknown flag --unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := Run(test.args, &stdout, &stderr)
			require.Equal(t, test.wantCode, code)
			assert.Contains(t, stdout.String(), test.wantOutput)
			assert.Contains(t, stderr.String(), test.wantError)
		})
	}
}

func TestRunVersion(t *testing.T) {
	originalVersion, originalCommit, originalDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = originalVersion, originalCommit, originalDate
	})
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "v0.1.0", "abc123", "2026-09-23"

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"--version"}, &stdout, &stderr), stderr.String())
	assert.Equal(t, "rapidgo v0.1.0 (commit abc123, built 2026-09-23)\n", stdout.String())
}

func TestRunVersionWithoutBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = originalVersion, originalCommit, originalDate
	})
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "v0.1.0", "unknown", "unknown"

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"--version"}, &stdout, &stderr), stderr.String())
	assert.Equal(t, "rapidgo v0.1.0\n", stdout.String())
}

func TestRunDirectory(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	var stdout, stderr bytes.Buffer
	var launchedRoot string
	launch := func(root string) error {
		launchedRoot = root
		return nil
	}
	require.Equal(t, 0, run([]string{directory}, &stdout, &stderr, launch), stderr.String())
	assert.Equal(t, filepath.Clean(directory), launchedRoot)
	assert.Empty(t, stdout.String())
}

func TestRunReportsTerminalFailure(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	launch := func(string) error { return errors.New("no usable terminal") }
	require.Equal(t, 1, run([]string{t.TempDir()}, &stdout, &stderr, launch))
	assert.Contains(t, stderr.String(), "start terminal UI: no usable terminal")
}

func TestRunRejectsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o600))
	var stdout, stderr bytes.Buffer
	require.Equal(t, 1, Run([]string{path}, &stdout, &stderr))
	assert.Contains(t, stderr.String(), "not a directory")
}

func TestRunRejectsMissingDirectoryWithoutLaunching(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	launched := false
	code := run([]string{path}, &stdout, &stderr, func(string) error {
		launched = true
		return nil
	})
	assert.Equal(t, 1, code)
	_, err := validateRoot(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, stderr.String(), "open project directory")
	assert.False(t, launched)
}

func TestRunReportsOutputFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "render help"},
		{name: "version", args: []string{"--version"}, want: "write version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run(test.args, failingWriter{}, &stderr, func(string) error {
				t.Fatal("output requests must not launch the UI")
				return nil
			})
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr.String(), test.want)
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestRunSelectsStartupLanguage(t *testing.T) {
	previous := i18n.Use(nil)
	t.Cleanup(func() { i18n.Use(previous) })
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "en_US")
	t.Setenv("LANG", "en_US")
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"--help"}, &stdout, &stderr), stderr.String())
	assert.Contains(t, stdout.String(), "显示帮助")
	assert.Contains(t, stdout.String(), "要打开的项目目录")
	assert.Equal(t, "文件", i18n.Text("msg_file"))
	t.Setenv("LC_ALL", "unsupported")
	stdout.Reset()
	require.Equal(t, 0, Run([]string{"--help"}, &stdout, &stderr))
	assert.Contains(t, stdout.String(), "Show this help.")
	assert.Equal(t, "File", i18n.Text("msg_file"))
}

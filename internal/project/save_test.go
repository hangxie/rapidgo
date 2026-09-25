package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFormatter func(context.Context, string) (string, error)

func (f testFormatter) Format(ctx context.Context, content string) (string, error) {
	return f(ctx, content)
}

type testWriter func(context.Context, string, string, string) error

func (w testWriter) Write(ctx context.Context, path, original, content string) error {
	return w(ctx, path, original, content)
}

func TestDiskSaverFormatsGoBeforeWriting(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o640))
	result, err := (DiskSaver{}).Save(context.Background(), SaveRequest{
		Path: path, Original: "package main\nfunc main() {}\n", Content: "package main\nfunc main(){println(1)}\n",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content, "func main() { println(1) }")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, result.Content, string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestDiskSaverPreservesOriginalOnFormatFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "main.go")
	original := "package main\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	_, err := (DiskSaver{}).Save(context.Background(), SaveRequest{Path: path, Original: original, Content: "package main\nfunc ("})
	assert.ErrorContains(t, err, "format")
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(data))
}

func TestDiskSaverReportsWriteFailureWithoutChangingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "main.go")
	original := "package main\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	writeErr := errors.New("disk full")
	called := false
	saver := DiskSaver{
		Formatter: testFormatter(func(_ context.Context, content string) (string, error) { return content, nil }),
		Writer: testWriter(func(_ context.Context, _, _, _ string) error {
			called = true
			return writeErr
		}),
	}
	_, err := saver.Save(context.Background(), SaveRequest{Path: path, Original: original, Content: "changed"})
	assert.ErrorIs(t, err, writeErr)
	assert.True(t, called)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(data))
}

func TestDiskSaverRejectsExternalChange(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("newer"), 0o600))
	_, err := (DiskSaver{}).Save(context.Background(), SaveRequest{Path: path, Original: "old", Content: "ours"})
	assert.ErrorIs(t, err, ErrFileChanged)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "newer", string(data))
}

func TestDiskSaverDoesNotFormatOtherFiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	saver := DiskSaver{Formatter: testFormatter(func(context.Context, string) (string, error) {
		t.Fatal("formatter should not run for .txt")
		return "", nil
	})}
	result, err := saver.Save(context.Background(), SaveRequest{Path: path, Original: "old", Content: "changed"})
	require.NoError(t, err)
	assert.Equal(t, "changed", result.Content)
}

func TestDiskSaverHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (DiskSaver{}).Save(ctx, SaveRequest{Path: "main.go"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAtomicWriterRejectsHardLinkedFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "first.txt")
	other := filepath.Join(directory, "second.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	if err := os.Link(path, other); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	err := (AtomicWriter{}).Write(context.Background(), path, "original", "changed")
	assert.ErrorIs(t, err, ErrMultipleLinks)
	for _, name := range []string{path, other} {
		data, readErr := os.ReadFile(name)
		require.NoError(t, readErr)
		assert.Equal(t, "original", string(data))
	}
}

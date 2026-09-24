//go:build linux || darwin

package project

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskSourceRejectsFIFOWithoutBlocking(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	entries, err := (DiskSource{}).List(context.Background(), filepath.Dir(path))
	require.NoError(t, err)
	assert.Contains(t, entries, Entry{Name: "pipe"})

	result := make(chan error, 1)
	go func() {
		_, err := (DiskSource{}).Open(context.Background(), path)
		result <- err
	}()
	select {
	case err := <-result:
		assert.ErrorContains(t, err, "not a regular file")
	case <-time.After(2 * time.Second):
		t.Fatal("opening a FIFO blocked the project worker")
	}
}

func TestOpenRegularFileRejectsReplacementFIFO(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o600))
	fifo := filepath.Join(root, "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	require.NoError(t, os.Rename(fifo, path))

	result := make(chan error, 1)
	go func() {
		file, err := openRegularFile(path)
		if file != nil {
			_ = file.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		assert.ErrorContains(t, err, "not a regular file")
	case <-time.After(2 * time.Second):
		t.Fatal("opening a replacement FIFO blocked the project worker")
	}
}

func TestOpenRegularFileRejectsReplacementSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	referent := filepath.Join(root, "other.go")
	require.NoError(t, os.WriteFile(referent, []byte("other"), 0o600))
	replacement := filepath.Join(root, "replacement")
	require.NoError(t, os.Symlink(referent, replacement))
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	require.NoError(t, os.Rename(replacement, path))

	file, err := openRegularFile(path)
	assert.Nil(t, file)
	assert.Error(t, err)
}

func TestFIFOGoModDoesNotMarkModule(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(root, "go.mod"), 0o600))
	entries, err := (DiskSource{}).List(context.Background(), root)
	require.NoError(t, err)
	assert.Contains(t, entries, Entry{Name: "go.mod"})

	tree := New(root)
	require.True(t, tree.Expand(tree.Root))
	tree.Apply(tree.Root, entries, nil)
	assert.False(t, tree.Root.Module)
}

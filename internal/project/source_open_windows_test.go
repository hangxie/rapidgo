package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRegularFileRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "target.go")
	require.NoError(t, os.WriteFile(target, []byte("package main\n"), 0o600))
	link := filepath.Join(root, "link.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("creating symlinks is unavailable: %v", err)
	}
	file, err := openRegularFile(link)
	assert.Nil(t, file)
	assert.Error(t, err)
}

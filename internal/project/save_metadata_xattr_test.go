//go:build linux || darwin

package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestAtomicWriterPreservesExtendedAttribute(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	name := testXattrName()
	if err := unix.Setxattr(path, name, []byte("metadata"), 0); err != nil {
		t.Skipf("extended attributes unavailable: %v", err)
	}
	require.NoError(t, (AtomicWriter{}).Write(context.Background(), path, "old", "new"))
	value := make([]byte, 32)
	n, err := unix.Getxattr(path, name, value)
	require.NoError(t, err)
	assert.Equal(t, "metadata", string(value[:n]))
}

//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriterPreservesSpecialMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "script.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	want := os.FileMode(0o751) | os.ModeSticky | os.ModeSetuid | os.ModeSetgid
	require.NoError(t, os.Chmod(path, want))
	before, err := os.Stat(path)
	require.NoError(t, err)
	if before.Mode()&(os.ModeSticky|os.ModeSetuid|os.ModeSetgid) != want&(os.ModeSticky|os.ModeSetuid|os.ModeSetgid) {
		t.Skip("filesystem does not retain special mode bits")
	}
	require.NoError(t, (AtomicWriter{}).Write(context.Background(), path, "old", "new"))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode()&(os.ModePerm|os.ModeSticky|os.ModeSetuid|os.ModeSetgid))
}

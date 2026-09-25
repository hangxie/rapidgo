package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskSourceListsArbitraryDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600))
	entries, err := (DiskSource{}).List(context.Background(), root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []Entry{
		{Name: "sub", IsDir: true}, {Name: "main.go", Regular: true}, {Name: "go.mod", Regular: true},
	}, entries)
	_, err = (DiskSource{}).List(context.Background(), filepath.Join(root, "missing"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDiskSourceOpenUTF8(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, test := range []struct {
		name, content string
		want          string
		wantError     string
	}{
		{name: "unicode.go", content: "package main\r\n// 界é\t!\n", want: "package main\r\n// 界é\t!\n"},
		{name: "control.go", content: "a\x01b\n", want: "a\x01b\n"},
		{name: "invalid.go", content: string([]byte{0xff}), wantError: "not UTF-8 text"},
		{name: "binary.go", content: "a\x00b", wantError: "not UTF-8 text"},
		{name: "large.go", content: strings.Repeat("x", MaxOpenBytes+1), wantError: "open limit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name)
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			result, err := (DiskSource{}).Open(context.Background(), path)
			if test.wantError != "" {
				assert.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, path, result.Path)
			assert.Equal(t, test.want, result.Text)
		})
	}
}

func TestDiskSourceHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (DiskSource{}).List(ctx, t.TempDir())
	assert.True(t, errors.Is(err, context.Canceled))
	_, err = (DiskSource{}).Open(ctx, filepath.Join(t.TempDir(), "missing"))
	assert.True(t, errors.Is(err, context.Canceled))
}

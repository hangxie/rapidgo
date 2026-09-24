package project

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTreeLoadsLazilyAndFindsModules(t *testing.T) {
	t.Parallel()

	tree := New("/tmp/work")
	root := tree.Root
	require.True(t, tree.Expand(root))
	assert.False(t, tree.Expand(root), "loading must not be queued twice")
	tree.Apply(root, []Entry{{Name: "z.go"}, {Name: "nested", IsDir: true}, {Name: "go.mod", Regular: true}}, nil)
	assert.True(t, root.Module)
	assert.Equal(t, []string{"work", "nested", "go.mod", "z.go"}, visibleNames(tree))

	nested := root.Children[0]
	assert.False(t, nested.Loaded, "child directories are not read recursively")
	require.True(t, tree.Expand(nested))
	tree.Collapse(nested)
	tree.Apply(nested, []Entry{{Name: "go.mod", Regular: true}, {Name: "main.go"}}, nil)
	assert.True(t, nested.Module)
	assert.False(t, nested.Expanded, "a completed load must not undo a collapse")
	assert.Equal(t, []string{"work", "nested", "go.mod", "z.go"}, visibleNames(tree))
	assert.False(t, tree.Expand(nested), "cached directories do not need another read")
	assert.Equal(t, []string{"work", "nested", "go.mod", "main.go", "go.mod", "z.go"}, visibleNames(tree))
}

func TestTreeErrorCanRetryAndSymlinkCannotExpand(t *testing.T) {
	t.Parallel()

	tree := New("/tmp/work")
	require.True(t, tree.Expand(tree.Root))
	permissionError := errors.New("permission denied")
	tree.Apply(tree.Root, nil, permissionError)
	assert.ErrorIs(t, tree.Root.Error, permissionError)
	assert.False(t, tree.Root.Loaded)
	require.True(t, tree.Expand(tree.Root))
	tree.Apply(tree.Root, []Entry{{Name: "linked", IsDir: true, Symlink: true}}, nil)
	assert.NoError(t, tree.Root.Error)
	assert.False(t, tree.Expand(tree.Root.Children[0]))
}

func TestTreeLargeDirectoryRemainsFlat(t *testing.T) {
	t.Parallel()

	tree := New("/tmp/work")
	require.True(t, tree.Expand(tree.Root))
	entries := make([]Entry, 5000)
	for index := range entries {
		entries[index] = Entry{Name: fmt.Sprintf("file-%04d.go", 4999-index)}
	}
	tree.Apply(tree.Root, entries, nil)
	items := tree.Visible()
	assert.Len(t, items, 5001)
	assert.Equal(t, "file-0000.go", items[1].Node.Name)
	assert.Equal(t, "file-4999.go", items[5000].Node.Name)
}

func visibleNames(tree *Tree) []string {
	items := tree.Visible()
	names := make([]string, len(items))
	for index, item := range items {
		names[index] = item.Node.Name
	}
	return names
}

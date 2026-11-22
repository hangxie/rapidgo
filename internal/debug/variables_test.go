package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hangxie/rapidgo/internal/dap"
)

func testScopes() []Scope {
	return []Scope{
		{Scope: dap.Scope{Name: "Locals", Reference: 1}, Loaded: true, Variables: []dap.Variable{
			{Name: "n", Value: "3", Type: "int"},
			{Name: "p", Value: "main.Point {X: 1, Y: 2}", Type: "main.Point", Reference: 10},
		}},
		{Scope: dap.Scope{Name: "Globals", Reference: 2, Expensive: true}},
	}
}

func rowNames(rows []Row) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func TestTreeRowsShowLoadedScopes(t *testing.T) {
	t.Parallel()

	tree := NewTree(5, testScopes())
	assert.Equal(t, 5, tree.FrameID)
	assert.Equal(t, []Row{
		{Name: "Locals", Expandable: true, Expanded: true},
		{Depth: 1, Name: "n", Value: "3", Type: "int"},
		{Depth: 1, Name: "p", Value: "main.Point {X: 1, Y: 2}", Type: "main.Point", Expandable: true},
		{Name: "Globals", Expandable: true},
	}, tree.Rows())
	assert.Empty(t, NewTree(5, nil).Rows())
}

func TestTreeExpandAndCollapse(t *testing.T) {
	t.Parallel()

	children := []dap.Variable{{Name: "X", Value: "1", Type: "int"}, {Name: "Y", Value: "2", Type: "int"}}
	tests := []struct {
		name      string
		act       func(*Tree) (int, bool)
		reference int
		load      bool
		rows      []string
	}{
		{"a leaf stays put", func(tree *Tree) (int, bool) { return tree.Expand(1) }, 0, false, []string{"Locals", "n", "p", "Globals"}},
		{"an unloaded value asks for its children", func(tree *Tree) (int, bool) { return tree.Expand(2) }, 10, true, []string{"Locals", "n", "p", "Globals"}},
		{"loaded children appear", func(tree *Tree) (int, bool) {
			tree.Expand(2)
			tree.Fill(10, children)
			return 0, false
		}, 0, false, []string{"Locals", "n", "p", "X", "Y", "Globals"}},
		{"a loaded value reopens without a request", func(tree *Tree) (int, bool) {
			tree.Expand(2)
			tree.Fill(10, children)
			tree.Collapse(2)
			return tree.Expand(2)
		}, 0, false, []string{"Locals", "n", "p", "X", "Y", "Globals"}},
		{"a value waiting for children is not asked for again", func(tree *Tree) (int, bool) {
			tree.Expand(2)
			return tree.Expand(2)
		}, 0, false, []string{"Locals", "n", "p", "Globals"}},
		{"a failed load closes the value", func(tree *Tree) (int, bool) {
			tree.Expand(2)
			tree.Fail(10)
			return tree.Expand(2)
		}, 10, true, []string{"Locals", "n", "p", "Globals"}},
		{"collapsing a scope hides its variables", func(tree *Tree) (int, bool) {
			tree.Collapse(0)
			return 0, false
		}, 0, false, []string{"Locals", "Globals"}},
		{"an expensive scope loads on demand", func(tree *Tree) (int, bool) { return tree.Expand(3) }, 2, true, []string{"Locals", "n", "p", "Globals"}},
		{"an index past the rows does nothing", func(tree *Tree) (int, bool) { return tree.Expand(9) }, 0, false, []string{"Locals", "n", "p", "Globals"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tree := NewTree(5, testScopes())
			reference, load := test.act(tree)
			assert.Equal(t, test.reference, reference)
			assert.Equal(t, test.load, load)
			assert.Equal(t, test.rows, rowNames(tree.Rows()))
		})
	}
}

func TestTreeShowsLoading(t *testing.T) {
	t.Parallel()

	tree := NewTree(5, testScopes())
	tree.Expand(2)
	assert.True(t, tree.Rows()[2].Loading)
	assert.True(t, tree.Rows()[2].Expanded)
	tree.Fill(10, nil)
	assert.False(t, tree.Rows()[2].Loading)
	assert.Equal(t, []string{"Locals", "n", "p", "Globals"}, rowNames(tree.Rows()), "an empty value has no rows")
	tree.Fill(99, []dap.Variable{{Name: "stray"}})
	assert.Len(t, tree.Rows(), 4, "children nobody asked for are dropped")
}

func TestTreeCollapseMovesToTheParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		index, want int
		rows        []string
	}{
		{"a leaf selects its parent", 3, 2, []string{"Locals", "n", "p", "X", "Globals"}},
		{"so does a variable directly in a scope", 1, 0, []string{"Locals", "n", "p", "X", "Globals"}},
		{"a closed top-level row stays", 4, 4, []string{"Locals", "n", "p", "X", "Globals"}},
		{"an open value closes in place", 2, 2, []string{"Locals", "n", "p", "Globals"}},
		{"an open scope closes in place", 0, 0, []string{"Locals", "Globals"}},
		{"an index past the rows is unchanged", 9, 9, []string{"Locals", "n", "p", "X", "Globals"}},
		{"a negative index is unchanged", -1, -1, []string{"Locals", "n", "p", "X", "Globals"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tree := NewTree(5, testScopes())
			tree.Expand(2)
			tree.Fill(10, []dap.Variable{{Name: "X", Value: "1"}})
			assert.Equal(t, test.want, tree.Collapse(test.index))
			assert.Equal(t, test.rows, rowNames(tree.Rows()))
		})
	}
}

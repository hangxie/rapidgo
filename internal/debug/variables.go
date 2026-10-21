package debug

import "github.com/hangxie/rapidgo/internal/dap"

// Scope is one of a frame's variable groups; Variables holds its contents when Loaded.
type Scope struct {
	dap.Scope
	Loaded    bool
	Variables []dap.Variable
}

// Tree is the expandable variables of one paused frame; it is not safe for concurrent use.
type Tree struct {
	FrameID int
	roots   []*node
}

type node struct {
	name, value, kind string
	reference         int // nonzero when the adapter can load children
	children          []*node
	loaded            bool
	expanded          bool
	loading           bool
}

// Row is one visible line of a Tree, indented by Depth.
type Row struct {
	Depth      int
	Name       string
	Value      string
	Type       string
	Expandable bool
	Expanded   bool
	Loading    bool
}

// NewTree opens each scope whose variables are already loaded.
func NewTree(frameID int, scopes []Scope) *Tree {
	tree := &Tree{FrameID: frameID}
	for _, scope := range scopes {
		root := &node{name: scope.Name, reference: scope.Reference}
		if scope.Loaded {
			root.fill(scope.Variables)
			root.expanded = true
		}
		tree.roots = append(tree.roots, root)
	}
	return tree
}

func (n *node) fill(variables []dap.Variable) {
	n.children = make([]*node, 0, len(variables))
	for _, variable := range variables {
		n.children = append(n.children, &node{name: variable.Name, value: variable.Value, kind: variable.Type, reference: variable.Reference})
	}
	n.loaded, n.loading = true, false
}

// visible is a row's node, its depth, and its parent's row index, or -1 at the top.
type visible struct {
	node   *node
	depth  int
	parent int
}

func (t *Tree) visible() []visible {
	var rows []visible
	var walk func(nodes []*node, depth, parent int)
	walk = func(nodes []*node, depth, parent int) {
		for _, n := range nodes {
			rows = append(rows, visible{n, depth, parent})
			if n.expanded {
				walk(n.children, depth+1, len(rows)-1)
			}
		}
	}
	walk(t.roots, 0, -1)
	return rows
}

func (t *Tree) Rows() []Row {
	visible := t.visible()
	rows := make([]Row, 0, len(visible))
	for _, row := range visible {
		n := row.node
		rows = append(rows, Row{
			Depth: row.depth, Name: n.name, Value: n.value, Type: n.kind,
			Expandable: n.reference != 0, Expanded: n.expanded, Loading: n.loading,
		})
	}
	return rows
}

// Expand opens the row at index and returns the reference to load when its children are not loaded yet.
func (t *Tree) Expand(index int) (int, bool) {
	rows := t.visible()
	if index < 0 || index >= len(rows) {
		return 0, false
	}
	n := rows[index].node
	if n.reference == 0 || n.loading {
		return 0, false
	}
	n.expanded = true
	if n.loaded {
		return 0, false
	}
	n.loading = true
	return n.reference, true
}

// Collapse closes the open row at index, or else returns its parent's index for the caller to select.
func (t *Tree) Collapse(index int) int {
	rows := t.visible()
	if index < 0 || index >= len(rows) {
		return index
	}
	row := rows[index]
	if row.node.expanded {
		row.node.expanded, row.node.loading = false, false
		return index
	}
	if row.parent < 0 {
		return index
	}
	return row.parent
}

// Fill gives the children loaded for reference to the rows waiting for them.
func (t *Tree) Fill(reference int, variables []dap.Variable) {
	t.each(func(n *node) {
		if n.loading && n.reference == reference {
			n.fill(variables)
		}
	})
}

// Fail closes the rows waiting for reference, so expanding them asks again.
func (t *Tree) Fail(reference int) {
	t.each(func(n *node) {
		if n.loading && n.reference == reference {
			n.loading, n.expanded = false, false
		}
	})
}

func (t *Tree) each(visit func(*node)) {
	var walk func(nodes []*node)
	walk = func(nodes []*node) {
		for _, n := range nodes {
			visit(n)
			walk(n.children)
		}
	}
	walk(t.roots)
}

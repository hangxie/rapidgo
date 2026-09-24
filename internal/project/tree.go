// Package project models a lazily browsed directory tree independently of the UI.
package project

import (
	"path/filepath"
	"sort"
)

// Entry is filesystem metadata returned by a directory scan.
type Entry struct {
	Name    string
	IsDir   bool
	Symlink bool
	Regular bool
}

// Node is a directory or file in the project tree.
type Node struct {
	Name     string
	Path     string
	Parent   *Node
	IsDir    bool
	Symlink  bool
	Module   bool
	Expanded bool
	Loaded   bool
	Loading  bool
	Error    error
	Children []*Node
}

// Item is a visible node and its indentation depth.
type Item struct {
	Node  *Node
	Depth int
}

// Tree only contains directories the user has opened; it never recursively walks.
type Tree struct {
	Root *Node
}

func New(root string) *Tree {
	return &Tree{Root: &Node{Name: filepath.Base(root), Path: root, IsDir: true}}
}

// Expand returns true when the caller needs to load this directory.
func (t *Tree) Expand(node *Node) bool {
	if node == nil || !node.IsDir || node.Symlink {
		return false
	}
	node.Expanded = true
	if node.Loaded || node.Loading {
		return false
	}
	node.Loading = true
	return true
}

func (t *Tree) Collapse(node *Node) {
	if node != nil && node.IsDir {
		node.Expanded = false
	}
}

// Apply finishes a directory load, preserving a collapse made while it ran.
func (t *Tree) Apply(node *Node, entries []Entry, err error) {
	if node == nil || !node.Loading {
		return
	}
	node.Loading = false
	node.Error = err
	if err != nil {
		return // A later expansion can retry.
	}
	node.Loaded = true
	node.Module = false
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	node.Children = make([]*Node, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "go.mod" && entry.Regular {
			node.Module = true
		}
		node.Children = append(node.Children, &Node{
			Name: entry.Name, Path: filepath.Join(node.Path, entry.Name), Parent: node,
			IsDir: entry.IsDir, Symlink: entry.Symlink,
		})
	}
}

func (t *Tree) Visible() []Item {
	if t == nil || t.Root == nil {
		return nil
	}
	items := make([]Item, 0, 32)
	var visit func(*Node, int)
	visit = func(node *Node, depth int) {
		items = append(items, Item{Node: node, Depth: depth})
		if node.Expanded {
			for _, child := range node.Children {
				visit(child, depth+1)
			}
		}
	}
	visit(t.Root, 0)
	return items
}

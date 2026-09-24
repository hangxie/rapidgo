package ui

import (
	"errors"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/project"
)

var errWorkQueueFull = errors.New("project work queue is full")

func newShellState(root string, enqueue func(workRequest) bool) shellState {
	state := shellState{projectRoot: root, tree: project.New(root), enqueue: enqueue, message: "Loading project..."}
	if state.tree.Expand(state.tree.Root) && !enqueue(workRequest{kind: listDirectory, path: root, node: state.tree.Root}) {
		state.tree.Apply(state.tree.Root, nil, errWorkQueueFull)
		state.message = errWorkQueueFull.Error()
	}
	return state
}

func calculateLayoutSize(screen tcell.Screen) layout {
	width, height := screen.Size()
	return calculateLayout(width, height)
}

func (state *shellState) treeArea(screen tcell.Screen) rectangle {
	view := calculateLayoutSize(screen)
	if view.projectVisible {
		return view.project
	}
	if state.focus == focusTree {
		return view.editor
	}
	return rectangle{}
}

func (state *shellState) treeAccessible(screen tcell.Screen) bool {
	return state.tree != nil && state.treeArea(screen).height > 0
}

func (state *shellState) selectedNode() *project.Node {
	if state.tree == nil {
		return nil
	}
	items := state.tree.Visible()
	if state.selected < 0 || state.selected >= len(items) {
		return nil
	}
	return items[state.selected].Node
}

func (state *shellState) moveSelection(screen tcell.Screen, offset int) {
	state.moveSelectionTo(screen, state.selected+offset)
}

func (state *shellState) moveSelectionTo(screen tcell.Screen, index int) {
	if !state.treeAccessible(screen) {
		return
	}
	items := state.tree.Visible()
	if len(items) == 0 {
		return
	}
	state.selected = max(0, min(index, len(items)-1))
	state.keepSelectionVisible(screen)
}

func (state *shellState) keepSelectionVisible(screen tcell.Screen) {
	if state.tree == nil {
		return
	}
	items := state.tree.Visible()
	state.selected = max(0, min(state.selected, len(items)-1))
	area := state.treeArea(screen)
	visibleRows := max(1, area.height-2)
	if state.selected < state.treeScroll {
		state.treeScroll = state.selected
	}
	if state.selected >= state.treeScroll+visibleRows {
		state.treeScroll = state.selected - visibleRows + 1
	}
	state.treeScroll = max(0, min(state.treeScroll, max(0, len(items)-visibleRows)))
}

func (state *shellState) expandSelected(screen tcell.Screen) {
	if !state.treeAccessible(screen) {
		return
	}
	node := state.selectedNode()
	if node == nil || !state.tree.Expand(node) {
		return
	}
	state.message = "Loading " + node.Path + "..."
	if state.enqueue == nil || !state.enqueue(workRequest{kind: listDirectory, path: node.Path, node: node}) {
		state.tree.Apply(node, nil, errWorkQueueFull)
		state.message = errWorkQueueFull.Error()
	}
}

func (state *shellState) collapseOrParent(screen tcell.Screen) {
	if !state.treeAccessible(screen) {
		return
	}
	node := state.selectedNode()
	if node == nil {
		return
	}
	if node.IsDir && node.Expanded {
		state.tree.Collapse(node)
		return
	}
	if node.Parent != nil {
		for index, item := range state.tree.Visible() {
			if item.Node == node.Parent {
				state.selected = index
				break
			}
		}
	}
}

func (state *shellState) openSelected(screen tcell.Screen) {
	if !state.treeAccessible(screen) {
		return
	}
	node := state.selectedNode()
	if node == nil {
		return
	}
	if node.IsDir {
		state.expandSelected(screen)
		return
	}
	if node.Symlink {
		state.message = "Symlinks are not opened in this preview"
		return
	}
	state.openSeq++
	state.opening = true
	state.message = "Opening " + node.Path + "..."
	if state.enqueue == nil || !state.enqueue(workRequest{kind: openFile, path: node.Path, seq: state.openSeq}) {
		state.opening = false
		state.message = errWorkQueueFull.Error()
	}
}

func (state *shellState) applyResult(result workResult) {
	if result.request.kind == listDirectory {
		state.tree.Apply(result.request.node, result.entries, result.err)
		if result.err != nil {
			state.message = result.err.Error() + " (Enter to retry)"
		} else {
			state.message = fmt.Sprintf("Loaded %s (%d entries)", result.request.path, len(result.entries))
		}
		return
	}
	if result.request.seq != state.openSeq {
		return // A newer open request superseded this result.
	}
	state.opening = false
	if result.err != nil {
		state.message = result.err.Error()
		return
	}
	state.document = &result.document
	state.fileScroll = 0
	state.focus = focusPreview
	state.message = "Opened " + result.document.Path + " (read-only preview)"
}

func (state *shellState) scrollFile(screen tcell.Screen, direction int) {
	if state.document == nil || state.focus != focusPreview {
		return
	}
	visibleRows := max(1, calculateLayoutSize(screen).editor.height-2)
	state.fileScroll = max(0, min(state.fileScroll+direction*visibleRows, max(0, len(state.document.Lines)-visibleRows)))
}

func (state *shellState) scrollFileLine(screen tcell.Screen, direction int) {
	if state.document == nil || state.focus != focusPreview {
		return
	}
	visibleRows := max(1, calculateLayoutSize(screen).editor.height-2)
	state.fileScroll = max(0, min(state.fileScroll+direction, max(0, len(state.document.Lines)-visibleRows)))
}

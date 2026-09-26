package ui

import (
	"errors"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/editor"
	"github.com/hangxie/rapidgo/internal/project"
)

var errWorkQueueFull = errors.New("project work queue is full")

func newShellState(root string, enqueue func(workRequest) bool) shellState {
	state := shellState{projectRoot: root, tree: project.New(root), enqueue: enqueue, message: "Loading project...", syntax: &syntaxCache{}}
	if state.tree.Expand(state.tree.Root) && !enqueue(workRequest{kind: listDirectory, path: root, node: state.tree.Root}) {
		state.tree.Apply(state.tree.Root, nil, errWorkQueueFull)
		state.message = errWorkQueueFull.Error()
	}
	return state
}

func (state *shellState) layout(screen tcell.Screen) layout {
	width, height := screen.Size()
	return calculateLayout(width, height, state.focus == focusOutput)
}

func (state *shellState) treeArea(screen tcell.Screen) rectangle {
	view := state.layout(screen)
	if view.projectVisible {
		return view.project
	}
	if state.focus == focusTree || (state.focus == focusOutput && state.mainFocus == focusTree) {
		return view.editor
	}
	return rectangle{}
}

// setFocus moves focus, remembering the last tree or editor pane.
func (state *shellState) setFocus(pane paneFocus) {
	if pane != focusOutput {
		state.mainFocus = pane
	}
	state.focus = pane
}

// focusNextPane cycles tree -> editor -> output, skipping any unusable pane.
func (state *shellState) focusNextPane(screen tcell.Screen) {
	outputVisible := state.layout(screen).output.height > 0
	switch state.focus {
	case focusTree:
		if state.document != nil {
			state.setFocus(focusEditor)
			return
		}
		if outputVisible {
			state.setFocus(focusOutput)
		}
	case focusEditor:
		if outputVisible {
			state.setFocus(focusOutput)
			return
		}
		state.setFocus(focusTree)
	default:
		state.setFocus(focusTree)
	}
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
	if state.saving {
		state.message = "Save in progress; wait before switching files"
		return
	}
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
		state.message = "Symlinks are not opened in the editor"
		return
	}
	if state.document != nil && state.document.Path == node.Path {
		state.openSeq++ // Re-selecting this file cancels any pending switch.
		state.opening = false
		state.focusSeq++
		state.setFocus(focusEditor)
		return
	}
	if state.buffer != nil && state.buffer.Dirty() {
		state.openSeq++ // Invalidate any older read before asking to switch files.
		state.opening = false
		state.confirm = confirmOpen
		state.pendingPath = node.Path
		state.message = "Unsaved changes: press D to discard and open another file, Esc to cancel"
		return
	}
	state.queueOpen(node.Path, false)
}

func (state *shellState) queueOpen(path string, discardApproved bool) {
	state.openSeq++
	state.opening = true
	state.message = "Opening " + path + "..."
	request := workRequest{kind: openFile, path: path, seq: state.openSeq, focusSeq: state.focusSeq, discardApproved: discardApproved}
	if discardApproved && state.buffer != nil {
		request.approvedText = state.buffer.Text()
	}
	if state.enqueue == nil || !state.enqueue(request) {
		state.opening = false
		state.message = errWorkQueueFull.Error()
	}
}

func (state *shellState) applyResult(result workResult) {
	if result.request.kind == saveFile {
		state.applySaveResult(result)
		return
	}
	if result.request.kind == listDirectory {
		selected := state.selectedNode()
		state.tree.Apply(result.request.node, result.entries, result.err)
		if selected != nil {
			for index, item := range state.tree.Visible() {
				if item.Node == selected {
					state.selected = index
					break
				}
			}
		}
		if result.err != nil {
			state.message = fmt.Sprintf("Failed to load %s: %v (select directory and press Enter to retry)", result.request.path, result.err)
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
	if state.buffer != nil && state.buffer.Dirty() && (!result.request.discardApproved || state.buffer.Text() != result.request.approvedText) {
		state.confirm = confirmLoaded
		state.pendingFile = &result
		state.message = "Unsaved changes: press D to discard and open loaded file, Esc to cancel"
		return
	}
	state.installDocument(result)
}

func (state *shellState) installDocument(result workResult) {
	buffer, err := editor.New(result.document.Text)
	if err != nil {
		state.message = fmt.Sprintf("Open %s: %v", result.document.Path, err)
		return
	}
	state.document = &result.document
	state.buffer = buffer
	state.fileScroll = 0
	state.fileColumn = 0
	if result.request.focusSeq == state.focusSeq {
		state.setFocus(focusEditor)
	}
	state.message = "Opened " + result.document.Path + " (editing)"
	state.applyPendingPosition() // A jump named a position in this file.
}

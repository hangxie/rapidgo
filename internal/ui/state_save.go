package ui

import (
	"fmt"

	"github.com/hangxie/rapidgo/internal/project"
)

func (state *shellState) requestSave() {
	if state.document == nil || state.buffer == nil {
		state.message = "Open a file before saving"
		return
	}
	if state.saving {
		state.message = "Save already in progress"
		return
	}
	state.openSeq++ // Saving the current document supersedes a pending file switch.
	state.opening = false
	state.saveSeq++
	request := workRequest{
		kind: saveFile, path: state.document.Path, seq: state.saveSeq,
		save:     project.SaveRequest{Path: state.document.Path, Original: state.document.Text, Content: state.buffer.SerializedText()},
		revision: state.buffer.Revision(),
	}
	if state.enqueue == nil || !state.enqueue(request) {
		state.message = errWorkQueueFull.Error()
		return
	}
	state.saving = true
	state.message = "Saving " + state.document.Path + "..."
}

func (state *shellState) applySaveResult(result workResult) {
	if result.request.seq != state.saveSeq || state.document == nil || state.document.Path != result.request.path {
		return
	}
	state.saving = false
	if result.err != nil {
		state.message = "Save failed: " + result.err.Error()
		return
	}
	state.document.Text = result.saved.Content
	// The file reached disk, so the cached package listing may be stale.
	state.invalidatePackages()
	if state.buffer.Revision() == result.request.revision {
		if err := state.buffer.ApplySavedText(result.saved.Content); err != nil {
			state.message = fmt.Sprintf("Saved %s, but could not apply formatted text: %v", state.document.Path, err)
			return
		}
	} else if result.saved.Content == result.request.save.Content {
		state.buffer.MarkSavedRevision(result.request.revision)
	}
	state.message = "Saved " + state.document.Path
	if state.buffer.Dirty() {
		state.message += " (newer edits remain unsaved)"
	}
}

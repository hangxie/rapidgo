package ui

import (
	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/project"
)

func (state *shellState) requestSave() {
	if state.document == nil || state.buffer == nil {
		state.message = i18n.Text("msg_open_a_file_before_saving")
		return
	}
	if state.saving {
		state.message = i18n.Text("msg_save_already_in_progress")
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
	state.message = i18n.Format("msg_saving_s", state.document.Path)
}

func (state *shellState) applySaveResult(result workResult) {
	if result.request.seq != state.saveSeq {
		return
	}
	if state.workspace != nil {
		for _, win := range state.workspace.Windows {
			if win.Document.Path != result.request.path {
				continue
			}
			document, buffer := state.document, state.buffer
			state.document, state.buffer = win.Document, win.Buffer
			defer func() { state.document, state.buffer = document, buffer }()
			break
		}
	}
	state.applyDocumentSave(result)
}

func (state *shellState) applyDocumentSave(result workResult) {
	if result.request.seq != state.saveSeq || state.document == nil || state.document.Path != result.request.path {
		return
	}
	state.saving = false
	if result.err != nil {
		state.message = i18n.Format("msg_save_failed_s", result.err.Error())
		state.saveErrorMessage = state.message
		return
	}
	state.document.Text = result.saved.Content
	// The file reached disk, so the cached package listing may be stale.
	state.invalidatePackages()
	if state.buffer.Revision() == result.request.revision {
		if err := state.buffer.ApplySavedText(result.saved.Content); err != nil {
			state.message = i18n.Format("msg_saved_s_but_could_not_apply_formatted_text_v", state.document.Path, err)
			return
		}
	} else if result.saved.Content == result.request.save.Content {
		state.buffer.MarkSavedRevision(result.request.revision)
	}
	state.message = i18n.Format("msg_saved_s", state.document.Path)
	if state.buffer.Dirty() {
		state.message += i18n.Text("msg_newer_edits_remain_unsaved")
	}
}

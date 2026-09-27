package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hangxie/rapidgo/internal/project"
)

func TestSaveRequestFeedbackAndStaleResult(t *testing.T) {
	t.Parallel()

	state := &shellState{}
	state.requestSave()
	assert.Equal(t, "Open a file before saving", state.message)
	setTestDocument(t, state, "main.go", "original")
	state.saving = true
	state.requestSave()
	assert.Equal(t, "Save already in progress", state.message)
	state.saving = false
	state.requestSave()
	assert.Equal(t, errWorkQueueFull.Error(), state.message)
	assert.False(t, state.saving)

	state.saving = true
	state.saveSeq = 2
	state.applySaveResult(workResult{
		request: workRequest{kind: saveFile, path: "main.go", seq: 1},
		saved:   project.SaveResult{Content: "stale"},
	})
	assert.True(t, state.saving)
	assert.Equal(t, "original", state.document.Text)
}

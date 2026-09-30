package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/project"
)

func TestWorkspaceEditing(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(100, 30)
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "界\nhello"}})
	_ = state.buffer.Insert("x")
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "second"}})
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModShift))
	if state.document.Path != "a.go" || state.buffer.Text() != "x界\nhello" {
		t.Fatal("switch lost unsaved document")
	}
	state.windowAction(screen, 0)
	_ = state.buffer.Insert("y")
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone))
	if state.buffer.Text() != "xy界\nhello" || state.buffer.Cursor().Column != 2 {
		t.Fatal("duplicate did not share edits")
	}
	state.windowAction(screen, 3)
	for _, size := range [][2]int{{25, 9}, {1, 1}, {100, 30}} {
		screen.SetSize(size[0], size[1])
		handleEvent(screen, &state, tcell.NewEventResize(size[0], size[1]))
		render(screen, state)
	}
	state.windowAction(screen, 4)
	if state.requestQuit() || state.confirm != confirmQuit {
		t.Fatal("quit missed dirty inactive document")
	}
}

func TestWorkspaceSaveAfterSwitch(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	var requests []workRequest
	state := newShellState("/project", func(r workRequest) bool { requests = append(requests, r); return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "a"}})
	_ = state.buffer.Insert("x")
	first := state.buffer
	state.requestSave()
	request := requests[len(requests)-1]
	state.installDocument(workResult{document: project.Document{Path: "b.go", Text: "b"}})
	state.applySaveResult(workResult{request: request, saved: project.SaveResult{Content: "xa"}})
	if state.saving || first.Dirty() || state.document.Path != "b.go" || state.buffer.Text() != "b" {
		t.Fatal("save applied to wrong document")
	}
	state.setFocus(focusEditor)
	state.windowAction(screen, 5)
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModShift))
	handleKey(screen, &state, tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if state.windowSizing {
		t.Fatal("placement did not finish")
	}
}

func TestInstallDocumentUsesSharedView(t *testing.T) {
	state := newShellState("/project", func(workRequest) bool { return true })
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "original"}})
	first := state.buffer
	_ = first.Insert("x")
	state.installDocument(workResult{document: project.Document{Path: "a.go", Text: "stale"}})
	if state.buffer.Text() != first.Text() {
		t.Fatal("installed view does not use the shared document")
	}
}

package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/project"
)

func TestRenderWorkspace(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	state := newShellState("/project", func(workRequest) bool { return true })
	for _, path := range []string{"a.go", "b.go"} {
		state.installDocument(workResult{document: project.Document{Path: path, Text: "package main"}})
	}
	state.windowAction(screen, 3)
	render(screen, state)
	var contents strings.Builder
	cells, _, _ := screen.GetContents()
	for _, cell := range cells {
		contents.WriteString(string(cell.Runes))
	}
	if !strings.Contains(contents.String(), "a.go") || !strings.Contains(contents.String(), "b.go") {
		t.Fatal("tiled window titles missing")
	}
	x, y, visible := screen.GetCursor()
	area := state.layout(screen).editor
	if !visible || x <= area.x || x >= area.x+area.width || y <= area.y || y >= area.y+area.height {
		t.Fatal("active cursor outside window")
	}
	screen.SetSize(20, 8)
	render(screen, state)
	cells, _, _ = screen.GetContents()
	contents.Reset()
	for _, cell := range cells {
		contents.WriteString(string(cell.Runes))
	}
	if strings.Contains(contents.String(), "a.go") || !strings.Contains(contents.String(), "b.go") {
		t.Fatal("active overlapping window not on top")
	}
}

package editor

import "testing"

func TestSharedViews(t *testing.T) {
	b, _ := New("界a\nsecond")
	other := b.NewView()
	_ = other.Select(Position{Column: 1}, Position{Column: 2})
	_ = b.Insert("e\u0301")
	if other.Text() != b.Text() || other.Cursor() != (Position{Column: 3}) || !other.HasSelection() {
		t.Fatalf("shared edit lost view: %q %v", other.Text(), other.Cursor())
	}
	if b.Cursor() != (Position{Column: 1}) {
		t.Fatal(b.Cursor())
	}
	if !other.Undo() || b.Text() != "界a\nsecond" || b.Dirty() {
		t.Fatal("shared undo failed")
	}
	if !b.Redo() || other.Text() != b.Text() {
		t.Fatal("shared redo failed")
	}
	b.MarkSaved()
	if other.Dirty() {
		t.Fatal("save checkpoint is not shared")
	}
	_ = b.Select(Position{}, Position{Line: 1, Column: 6})
	_ = b.Insert("x")
	if other.Cursor().Line != 0 || other.Cursor().Column > 1 {
		t.Fatal("stale cursor after deletion")
	}
	other.CloseView()
}

func TestUndoFromAnotherViewPreservesCaret(t *testing.T) {
	b, _ := New("abc\ndef")
	other := b.NewView()
	_ = other.MoveTo(Position{Line: 1, Column: 3}, false)
	_ = b.Insert("x")
	other.Undo()
	if other.Cursor() != (Position{Line: 1, Column: 3}) {
		t.Fatal("undo copied another window's caret", other.Cursor())
	}
	other.Redo()
	if other.Cursor() != (Position{Line: 1, Column: 3}) {
		t.Fatal("redo copied another window's caret", other.Cursor())
	}
}

func TestFormattingPreservesOtherViewPosition(t *testing.T) {
	b, _ := New("package main\nfunc main(){}\n")
	other := b.NewView()
	_ = other.Select(Position{Line: 1}, Position{Line: 1, Column: 4})
	if err := b.ApplySavedText("package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	if other.Cursor() != (Position{Line: 1, Column: 4}) || !other.HasSelection() {
		t.Fatal("formatted save lost the independent view", other.Cursor())
	}
}

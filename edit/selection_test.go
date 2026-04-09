package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestCursorState_HasSelection(t *testing.T) {
	cs := CursorState{
		Cursor: buffer.Position{Line: 0, ByteCol: 3},
		Anchor: buffer.Position{Line: 0, ByteCol: 3},
	}
	if cs.HasSelection() {
		t.Error("same pos should not be selection")
	}
	cs.Anchor.ByteCol = 0
	if !cs.HasSelection() {
		t.Error("different pos should be selection")
	}
}

func TestCursorState_SelectionRange_Ordered(t *testing.T) {
	// Cursor before anchor.
	cs := CursorState{
		Cursor: buffer.Position{Line: 0, ByteCol: 0},
		Anchor: buffer.Position{Line: 1, ByteCol: 5},
	}
	r := cs.SelectionRange()
	if r.Start != cs.Cursor || r.End != cs.Anchor {
		t.Errorf("got %+v", r)
	}

	// Anchor before cursor (reversed).
	cs.Cursor, cs.Anchor = cs.Anchor, cs.Cursor
	r = cs.SelectionRange()
	if r.Start != cs.Anchor || r.End != cs.Cursor {
		t.Errorf("reversed: got %+v", r)
	}
}

func TestDeleteCursorSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	cs := CursorState{
		Cursor: buffer.Position{Line: 0, ByteCol: 5},
		Anchor: buffer.Position{Line: 0, ByteCol: 0},
	}
	ok := deleteCursorSelection(&cs, buf)
	if !ok {
		t.Error("should return true")
	}
	if buf.String() != " world" {
		t.Errorf("buf=%q", buf.String())
	}
	if cs.Cursor != (buffer.Position{}) {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
	if cs.HasSelection() {
		t.Error("selection should be cleared")
	}
}

func TestDeleteCursorSelection_NoSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	pos := buffer.Position{Line: 0, ByteCol: 1}
	cs := CursorState{Cursor: pos, Anchor: pos}
	ok := deleteCursorSelection(&cs, buf)
	if ok {
		t.Error("should return false for no selection")
	}
	if buf.String() != "abc" {
		t.Errorf("buf=%q (should be unchanged)", buf.String())
	}
}

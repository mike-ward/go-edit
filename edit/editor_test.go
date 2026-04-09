package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func mkCursor(line, col int) CursorState {
	pos := buffer.Position{Line: line, ByteCol: col}
	return CursorState{
		Cursor:     pos,
		Anchor:     pos,
		DesiredCol: col,
	}
}

func mkBuf(s string) *buffer.Buffer {
	return buffer.FromBytes([]byte(s))
}

// ---------- movement ----------

func TestMoveLeftWithinLine(t *testing.T) {
	cs := mkCursor(0, 3)
	moveLeft(&cs, mkBuf("abcdef"))
	if cs.Cursor.ByteCol != 2 || cs.Cursor.Line != 0 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestMoveLeftCrossLine(t *testing.T) {
	cs := mkCursor(1, 0)
	moveLeft(&cs, mkBuf("abc\ndef"))
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 3 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestMoveLeftAtStart(t *testing.T) {
	cs := mkCursor(0, 0)
	moveLeft(&cs, mkBuf("abc"))
	if cs.Cursor != (buffer.Position{}) {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestMoveRightCrossLine(t *testing.T) {
	cs := mkCursor(0, 3)
	moveRight(&cs, mkBuf("abc\ndef"))
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 0 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestMoveRightAtEnd(t *testing.T) {
	cs := mkCursor(0, 3)
	moveRight(&cs, mkBuf("abc"))
	if cs.Cursor.ByteCol != 3 || cs.Cursor.Line != 0 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestMoveUpDesiredColPreserved(t *testing.T) {
	cs := mkCursor(2, 10)
	cs.DesiredCol = 10
	// Line 1 is shorter; cursor should clamp to its length but
	// DesiredCol should survive.
	moveUp(&cs, mkBuf("long line here\nshort\nanother long line"), 1)
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 5 {
		t.Errorf("got %+v", cs.Cursor)
	}
	if cs.DesiredCol != 10 {
		t.Errorf("DesiredCol=%d want 10", cs.DesiredCol)
	}
}

func TestMoveDownPastEnd(t *testing.T) {
	cs := mkCursor(0, 0)
	moveDown(&cs, mkBuf("a\nb\nc"), 100)
	if cs.Cursor.Line != 2 {
		t.Errorf("got Line=%d want 2", cs.Cursor.Line)
	}
}

// ---------- editing ----------

func TestBackspaceMidLine(t *testing.T) {
	buf := mkBuf("hello")
	cs := mkCursor(0, 3)
	backspace(&cs, buf)
	if buf.String() != "helo" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.ByteCol != 2 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

func TestBackspaceJoinsLines(t *testing.T) {
	buf := mkBuf("foo\nbar")
	cs := mkCursor(1, 0)
	backspace(&cs, buf)
	if buf.String() != "foobar" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 3 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

func TestBackspaceAtStartNoop(t *testing.T) {
	buf := mkBuf("abc")
	cs := mkCursor(0, 0)
	backspace(&cs, buf)
	if buf.String() != "abc" {
		t.Errorf("content=%q", buf.String())
	}
}

func TestDeleteForwardJoinsLines(t *testing.T) {
	buf := mkBuf("foo\nbar")
	cs := mkCursor(0, 3)
	deleteForward(&cs, buf)
	if buf.String() != "foobar" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.ByteCol != 3 || cs.Cursor.Line != 0 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

func TestDeleteForwardAtEOFNoop(t *testing.T) {
	buf := mkBuf("abc")
	cs := mkCursor(0, 3)
	deleteForward(&cs, buf)
	if buf.String() != "abc" {
		t.Errorf("content=%q", buf.String())
	}
}

func TestInsertNewlineSplitsLine(t *testing.T) {
	buf := mkBuf("hello")
	cs := mkCursor(0, 3)
	insertNewline(EditorCfg{Buffer: buf}, &cs, buf)
	if buf.String() != "hel\nlo" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 0 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

// ---------- scroll ----------

func TestEnsureCursorVisibleScrollsDown(t *testing.T) {
	st := editorState{
		Cursors: []CursorState{
			{Cursor: buffer.Position{Line: 15}},
		},
		ScrollY: 0,
	}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 100})
	if st.ScrollY != 60 {
		t.Errorf("ScrollY=%v want 60", st.ScrollY)
	}
}

func TestEnsureCursorVisibleScrollsUp(t *testing.T) {
	st := editorState{
		Cursors: []CursorState{
			{Cursor: buffer.Position{Line: 2}},
		},
		ScrollY: 100,
	}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 100})
	if st.ScrollY != 20 {
		t.Errorf("ScrollY=%v want 20", st.ScrollY)
	}
}

func TestEnsureCursorVisibleNoop(t *testing.T) {
	st := editorState{
		Cursors: []CursorState{
			{Cursor: buffer.Position{Line: 5}},
		},
		ScrollY: 20,
	}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 100})
	if st.ScrollY != 20 {
		t.Errorf("ScrollY=%v want 20 (unchanged)", st.ScrollY)
	}
}

// ---------- integration: key sequence ----------

func TestEditorFactoryBuilds(t *testing.T) {
	v := Editor(EditorCfg{
		IDFocus: 1,
		Buffer:  mkBuf("hello\nworld"),
		Width:   400,
		Height:  200,
	})
	if v == nil {
		t.Fatal("Editor returned nil")
	}
}

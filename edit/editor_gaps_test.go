package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

// ---------- clampCursors ----------

func TestClampCursors_LineOOB(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{Line: 99}},
	}}
	clampCursors(&st, mkBuf("a\nb\nc"))
	if st.Cursors[0].Cursor.Line != 2 {
		t.Errorf("Line=%d want 2", st.Cursors[0].Cursor.Line)
	}
}

func TestClampCursors_ColOOB(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{Line: 0, ByteCol: 99}},
	}}
	clampCursors(&st, mkBuf("abc"))
	if st.Cursors[0].Cursor.ByteCol != 3 {
		t.Errorf("ByteCol=%d want 3", st.Cursors[0].Cursor.ByteCol)
	}
}

func TestClampCursors_NegativeCursor(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{Line: -5, ByteCol: -3}},
	}}
	clampCursors(&st, mkBuf("abc"))
	cs := st.Cursors[0]
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 0 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

func TestClampCursors_EmptyBuffer(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{Line: 3, ByteCol: 7}},
	}}
	clampCursors(&st, buffer.New())
	cs := st.Cursors[0]
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 0 {
		t.Errorf("got %+v", cs.Cursor)
	}
}

// ---------- clampScroll ----------

func TestClampScroll_LargeBuffer(t *testing.T) {
	cfg := EditorCfg{Buffer: mkBuf("a\nb\nc\nd\ne"), Height: 20}
	st := editorState{ScrollY: 1000}
	clampScroll(&st, cfg, &editorFrameData{}, 10) // 5 lines * 10 = 50; 50-20 = 30 max
	if st.ScrollY != 30 {
		t.Errorf("ScrollY=%v want 30", st.ScrollY)
	}
}

func TestClampScroll_BufferFitsInViewport(t *testing.T) {
	cfg := EditorCfg{Buffer: mkBuf("a\nb"), Height: 100}
	st := editorState{ScrollY: 50}
	clampScroll(&st, cfg, &editorFrameData{}, 10)
	if st.ScrollY != 0 {
		t.Errorf("ScrollY=%v want 0", st.ScrollY)
	}
}

func TestClampScroll_NegativeIn(t *testing.T) {
	cfg := EditorCfg{Buffer: mkBuf("a\nb\nc"), Height: 10}
	st := editorState{ScrollY: -50}
	clampScroll(&st, cfg, &editorFrameData{}, 10)
	if st.ScrollY != 0 {
		t.Errorf("ScrollY=%v want 0", st.ScrollY)
	}
}

// ---------- pageLines ----------

func TestPageLines_ExactFit(t *testing.T) {
	fr := &editorFrameData{lineHeight: 10}
	if n := pageLines(fr, 100); n != 10 {
		t.Errorf("got %d want 10", n)
	}
}

func TestPageLines_ZeroLineHeight(t *testing.T) {
	fr := &editorFrameData{lineHeight: 0}
	if n := pageLines(fr, 100); n != 1 {
		t.Errorf("got %d want 1 (safe fallback)", n)
	}
}

func TestPageLines_SubLineViewport(t *testing.T) {
	fr := &editorFrameData{lineHeight: 10}
	if n := pageLines(fr, 5); n != 1 {
		t.Errorf("got %d want 1", n)
	}
}

// ---------- acceptChar ----------

func TestAcceptChar_AllowsPrintableAndTab(t *testing.T) {
	allowed := []rune{'a', 'Z', '5', '!', ' ', '\t', 'é', '日', '€'}
	for _, r := range allowed {
		if !acceptChar(r) {
			t.Errorf("rejected %q", r)
		}
	}
}

func TestAcceptChar_RejectsControl(t *testing.T) {
	rejected := []rune{0, '\n', '\r', '\x01', '\x1f', 0x7f}
	for _, r := range rejected {
		if acceptChar(r) {
			t.Errorf("accepted %q (%U)", r, r)
		}
	}
}

// ---------- movement edges ----------

func TestMoveUp_AtTop(t *testing.T) {
	cs := mkCursor(0, 2)
	moveUp(&cs, mkBuf("abc\ndef"), 1)
	if cs.Cursor.Line != 0 {
		t.Errorf("Line=%d want 0", cs.Cursor.Line)
	}
}

func TestMoveDown_AtBottom(t *testing.T) {
	cs := mkCursor(1, 2)
	moveDown(&cs, mkBuf("abc\ndef"), 1)
	if cs.Cursor.Line != 1 {
		t.Errorf("Line=%d want 1 (clamped)", cs.Cursor.Line)
	}
}

// ---------- edit edges ----------

func TestBackspace_EmptyBufferNoop(t *testing.T) {
	buf := buffer.New()
	cs := mkCursor(0, 0)
	backspace(&cs, buf)
	if buf.String() != "" || cs.Cursor != (buffer.Position{}) {
		t.Errorf("content=%q cursor=%+v", buf.String(), cs.Cursor)
	}
}

func TestDeleteForward_EmptyBufferNoop(t *testing.T) {
	buf := buffer.New()
	cs := mkCursor(0, 0)
	deleteForward(&cs, buf)
	if buf.String() != "" {
		t.Errorf("content=%q", buf.String())
	}
}

func TestInsertNewline_AtLineStart(t *testing.T) {
	buf := mkBuf("hello")
	cs := mkCursor(0, 0)
	insertNewline(EditorCfg{Buffer: buf}, &cs, buf)
	if buf.String() != "\nhello" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 0 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

func TestInsertNewline_AtLineEnd(t *testing.T) {
	buf := mkBuf("hello")
	cs := mkCursor(0, 5)
	insertNewline(EditorCfg{Buffer: buf}, &cs, buf)
	if buf.String() != "hello\n" {
		t.Errorf("content=%q", buf.String())
	}
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 0 {
		t.Errorf("cursor=%+v", cs.Cursor)
	}
}

// ---------- ensureCursorVisible edges ----------

func TestEnsureCursorVisible_TinyViewport(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{Line: 5}},
	}}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 5})
	if st.ScrollY < 0 {
		t.Errorf("ScrollY=%v negative", st.ScrollY)
	}
}

func TestEnsureCursorVisible_InvalidFrame(t *testing.T) {
	st := editorState{
		Cursors: []CursorState{
			{Cursor: buffer.Position{Line: 10}},
		},
		ScrollY: 7,
	}
	fr := &editorFrameData{lineHeight: 10, valid: false}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 100})
	if st.ScrollY != 7 {
		t.Errorf("ScrollY=%v want 7 (unchanged)", st.ScrollY)
	}
}

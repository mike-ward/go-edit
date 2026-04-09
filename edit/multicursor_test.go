package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func pos(line, col int) buffer.Position {
	return buffer.Position{Line: line, ByteCol: col}
}

func cs(line, col int) CursorState {
	p := pos(line, col)
	return CursorState{Cursor: p, Anchor: p, DesiredCol: col}
}

func csSel(curLine, curCol, ancLine, ancCol int) CursorState {
	return CursorState{
		Cursor:     pos(curLine, curCol),
		Anchor:     pos(ancLine, ancCol),
		DesiredCol: curCol,
	}
}

// ---------- sortCursors ----------

func TestSortCursors(t *testing.T) {
	cursors := []CursorState{cs(2, 0), cs(0, 5), cs(1, 3)}
	sortCursors(cursors)
	if cursors[0].Cursor.Line != 0 || cursors[1].Cursor.Line != 1 ||
		cursors[2].Cursor.Line != 2 {
		t.Errorf("sort failed: %+v", cursors)
	}
}

func TestSortCursors_SameLineDifferentCol(t *testing.T) {
	cursors := []CursorState{cs(0, 5), cs(0, 2), cs(0, 8)}
	sortCursors(cursors)
	if cursors[0].Cursor.ByteCol != 2 ||
		cursors[1].Cursor.ByteCol != 5 ||
		cursors[2].Cursor.ByteCol != 8 {
		t.Errorf("sort failed: %+v", cursors)
	}
}

// ---------- mergeCursors ----------

func TestMergeCursors_Disjoint(t *testing.T) {
	cursors := []CursorState{cs(0, 0), cs(0, 5), cs(1, 0)}
	result := mergeCursors(cursors)
	if len(result) != 3 {
		t.Errorf("expected 3 disjoint, got %d", len(result))
	}
}

func TestMergeCursors_Overlapping(t *testing.T) {
	// Two selections that overlap: [0:0, 0:5] and [0:3, 0:8]
	cursors := []CursorState{
		csSel(0, 5, 0, 0),
		csSel(0, 8, 0, 3),
	}
	sortCursors(cursors)
	result := mergeCursors(cursors)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged, got %d", len(result))
	}
	r := result[0].SelectionRange()
	if r.Start != pos(0, 0) || r.End != pos(0, 8) {
		t.Errorf("merged range: %+v", r)
	}
}

func TestMergeCursors_Touching(t *testing.T) {
	// Two cursors at same position should merge.
	cursors := []CursorState{cs(0, 5), cs(0, 5)}
	result := mergeCursors(cursors)
	if len(result) != 1 {
		t.Errorf("expected 1 merged, got %d", len(result))
	}
}

func TestMergeCursors_SingleCursor(t *testing.T) {
	cursors := []CursorState{cs(0, 0)}
	result := mergeCursors(cursors)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

// ---------- addCursor ----------

func TestAddCursor(t *testing.T) {
	st := editorState{Cursors: []CursorState{cs(0, 0)}}
	addCursor(&st, cs(1, 5))
	if len(st.Cursors) != 2 {
		t.Fatalf("expected 2 cursors, got %d", len(st.Cursors))
	}
	// Should be sorted.
	if st.Cursors[0].Cursor.Line != 0 || st.Cursors[1].Cursor.Line != 1 {
		t.Errorf("not sorted: %+v", st.Cursors)
	}
}

func TestAddCursor_MergesOverlap(t *testing.T) {
	st := editorState{Cursors: []CursorState{cs(0, 5)}}
	addCursor(&st, cs(0, 5)) // same position
	if len(st.Cursors) != 1 {
		t.Errorf("expected 1 merged, got %d", len(st.Cursors))
	}
}

func TestAddCursor_CapReached(t *testing.T) {
	st := editorState{Cursors: make([]CursorState, maxCursors)}
	addCursor(&st, cs(0, 0))
	if len(st.Cursors) != maxCursors {
		t.Errorf("expected cap at %d, got %d", maxCursors, len(st.Cursors))
	}
}

// ---------- collapseToPrimary ----------

func TestCollapseToPrimary(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		cs(0, 0), cs(1, 0), cs(2, 0),
	}}
	collapseToPrimary(&st)
	if len(st.Cursors) != 1 {
		t.Errorf("expected 1, got %d", len(st.Cursors))
	}
	if st.Cursors[0].Cursor.Line != 0 {
		t.Errorf("wrong primary: %+v", st.Cursors[0])
	}
}

func TestCollapseToPrimary_Single(t *testing.T) {
	st := editorState{Cursors: []CursorState{cs(5, 3)}}
	collapseToPrimary(&st)
	if len(st.Cursors) != 1 || st.Cursors[0].Cursor.Line != 5 {
		t.Errorf("unexpected: %+v", st.Cursors)
	}
}

// ---------- adjustCursorsAfterEdit ----------

// ---------- overlapsOrTouches ----------

func TestOverlapsOrTouches_Disjoint(t *testing.T) {
	a := buffer.Range{Start: pos(0, 0), End: pos(0, 3)}
	b := buffer.Range{Start: pos(0, 5), End: pos(0, 8)}
	if overlapsOrTouches(a, b) {
		t.Error("disjoint ranges should not overlap")
	}
}

func TestOverlapsOrTouches_Adjacent(t *testing.T) {
	a := buffer.Range{Start: pos(0, 0), End: pos(0, 3)}
	b := buffer.Range{Start: pos(0, 3), End: pos(0, 5)}
	if !overlapsOrTouches(a, b) {
		t.Error("adjacent ranges should touch")
	}
}

func TestOverlapsOrTouches_ZeroWidth(t *testing.T) {
	a := buffer.Range{Start: pos(0, 3), End: pos(0, 3)}
	b := buffer.Range{Start: pos(0, 3), End: pos(0, 3)}
	if !overlapsOrTouches(a, b) {
		t.Error("identical zero-width ranges should touch")
	}
}

// ---------- reversePositionOrder ----------

func TestReversePositionOrder_TwoCursors(t *testing.T) {
	cursors := []CursorState{cs(0, 0), cs(1, 5)}
	idx := reversePositionOrder(cursors)
	if idx[0] != 1 || idx[1] != 0 {
		t.Errorf("got %v want [1,0]", idx)
	}
}

func TestReversePositionOrder_HeapPath(t *testing.T) {
	// >8 cursors triggers heap allocation path.
	cursors := make([]CursorState, 10)
	for i := range cursors {
		cursors[i] = cs(i, 0)
	}
	idx := reversePositionOrder(cursors)
	if idx[0] != 9 || idx[9] != 0 {
		t.Errorf("first=%d last=%d", idx[0], idx[9])
	}
}

func TestReversePositionOrder_Single(t *testing.T) {
	idx := reversePositionOrder([]CursorState{cs(0, 0)})
	if len(idx) != 1 || idx[0] != 0 {
		t.Errorf("got %v", idx)
	}
}

// ---------- findNext ----------

func TestFindNext_Found(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	r, ok := findNext(buf, []byte("world"), pos(0, 0))
	if !ok {
		t.Fatal("should find")
	}
	if r.Start != pos(0, 6) || r.End != pos(0, 11) {
		t.Errorf("range=%+v", r)
	}
}

func TestFindNext_NotFound(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	_, ok := findNext(buf, []byte("xyz"), pos(0, 0))
	if ok {
		t.Error("should not find")
	}
}

func TestFindNext_EmptyNeedle(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	_, ok := findNext(buf, nil, pos(0, 0))
	if ok {
		t.Error("empty needle should return false")
	}
}

func TestFindNext_EmptyBuffer(t *testing.T) {
	buf := buffer.New()
	_, ok := findNext(buf, []byte("x"), pos(0, 0))
	if ok {
		t.Error("should not find in empty buffer")
	}
}

func TestFindNext_WrapAround(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef"))
	// Search from line 1; "abc" is on line 0 → wrap.
	r, ok := findNext(buf, []byte("abc"), pos(1, 0))
	if !ok {
		t.Fatal("should find via wrap")
	}
	if r.Start != pos(0, 0) {
		t.Errorf("start=%+v", r.Start)
	}
}

func TestFindNext_AtEOF(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	// from at end of buffer.
	r, ok := findNext(buf, []byte("abc"), pos(0, 3))
	if !ok {
		t.Fatal("should find via wrap from EOF")
	}
	if r.Start != pos(0, 0) {
		t.Errorf("start=%+v", r.Start)
	}
}

func TestFindNext_UTF8(t *testing.T) {
	buf := buffer.FromBytes([]byte("café résumé"))
	r, ok := findNext(buf, []byte("résumé"), pos(0, 0))
	if !ok {
		t.Fatal("should find UTF-8 needle")
	}
	if r.Start.ByteCol != 6 { // "café " = 6 bytes (é is 2)
		t.Errorf("start col=%d", r.Start.ByteCol)
	}
}

// ---------- collectSelections ----------

func TestCollectSelections_NoSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	st := editorState{Cursors: []CursorState{cs(0, 1)}}
	if s := collectSelections(&st, buf); s != "" {
		t.Errorf("got %q want empty", s)
	}
}

func TestCollectSelections_SingleSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	st := editorState{Cursors: []CursorState{
		csSel(0, 3, 0, 0),
	}}
	if s := collectSelections(&st, buf); s != "hel" {
		t.Errorf("got %q want hel", s)
	}
}

func TestCollectSelections_MultipleSelections(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	st := editorState{Cursors: []CursorState{
		csSel(0, 3, 0, 0),
		csSel(2, 3, 2, 0),
	}}
	if s := collectSelections(&st, buf); s != "aaa\nccc" {
		t.Errorf("got %q", s)
	}
}

// ---------- multiCursorPaste ----------

func TestMultiCursorPaste_BroadcastToAll(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb"))
	st := editorState{Cursors: []CursorState{cs(0, 1), cs(1, 1)}}
	multiCursorPaste(&st, buf, "X")
	if buf.String() != "aX\nbX" {
		t.Errorf("buf=%q", buf.String())
	}
}

func TestMultiCursorPaste_PerCursorSplit(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb"))
	st := editorState{Cursors: []CursorState{cs(0, 1), cs(1, 1)}}
	// 2 cursors, text has 1 newline → 2 lines → per-cursor split.
	multiCursorPaste(&st, buf, "X\nY")
	if buf.String() != "aX\nbY" {
		t.Errorf("buf=%q want aX\\nbY", buf.String())
	}
}

func TestMultiCursorPaste_LineMismatchBroadcasts(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb"))
	st := editorState{Cursors: []CursorState{cs(0, 1), cs(1, 1)}}
	// 2 cursors but 3 lines → broadcast full text.
	multiCursorPaste(&st, buf, "X\nY\nZ")
	if buf.String() != "aX\nY\nZ\nbX\nY\nZ" {
		t.Errorf("buf=%q", buf.String())
	}
}

func TestMultiCursorDeleteSelections_NoSelections(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	st := editorState{Cursors: []CursorState{cs(0, 1), cs(0, 2)}}
	multiCursorDeleteSelections(&st, buf)
	if buf.String() != "abc" {
		t.Errorf("buf=%q want unchanged", buf.String())
	}
}

// ---------- buildUndoCursorState + restoreCursorsFromUndo ----------

func TestUndoCursorStateRoundTrip(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: pos(0, 3), Anchor: pos(0, 0), DesiredCol: 3},
		{Cursor: pos(2, 5), Anchor: pos(2, 1), DesiredCol: 5},
	}}
	ucs := buildUndoCursorState(&st)
	if ucs.Cursor != pos(0, 3) || ucs.Anchor != pos(0, 0) {
		t.Errorf("primary: %+v / %+v", ucs.Cursor, ucs.Anchor)
	}
	if len(ucs.Extra) != 1 {
		t.Fatalf("extra=%d want 1", len(ucs.Extra))
	}
	if ucs.Extra[0].Cursor != pos(2, 5) {
		t.Errorf("extra[0]=%+v", ucs.Extra[0])
	}

	// Restore into a fresh state.
	var st2 editorState
	st2.ensureCursors()
	restoreCursorsFromUndo(&st2, ucs)
	if len(st2.Cursors) != 2 {
		t.Fatalf("restored %d want 2", len(st2.Cursors))
	}
	if st2.Cursors[0].Cursor != pos(0, 3) {
		t.Errorf("restored[0]=%+v", st2.Cursors[0])
	}
	if st2.Cursors[1].Cursor != pos(2, 5) {
		t.Errorf("restored[1]=%+v", st2.Cursors[1])
	}
}

func TestUndoCursorStateRoundTrip_SingleCursor(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: pos(1, 4), Anchor: pos(1, 4)},
	}}
	ucs := buildUndoCursorState(&st)
	if ucs.Extra != nil {
		t.Errorf("extra should be nil for single cursor")
	}
	var st2 editorState
	st2.ensureCursors()
	restoreCursorsFromUndo(&st2, ucs)
	if len(st2.Cursors) != 1 || st2.Cursors[0].Cursor != pos(1, 4) {
		t.Errorf("restored=%+v", st2.Cursors)
	}
}

// ---------- adjustCursorsAfterEdit ----------

func TestAdjustCursors_InsertShiftsLater(t *testing.T) {
	// Two cursors on same line, insert at cursor 0 shifts cursor 1.
	cursors := []CursorState{cs(0, 2), cs(0, 5)}
	c := buffer.Change{
		Applied: buffer.Edit{
			Range:    buffer.Range{Start: pos(0, 2), End: pos(0, 2)},
			NewBytes: []byte("XX"),
		},
		AppliedRange: buffer.Range{Start: pos(0, 2), End: pos(0, 4)},
	}
	adjustCursorsAfterEdit(cursors, 0, c)
	if cursors[1].Cursor.ByteCol != 7 {
		t.Errorf("col=%d want 7", cursors[1].Cursor.ByteCol)
	}
}

func TestAdjustCursors_InsertNewlineShiftsLine(t *testing.T) {
	// Cursor 0 at (0,2), cursor 1 at (0,5). Insert newline at (0,3).
	cursors := []CursorState{cs(0, 2), cs(0, 5)}
	c := buffer.Change{
		Applied: buffer.Edit{
			Range:    buffer.Range{Start: pos(0, 3), End: pos(0, 3)},
			NewBytes: []byte("\n"),
		},
		AppliedRange: buffer.Range{Start: pos(0, 3), End: pos(1, 0)},
	}
	adjustCursorsAfterEdit(cursors, -1, c) // -1 = adjust all
	// Cursor 0 at (0,2): before edit start, unchanged.
	if cursors[0].Cursor != pos(0, 2) {
		t.Errorf("cursor0=%+v", cursors[0].Cursor)
	}
	// Cursor 1 at (0,5): after edit on same line → (1, 5-3=2).
	if cursors[1].Cursor != pos(1, 2) {
		t.Errorf("cursor1=%+v want (1,2)", cursors[1].Cursor)
	}
}

func TestAdjustCursors_DeleteCollapsesInside(t *testing.T) {
	// Delete range [0:2, 0:5], cursor at 0:3 (inside) → endPos.
	cursors := []CursorState{cs(0, 0), cs(0, 3)}
	c := buffer.Change{
		Applied: buffer.Edit{
			Range: buffer.Range{Start: pos(0, 2), End: pos(0, 5)},
		},
		AppliedRange: buffer.Range{Start: pos(0, 2), End: pos(0, 2)},
	}
	adjustCursorsAfterEdit(cursors, -1, c)
	if cursors[0].Cursor != pos(0, 0) {
		t.Errorf("cursor0=%+v", cursors[0].Cursor)
	}
	if cursors[1].Cursor != pos(0, 2) {
		t.Errorf("cursor1=%+v want (0,2)", cursors[1].Cursor)
	}
}

func TestAdjustCursors_DeleteShiftsAfter(t *testing.T) {
	// Delete [0:2, 0:5] (3 bytes), cursor at 0:8 → 0:5.
	cursors := []CursorState{cs(0, 8)}
	c := buffer.Change{
		Applied: buffer.Edit{
			Range: buffer.Range{Start: pos(0, 2), End: pos(0, 5)},
		},
		AppliedRange: buffer.Range{Start: pos(0, 2), End: pos(0, 2)},
	}
	adjustCursorsAfterEdit(cursors, -1, c)
	if cursors[0].Cursor != pos(0, 5) {
		t.Errorf("cursor=%+v want (0,5)", cursors[0].Cursor)
	}
}

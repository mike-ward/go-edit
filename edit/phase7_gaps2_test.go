package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// ---- rangeOverlapsSubRow ----

func TestRangeOverlapsSubRow_FullOverlap(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 0},
		End:   buffer.Position{Line: 0, ByteCol: 20},
	}
	if !rangeOverlapsSubRow(r, 0, 5, 15) {
		t.Fatal("expected overlap")
	}
}

func TestRangeOverlapsSubRow_NoOverlap(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 0},
		End:   buffer.Position{Line: 0, ByteCol: 5},
	}
	if rangeOverlapsSubRow(r, 0, 10, 20) {
		t.Fatal("expected no overlap")
	}
}

func TestRangeOverlapsSubRow_WrongLine(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 1, ByteCol: 0},
		End:   buffer.Position{Line: 1, ByteCol: 10},
	}
	if rangeOverlapsSubRow(r, 0, 0, 20) {
		t.Fatal("expected no overlap on wrong line")
	}
}

func TestRangeOverlapsSubRow_MultiLine(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 5},
		End:   buffer.Position{Line: 2, ByteCol: 3},
	}
	// Middle line is fully covered.
	if !rangeOverlapsSubRow(r, 1, 0, 20) {
		t.Fatal("expected overlap on middle line")
	}
}

func TestRangeOverlapsSubRow_ExactBoundary(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 10},
		End:   buffer.Position{Line: 0, ByteCol: 20},
	}
	// Sub-row [0,10) — range starts exactly at subEnd.
	if rangeOverlapsSubRow(r, 0, 0, 10) {
		t.Fatal("expected no overlap at exact boundary")
	}
	// Sub-row [10,20) — exact match.
	if !rangeOverlapsSubRow(r, 0, 10, 20) {
		t.Fatal("expected overlap on matching sub-row")
	}
}

func TestRangeOverlapsSubRow_PartialOverlap(t *testing.T) {
	r := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 8},
		End:   buffer.Position{Line: 0, ByteCol: 15},
	}
	if !rangeOverlapsSubRow(r, 0, 5, 12) {
		t.Fatal("expected partial overlap")
	}
}

// ---- subRowByteRange ----

func TestSubRowByteRange_NoBreaks(t *testing.T) {
	s, e := subRowByteRange(nil, 0, 25)
	if s != 0 || e != 25 {
		t.Fatalf("got [%d,%d), want [0,25)", s, e)
	}
}

func TestSubRowByteRange_WithBreaks(t *testing.T) {
	breaks := []int{10, 20}
	s, e := subRowByteRange(breaks, 0, 30)
	if s != 0 || e != 10 {
		t.Fatalf("sub0: got [%d,%d), want [0,10)", s, e)
	}
	s, e = subRowByteRange(breaks, 1, 30)
	if s != 10 || e != 20 {
		t.Fatalf("sub1: got [%d,%d), want [10,20)", s, e)
	}
	s, e = subRowByteRange(breaks, 2, 30)
	if s != 20 || e != 30 {
		t.Fatalf("sub2: got [%d,%d), want [20,30)", s, e)
	}
}

func TestSubRowByteRange_NegativeSr(t *testing.T) {
	breaks := []int{10}
	s, e := subRowByteRange(breaks, -1, 20)
	if s != 0 || e != 10 {
		t.Fatalf("got [%d,%d), want [0,10)", s, e)
	}
}

func TestSubRowByteRange_NegativeLineLen(t *testing.T) {
	s, e := subRowByteRange(nil, 0, -5)
	if s != 0 || e != 0 {
		t.Fatalf("got [%d,%d), want [0,0)", s, e)
	}
}

// ---- buildSelInfos ----

func TestBuildSelInfos_NoSelection(t *testing.T) {
	cursors := []CursorState{
		{Cursor: buffer.Position{Line: 0, ByteCol: 5},
			Anchor: buffer.Position{Line: 0, ByteCol: 5}},
	}
	sels := buildSelInfos(cursors)
	if len(sels) != 1 || sels[0].hasSel {
		t.Fatal("expected 1 entry with hasSel=false")
	}
}

func TestBuildSelInfos_WithSelection(t *testing.T) {
	cursors := []CursorState{
		{Cursor: buffer.Position{Line: 0, ByteCol: 10},
			Anchor: buffer.Position{Line: 0, ByteCol: 2}},
	}
	sels := buildSelInfos(cursors)
	if !sels[0].hasSel {
		t.Fatal("expected hasSel=true")
	}
	if sels[0].sel.Start.ByteCol != 2 || sels[0].sel.End.ByteCol != 10 {
		t.Fatalf("got sel %v", sels[0].sel)
	}
}

func TestBuildSelInfos_Empty(t *testing.T) {
	sels := buildSelInfos(nil)
	if len(sels) != 0 {
		t.Fatal("expected empty")
	}
}

func TestBuildSelInfos_MoreThanFour(t *testing.T) {
	// Exercises the heap-alloc path (>4 cursors).
	cursors := make([]CursorState, 6)
	for i := range cursors {
		cursors[i].Cursor = buffer.Position{Line: i, ByteCol: 5}
		cursors[i].Anchor = buffer.Position{Line: i, ByteCol: 0}
	}
	sels := buildSelInfos(cursors)
	if len(sels) != 6 {
		t.Fatalf("got %d, want 6", len(sels))
	}
	for i, s := range sels {
		if !s.hasSel {
			t.Fatalf("sels[%d] should have selection", i)
		}
	}
}

// ---- resolveTabWidth ----

func TestResolveTabWidth_NilMeasurer(t *testing.T) {
	got := resolveTabWidth(nil)
	if got != text.DefaultTabWidth {
		t.Fatalf("got %d, want %d", got, text.DefaultTabWidth)
	}
}

func TestResolveTabWidth_ZeroTabWidth(t *testing.T) {
	m := text.NewFake(8, 16)
	m.TabWidth = 0
	got := resolveTabWidth(m)
	if got != text.DefaultTabWidth {
		t.Fatalf("got %d, want %d", got, text.DefaultTabWidth)
	}
}

func TestResolveTabWidth_CustomTabWidth(t *testing.T) {
	m := text.NewFake(8, 16)
	m.TabWidth = 8
	got := resolveTabWidth(m)
	if got != 8 {
		t.Fatalf("got %d, want 8", got)
	}
}

// ---- wrapLineVisualRowCount ----

func TestWrapLineVisualRowCount_NilMeasurer(t *testing.T) {
	got := wrapLineVisualRowCount([]byte("hello"), nil, 80)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestWrapLineVisualRowCount_EmptyLine(t *testing.T) {
	m := text.NewFake(8, 16)
	got := wrapLineVisualRowCount(nil, m, 80)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestWrapLineVisualRowCount_Fits(t *testing.T) {
	m := text.NewFake(8, 16)
	got := wrapLineVisualRowCount([]byte("short"), m, 800)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestWrapLineVisualRowCount_Wraps(t *testing.T) {
	m := text.NewFake(8, 16)
	// 20 chars * 8px = 160px, wrapWidth 80 → 2 rows
	line := []byte("01234567890123456789")
	got := wrapLineVisualRowCount(line, m, 80)
	if got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestWrapLineVisualRowCount_ZeroWrapWidth(t *testing.T) {
	m := text.NewFake(8, 16)
	got := wrapLineVisualRowCount([]byte("hello"), m, 0)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

// ---- visRangeToLogical ----

func TestVisRangeToLogical_Plain(t *testing.T) {
	f, l := visRangeToLogical(nil, nil, &editorFrameData{},
		nil, false, false, 3, 7)
	if f != 3 || l != 7 {
		t.Fatalf("got (%d,%d), want (3,7)", f, l)
	}
}

func TestVisRangeToLogical_WithFolds(t *testing.T) {
	folds := []FoldRange{{StartLine: 2, EndLine: 4}}
	buf := bufFromLines("a", "b", "c", "d", "e", "f", "g")
	f, l := visRangeToLogical(buf, nil, &editorFrameData{},
		folds, true, false, 0, 3)
	// vis 0→line 0, vis 3→line 5 (fold hides lines 3-4)
	if f != 0 {
		t.Fatalf("first: got %d, want 0", f)
	}
	if l != 5 {
		t.Fatalf("last: got %d, want 5", l)
	}
}

func TestVisRangeToLogical_NilBuf(t *testing.T) {
	f, l := visRangeToLogical(nil, nil, &editorFrameData{},
		nil, false, true, 2, 5)
	if f != 2 || l != 5 {
		t.Fatalf("nil buf: got (%d,%d), want (2,5)", f, l)
	}
}

// ---- visRowToStartLine ----

func TestVisRowToStartLine_Plain(t *testing.T) {
	line, sr := visRowToStartLine(nil, nil, &editorFrameData{},
		nil, false, false, 5)
	if line != 5 || sr != 0 {
		t.Fatalf("got (%d,%d), want (5,0)", line, sr)
	}
}

func TestVisRowToStartLine_WithFolds(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	buf := bufFromLines("a", "b", "c", "d", "e")
	line, sr := visRowToStartLine(buf, nil, &editorFrameData{},
		folds, true, false, 2)
	// vis 0=line0, vis 1=line1(header), vis 2=line4
	if line != 4 || sr != 0 {
		t.Fatalf("got (%d,%d), want (4,0)", line, sr)
	}
}

func TestVisRowToStartLine_NilBuf(t *testing.T) {
	line, sr := visRowToStartLine(nil, nil,
		&editorFrameData{wrapActive: true},
		nil, false, true, 3)
	if line != 3 || sr != 0 {
		t.Fatalf("nil buf: got (%d,%d), want (3,0)", line, sr)
	}
}

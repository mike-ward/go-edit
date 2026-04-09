package edit

import (
	"math"
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// ---- brackets ----

func TestBracketAtCursor_NilBuf(t *testing.T) {
	b, _ := bracketAtCursor(nil, buffer.Position{})
	if b != 0 {
		t.Fatal("expected 0 for nil buf")
	}
}

func TestBracketAtCursor_NegativeLine(t *testing.T) {
	buf := bufFromLines("()")
	b, _ := bracketAtCursor(buf, buffer.Position{Line: -1})
	if b != 0 {
		t.Fatal("expected 0 for negative line")
	}
}

func TestBracketAtCursor_LineOutOfRange(t *testing.T) {
	buf := bufFromLines("()")
	b, _ := bracketAtCursor(buf, buffer.Position{Line: 999})
	if b != 0 {
		t.Fatal("expected 0 for out-of-range line")
	}
}

func TestFindMatchingBracket_NilBuf(t *testing.T) {
	_, ok := findMatchingBracket(nil, buffer.Position{})
	if ok {
		t.Fatal("expected false for nil buf")
	}
}

// ---- autoclose ----

func TestShouldSkipCloser_NilBuf(t *testing.T) {
	if shouldSkipCloser(nil, buffer.Position{}, ')',
		DefaultAutoClosePairs) {
		t.Fatal("expected false for nil buf")
	}
}

func TestShouldDeletePair_NilBuf(t *testing.T) {
	if shouldDeletePair(nil, buffer.Position{Line: 0, ByteCol: 1},
		DefaultAutoClosePairs) {
		t.Fatal("expected false for nil buf")
	}
}

func TestShouldSkipCloser_NegativeLine(t *testing.T) {
	buf := bufFromLines(")")
	if shouldSkipCloser(buf, buffer.Position{Line: -1}, ')',
		DefaultAutoClosePairs) {
		t.Fatal("expected false for negative line")
	}
}

// ---- fold ----

func TestFoldRangeAt_NilBuf(t *testing.T) {
	_, ok := foldRangeAt(nil, 0, 4)
	if ok {
		t.Fatal("expected false for nil buf")
	}
}

func TestFoldRangeAt_NegativeLine(t *testing.T) {
	buf := bufFromLines("a", "  b")
	_, ok := foldRangeAt(buf, -1, 4)
	if ok {
		t.Fatal("expected false for negative line")
	}
}

func TestFoldRangeAt_ZeroTabWidth(t *testing.T) {
	buf := bufFromLines("a {", "\tb", "}")
	// Should not panic; defaults to 4.
	_, _ = foldRangeAt(buf, 0, 0)
}

func TestToggleFold_NilBuf(t *testing.T) {
	folds := toggleFold(nil, nil, 0, 4)
	if len(folds) != 0 {
		t.Fatal("expected empty for nil buf")
	}
}

func TestFoldAll_NilBuf(t *testing.T) {
	folds := foldAll(nil, 4)
	if len(folds) != 0 {
		t.Fatal("expected nil for nil buf")
	}
}

func TestVisibleLineCount_DegenerateFold(t *testing.T) {
	// EndLine < StartLine should not produce negative count.
	folds := []FoldRange{{StartLine: 5, EndLine: 2}}
	got := visibleLineCount(10, folds)
	if got < 1 {
		t.Fatalf("got %d, expected >= 1", got)
	}
}

func TestVisibleToLogical_Negative(t *testing.T) {
	got := visibleToLogical(-5, nil)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestLogicalToVisible_Negative(t *testing.T) {
	got := logicalToVisible(-5, nil)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestSnapCursorOutOfFold_NilCursor(t *testing.T) {
	// Should not panic.
	snapCursorOutOfFold(nil, []FoldRange{{0, 5}})
}

func TestSkipFoldsDown_NilCursor(t *testing.T) {
	skipFoldsDown(nil, []FoldRange{{0, 5}})
}

// ---- wrap ----

func TestComputeBreaks_NilMeasurer(t *testing.T) {
	breaks := computeBreaks([]byte("hello"), nil, 80)
	if len(breaks) != 0 {
		t.Fatal("expected nil for nil measurer")
	}
}

func TestComputeBreaks_NaNWrapWidth(t *testing.T) {
	m := text.NewFake(8, 16)
	nan := float32(math.NaN())
	breaks := computeBreaks([]byte("hello"), m, nan)
	if len(breaks) != 0 {
		t.Fatal("expected nil for NaN wrapWidth")
	}
}

func TestBuildWrapMap_NilBuf(t *testing.T) {
	m := text.NewFake(8, 16)
	wm := buildWrapMap(nil, m, 80, 0, 10, nil)
	if wm != nil {
		t.Fatal("expected nil for nil buf")
	}
}

func TestWrapSubRowRange_NilEntry(t *testing.T) {
	s, e := wrapSubRowRange(nil, 10, 0)
	if s != 0 || e != 10 {
		t.Fatalf("got [%d,%d), want [0,10)", s, e)
	}
}

func TestWrapSubRowRange_NegativeSr(t *testing.T) {
	we := wrapEntry{BreakCols: []int{5}}
	s, e := wrapSubRowRange(&we, 10, -1)
	if s != 0 || e != 5 {
		t.Fatalf("got [%d,%d), want [0,5)", s, e)
	}
}

func TestGlobalLogicalToVisualRow_NilBuf(t *testing.T) {
	got := globalLogicalToVisualRow(nil, nil, 80, nil, 5)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestGlobalVisualRowToLogical_NilBuf(t *testing.T) {
	line, sr := globalVisualRowToLogical(nil, nil, 80, nil, 5)
	if line != 0 || sr != 0 {
		t.Fatalf("got (%d,%d), want (0,0)", line, sr)
	}
}

func TestGlobalVisualRowToLogical_Negative(t *testing.T) {
	line, sr := globalVisualRowToLogical(nil, nil, 80, nil, -1)
	if line != 0 || sr != 0 {
		t.Fatalf("got (%d,%d), want (0,0)", line, sr)
	}
}

func TestTotalVisualRowsForBuffer_NilBuf(t *testing.T) {
	got := totalVisualRowsForBuffer(nil, nil, 80, nil)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestWrapLineHitTest_NilEntry(t *testing.T) {
	got := wrapLineHitTest(nil, []byte("hi"), 0, 10, nil)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

// ---- stickyscroll ----

func TestFindScopeHeaders_NilBuf(t *testing.T) {
	headers := findScopeHeaders(nil, 5, 3, 4)
	if len(headers) != 0 {
		t.Fatal("expected nil for nil buf")
	}
}

func TestFindScopeHeaders_LineOutOfRange(t *testing.T) {
	buf := bufFromLines("a", "b")
	// firstVisibleLine >= LineCount → should clamp, not panic.
	headers := findScopeHeaders(buf, 999, 3, 4)
	if len(headers) != 0 {
		t.Fatalf("expected empty, got %v", headers)
	}
}

func TestFindScopeHeaders_ZeroTabWidth(t *testing.T) {
	buf := bufFromLines("func f() {", "    x")
	// Should not panic; defaults to 4.
	_ = findScopeHeaders(buf, 1, 5, 0)
}

// ---- whitespace ----

func TestResolveWhitespace_HugeOverride(t *testing.T) {
	got := resolveWhitespace(WhitespaceNone, 999)
	if got != WhitespaceNone {
		t.Fatalf("got %d, want WhitespaceNone for huge override",
			got)
	}
}

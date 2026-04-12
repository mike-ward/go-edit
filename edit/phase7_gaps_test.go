package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// ---- wrap.go: wrapVisualRowToLogical ----

func TestWrapVisualRowToLogical(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 0, BreakCols: nil},       // 1 row
			{Line: 1, BreakCols: []int{10}}, // 2 rows
			{Line: 2, BreakCols: nil},       // 1 row
		},
	}
	tests := []struct {
		visRow     int
		wantLine   int
		wantSubRow int
	}{
		{0, 0, 0},
		{1, 1, 0},
		{2, 1, 1},
		{3, 2, 0},
	}
	for _, tt := range tests {
		line, sr := wrapVisualRowToLogical(wm, tt.visRow)
		if line != tt.wantLine || sr != tt.wantSubRow {
			t.Errorf("visRow %d: got (%d,%d), want (%d,%d)",
				tt.visRow, line, sr, tt.wantLine, tt.wantSubRow)
		}
	}
}

func TestWrapVisualRowToLogical_PastEnd(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 0, BreakCols: nil},
		},
	}
	line, _ := wrapVisualRowToLogical(wm, 5)
	if line != 1 {
		t.Errorf("got line %d, want 1 (past end)", line)
	}
}

func TestWrapVisualRowToLogical_NilMap(t *testing.T) {
	line, sr := wrapVisualRowToLogical(nil, 3)
	if line != 3 || sr != 0 {
		t.Errorf("got (%d,%d), want (3,0)", line, sr)
	}
}

// ---- wrap.go: wrapLogicalToVisualRow ----

func TestWrapLogicalToVisualRow(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 0, BreakCols: nil},
			{Line: 1, BreakCols: []int{10}},
			{Line: 2, BreakCols: nil},
		},
	}
	tests := []struct {
		line    int
		wantVis int
	}{
		{0, 0},
		{1, 1},
		{2, 3}, // line 1 takes 2 visual rows
	}
	for _, tt := range tests {
		got := wrapLogicalToVisualRow(wm, tt.line)
		if got != tt.wantVis {
			t.Errorf("line %d: got %d, want %d",
				tt.line, got, tt.wantVis)
		}
	}
}

func TestWrapLogicalToVisualRow_NilMap(t *testing.T) {
	got := wrapLogicalToVisualRow(nil, 5)
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

// ---- wrap.go: wrapMapTotalVisRows ----

func TestWrapMapTotalVisRows(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 0, BreakCols: nil},          // 1
			{Line: 1, BreakCols: []int{10}},    // 2
			{Line: 2, BreakCols: []int{5, 10}}, // 3
		},
	}
	got := wrapMapTotalVisRows(wm)
	if got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
}

func TestWrapMapTotalVisRows_Nil(t *testing.T) {
	if wrapMapTotalVisRows(nil) != 0 {
		t.Fatal("expected 0 for nil")
	}
}

func TestWrapMapTotalVisRows_Empty(t *testing.T) {
	wm := &wrapMap{}
	if wrapMapTotalVisRows(wm) != 0 {
		t.Fatal("expected 0 for empty")
	}
}

// ---- wrap.go: wrapEntryForLine ----

func TestWrapEntryForLine(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 2, BreakCols: []int{10}},
			{Line: 5, BreakCols: nil},
		},
	}
	we := wrapEntryForLine(wm, 2)
	if we == nil || we.Line != 2 {
		t.Fatal("expected entry for line 2")
	}
	we = wrapEntryForLine(wm, 5)
	if we == nil || we.Line != 5 {
		t.Fatal("expected entry for line 5")
	}
}

func TestWrapEntryForLine_NotFound(t *testing.T) {
	wm := &wrapMap{
		entries: []wrapEntry{
			{Line: 2, BreakCols: nil},
		},
	}
	if wrapEntryForLine(wm, 0) != nil {
		t.Fatal("expected nil for missing line")
	}
	if wrapEntryForLine(wm, 99) != nil {
		t.Fatal("expected nil for out-of-range line")
	}
}

func TestWrapEntryForLine_NilMap(t *testing.T) {
	if wrapEntryForLine(nil, 0) != nil {
		t.Fatal("expected nil for nil map")
	}
}

// ---- wrap.go: wrapLineHitTest positive path ----

func TestWrapLineHitTest_PositivePath(t *testing.T) {
	m := text.NewFake(8, 16)
	line := []byte("0123456789abcdef")
	we := &wrapEntry{Line: 0, BreakCols: []int{10}}

	// Sub-row 0: bytes [0,10), sub-row 1: bytes [10,16)
	// Click x=8 on sub-row 0 → byte col 1
	col := wrapLineHitTest(we, line, 0, 8, m)
	if col < 0 || col > 10 {
		t.Fatalf("sub0 x=8: col=%d out of range [0,10)", col)
	}

	// Click x=0 on sub-row 1 → byte col 10
	col = wrapLineHitTest(we, line, 1, 0, m)
	if col < 10 {
		t.Fatalf("sub1 x=0: col=%d, want >= 10", col)
	}
}

// ---- wrap.go: totalVisualRowsForBuffer positive ----

func TestTotalVisualRows_WithWrapping(t *testing.T) {
	m := text.NewFake(8, 16)
	buf := bufFromLines(
		"short",                // 1 row
		"01234567890123456789", // 20 chars → 160px, wraps at 80 → 2 rows
		"tiny",                 // 1 row
	)
	got := totalVisualRowsForBuffer(buf, m, 80, nil)
	if got != 4 {
		t.Fatalf("got %d, want 4", got)
	}
}

func TestTotalVisualRows_NoWrap(t *testing.T) {
	m := text.NewFake(8, 16)
	buf := bufFromLines("a", "b", "c")
	got := totalVisualRowsForBuffer(buf, m, 800, nil)
	if got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

// ---- fold.go: unfoldAt ----

func TestUnfoldAt_RemovesContaining(t *testing.T) {
	folds := []FoldRange{
		{StartLine: 1, EndLine: 3},
		{StartLine: 5, EndLine: 7},
	}
	// Unfold line 2 (inside first fold).
	result := unfoldAt(folds, 2)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].StartLine != 5 {
		t.Fatalf("wrong fold: %+v", result[0])
	}
}

func TestUnfoldAt_Header(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	result := unfoldAt(folds, 1)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

func TestUnfoldAt_NoMatch(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	result := unfoldAt(folds, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

// ---- fold.go: skipFoldsDown behavior ----

func TestSkipFoldsDown_JumpsPastFold(t *testing.T) {
	folds := []FoldRange{{StartLine: 2, EndLine: 5}}
	cs := CursorState{
		Cursor: buffer.Position{Line: 3, ByteCol: 4},
	}
	skipFoldsDown(&cs, folds)
	if cs.Cursor.Line != 6 {
		t.Fatalf("got line %d, want 6", cs.Cursor.Line)
	}
}

func TestSkipFoldsDown_NotInFold(t *testing.T) {
	folds := []FoldRange{{StartLine: 2, EndLine: 5}}
	cs := CursorState{
		Cursor: buffer.Position{Line: 1},
	}
	skipFoldsDown(&cs, folds)
	if cs.Cursor.Line != 1 {
		t.Fatalf("got line %d, want 1 (unchanged)", cs.Cursor.Line)
	}
}

// ---- fold.go: lineIndent with tabs ----

func TestLineIndent_Tabs(t *testing.T) {
	// One tab at tabWidth=4 → 4 visual columns.
	got := lineIndent([]byte("\tx"), 4)
	if got != 4 {
		t.Fatalf("got %d, want 4", got)
	}
}

func TestLineIndent_MixedTabsSpaces(t *testing.T) {
	// Tab + 2 spaces at tabWidth=4 → 4+2 = 6.
	got := lineIndent([]byte("\t  x"), 4)
	if got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
}

func TestLineIndent_Empty(t *testing.T) {
	got := lineIndent(nil, 4)
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

// ---- fold.go: isFoldHeader ----

func TestIsFoldHeader_True(t *testing.T) {
	folds := []FoldRange{{StartLine: 3, EndLine: 7}}
	if !isFoldHeader(folds, 3) {
		t.Fatal("expected true for fold header")
	}
}

func TestIsFoldHeader_False(t *testing.T) {
	folds := []FoldRange{{StartLine: 3, EndLine: 7}}
	if isFoldHeader(folds, 4) {
		t.Fatal("expected false for non-header")
	}
	if isFoldHeader(folds, 0) {
		t.Fatal("expected false for line before fold")
	}
}

func TestIsFoldHeader_Empty(t *testing.T) {
	if isFoldHeader(nil, 0) {
		t.Fatal("expected false for nil folds")
	}
}

// ---- wrap.go: resolveBoolOverride (was resolveStickyScroll) ----

func TestResolveBoolOverride_Sticky(t *testing.T) {
	if resolveBoolOverride(false, 0) {
		t.Error("default false, no override")
	}
	if !resolveBoolOverride(true, 0) {
		t.Error("default true, no override")
	}
	if !resolveBoolOverride(false, 1) {
		t.Error("override on")
	}
	if resolveBoolOverride(true, 2) {
		t.Error("override off")
	}
}

// ---- editor_draw.go: matchCountStr ----

func TestMatchCountStr_EmptyQuery(t *testing.T) {
	ss := &searchState{Query: ""}
	if got := matchCountStr(ss); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestMatchCountStr_NoResults(t *testing.T) {
	ss := &searchState{Query: "foo", Matches: nil}
	if got := matchCountStr(ss); got != "No results" {
		t.Fatalf("got %q, want 'No results'", got)
	}
}

func TestMatchCountStr_Normal(t *testing.T) {
	ss := &searchState{
		Query:        "x",
		Matches:      make([]buffer.Range, 5),
		CurrentMatch: 2,
	}
	got := matchCountStr(ss)
	if got != "3 of 5" {
		t.Fatalf("got %q, want '3 of 5'", got)
	}
}

func TestMatchCountStr_MaxMatches(t *testing.T) {
	ss := &searchState{
		Query:        "x",
		Matches:      make([]buffer.Range, maxMatches),
		CurrentMatch: 0,
	}
	got := matchCountStr(ss)
	if got != "1 of 10000+" {
		t.Fatalf("got %q, want '1 of 10000+'", got)
	}
}

func TestMatchCountStr_NegativeCurrentMatch(t *testing.T) {
	ss := &searchState{
		Query:        "x",
		Matches:      make([]buffer.Range, 3),
		CurrentMatch: -1,
	}
	got := matchCountStr(ss)
	// max(-1+1, 1) = 1
	if got != "1 of 3" {
		t.Fatalf("got %q, want '1 of 3'", got)
	}
}

// ---- autoclose.go: shouldDeletePair at EOL ----

func TestShouldDeletePair_AtEOL(t *testing.T) {
	buf := bufFromLines("(")
	// Cursor at ByteCol=1, which is past the only char.
	pos := buffer.Position{Line: 0, ByteCol: 1}
	if shouldDeletePair(buf, pos, DefaultAutoClosePairs) {
		t.Fatal("should not delete pair when cursor at EOL")
	}
}

// ---- fold+wrap cross-feature ----

func TestFoldPlusWrap_VisualRowMapping(t *testing.T) {
	m := text.NewFake(8, 16)
	buf := bufFromLines(
		"short",                // line 0: 1 vis row
		"func f() {",           // line 1: fold header, 1 vis row
		"    body1",            // line 2: folded (hidden)
		"    body2",            // line 3: folded (hidden)
		"}",                    // line 4: 1 vis row (not folded)
		"01234567890123456789", // line 5: 2 vis rows (wraps at 80px)
	)
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}

	// Visible lines: 0, 1 (header), 4, 5
	// Visual rows:   0, 1,          2, 3+4

	// line 0 → vis row 0
	vr := globalLogicalToVisualRow(buf, m, 80, folds, 0)
	if vr != 0 {
		t.Errorf("line 0 → visRow %d, want 0", vr)
	}

	// line 1 (fold header) → vis row 1
	vr = globalLogicalToVisualRow(buf, m, 80, folds, 1)
	if vr != 1 {
		t.Errorf("line 1 → visRow %d, want 1", vr)
	}

	// line 4 → vis row 2
	vr = globalLogicalToVisualRow(buf, m, 80, folds, 4)
	if vr != 2 {
		t.Errorf("line 4 → visRow %d, want 2", vr)
	}

	// line 5 → vis row 3
	vr = globalLogicalToVisualRow(buf, m, 80, folds, 5)
	if vr != 3 {
		t.Errorf("line 5 → visRow %d, want 3", vr)
	}

	// Reverse: vis row 2 → line 4
	line, sr := globalVisualRowToLogical(buf, m, 80, folds, 2)
	if line != 4 || sr != 0 {
		t.Errorf("visRow 2 → (%d,%d), want (4,0)", line, sr)
	}

	// vis row 4 → line 5 sub-row 1
	line, sr = globalVisualRowToLogical(buf, m, 80, folds, 4)
	if line != 5 || sr != 1 {
		t.Errorf("visRow 4 → (%d,%d), want (5,1)", line, sr)
	}

	// Total visual rows: 1 + 1 + 1 + 2 = 5
	total := totalVisualRowsForBuffer(buf, m, 80, folds)
	if total != 5 {
		t.Errorf("total vis rows = %d, want 5", total)
	}
}

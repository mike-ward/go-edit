package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// fakeMeasurer returns a measurer with 8px advance for testing.
func fakeMeasurer() *text.Measurer {
	return text.NewFake(8, 16)
}

func TestComputeBreaks_NoWrap(t *testing.T) {
	m := fakeMeasurer()
	// "hello" = 5 chars * 8px = 40px, wrapWidth = 100
	breaks := computeBreaks([]byte("hello"), m, 100)
	if len(breaks) != 0 {
		t.Fatalf("expected no breaks, got %v", breaks)
	}
}

func TestComputeBreaks_SingleWrap(t *testing.T) {
	m := fakeMeasurer()
	// 20 chars * 8px = 160px, wrapWidth = 80
	line := []byte("01234567890123456789")
	breaks := computeBreaks(line, m, 80)
	if len(breaks) != 1 {
		t.Fatalf("expected 1 break, got %v", breaks)
	}
	if breaks[0] != 10 {
		t.Fatalf("break at %d, want 10", breaks[0])
	}
}

func TestComputeBreaks_MultiWrap(t *testing.T) {
	m := fakeMeasurer()
	// 30 chars * 8px = 240px, wrapWidth = 80
	line := []byte("012345678901234567890123456789")
	breaks := computeBreaks(line, m, 80)
	if len(breaks) != 2 {
		t.Fatalf("expected 2 breaks, got %v", breaks)
	}
}

func TestComputeBreaks_WordBreak(t *testing.T) {
	m := fakeMeasurer()
	// "hello world test" = 16 chars * 8 = 128px, wrapWidth = 80
	line := []byte("hello world test")
	breaks := computeBreaks(line, m, 80)
	if len(breaks) < 1 {
		t.Fatalf("expected breaks, got none")
	}
	// Should break after "hello " (col 6) or at the space.
	if breaks[0] > 11 {
		t.Fatalf("break at %d, expected word break before 11",
			breaks[0])
	}
}

func TestComputeBreaks_Empty(t *testing.T) {
	m := fakeMeasurer()
	breaks := computeBreaks(nil, m, 80)
	if len(breaks) != 0 {
		t.Fatalf("expected no breaks for empty line")
	}
}

func TestWrapSubRowRange(t *testing.T) {
	we := wrapEntry{Line: 0, BreakCols: []int{10, 20}}

	start, end := wrapSubRowRange(&we, 25, 0)
	if start != 0 || end != 10 {
		t.Fatalf("sub 0: got [%d,%d), want [0,10)", start, end)
	}
	start, end = wrapSubRowRange(&we, 25, 1)
	if start != 10 || end != 20 {
		t.Fatalf("sub 1: got [%d,%d), want [10,20)", start, end)
	}
	start, end = wrapSubRowRange(&we, 25, 2)
	if start != 20 || end != 25 {
		t.Fatalf("sub 2: got [%d,%d), want [20,25)", start, end)
	}
}

func TestWrapCursorVisualRow(t *testing.T) {
	we := wrapEntry{Line: 0, BreakCols: []int{10, 20}}
	if got := wrapCursorVisualRow(&we, 5); got != 0 {
		t.Fatalf("col 5: got %d, want 0", got)
	}
	if got := wrapCursorVisualRow(&we, 10); got != 1 {
		t.Fatalf("col 10: got %d, want 1", got)
	}
	if got := wrapCursorVisualRow(&we, 15); got != 1 {
		t.Fatalf("col 15: got %d, want 1", got)
	}
	if got := wrapCursorVisualRow(&we, 25); got != 2 {
		t.Fatalf("col 25: got %d, want 2", got)
	}
}

func TestResolveWrap(t *testing.T) {
	if resolveWrap(false, 0) {
		t.Error("default false, no override")
	}
	if !resolveWrap(true, 0) {
		t.Error("default true, no override")
	}
	if !resolveWrap(false, 1) {
		t.Error("override on")
	}
	if resolveWrap(true, 2) {
		t.Error("override off")
	}
}

func TestBuildWrapMap(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines(
		"short",
		"01234567890123456789", // 20 chars, wraps at 80px
		"tiny",
	)
	wm := buildWrapMap(buf, m, 80, 0, 2, nil)
	if wm == nil {
		t.Fatal("nil wrapMap")
	}
	if len(wm.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(wm.entries))
	}
	// Line 0: no break
	if wm.entries[0].subRows() != 1 {
		t.Errorf("line 0: %d sub-rows", wm.entries[0].subRows())
	}
	// Line 1: should wrap
	if wm.entries[1].subRows() < 2 {
		t.Errorf("line 1: %d sub-rows, want >=2",
			wm.entries[1].subRows())
	}
	// Line 2: no break
	if wm.entries[2].subRows() != 1 {
		t.Errorf("line 2: %d sub-rows", wm.entries[2].subRows())
	}
}

func TestGlobalVisualRowToLogical(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines(
		"short",                // 1 row
		"01234567890123456789", // 2 rows at 80px
		"tiny",                 // 1 row
	)
	tests := []struct {
		visRow int
		line   int
		subRow int
	}{
		{0, 0, 0},
		{1, 1, 0},
		{2, 1, 1},
		{3, 2, 0},
	}
	for _, tt := range tests {
		line, sr := globalVisualRowToLogical(
			buf, m, 80, nil, tt.visRow)
		if line != tt.line || sr != tt.subRow {
			t.Errorf("visRow %d: got (%d,%d), want (%d,%d)",
				tt.visRow, line, sr, tt.line, tt.subRow)
		}
	}
}

func TestGlobalLogicalToVisualRow(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines(
		"short",                // 1 row
		"01234567890123456789", // 2 rows at 80px
		"tiny",                 // 1 row
	)
	tests := []struct {
		line   int
		visRow int
	}{
		{0, 0},
		{1, 1},
		{2, 3},
	}
	for _, tt := range tests {
		got := globalLogicalToVisualRow(buf, m, 80, nil, tt.line)
		if got != tt.visRow {
			t.Errorf("line %d: got %d, want %d",
				tt.line, got, tt.visRow)
		}
	}
}

// TestVisRowsDelta_MatchesFullWalk feeds a random sequence of
// edits through applyVisRowsDelta and compares the incrementally
// maintained totalVisRows against totalVisualRowsForBuffer on the
// mutated buffer. Any drift between the two is a W6 bug.
func TestVisRowsDelta_MatchesFullWalk(t *testing.T) {
	m := fakeMeasurer()
	const wrapWidth float32 = 80

	buf := buffer.FromBytes([]byte(
		"short\n" +
			"medium line that might wrap\n" +
			"01234567890123456789012345678901234567890123456789\n" +
			"tail\n" +
			"last"))

	frame := &editorFrameData{}
	frame.state.Measurer = m
	frame.wrapWidth = wrapWidth
	frame.totalVisRows, frame.lineRowsCache = buildLineRowsCache(
		buf, m, wrapWidth, nil, nil)
	frame.visRowsCacheLines = buf.LineCount()
	frame.visRowsCacheWidth = wrapWidth

	// Attach the delta observer.
	removeObs := buf.OnEdit(func(c buffer.Change) {
		applyVisRowsDelta(buf, frame, c)
	})
	defer removeObs()

	// Edit sequence: insert, delete, insert newline, delete
	// newline, append at end.
	pos := func(l, c int) buffer.Position {
		return buffer.Position{Line: l, ByteCol: c}
	}
	edits := []buffer.Edit{
		{ // insert mid-line 0
			Range:    buffer.Range{Start: pos(0, 2), End: pos(0, 2)},
			NewBytes: []byte("XYZ"),
		},
		{ // delete chars on line 1
			Range:    buffer.Range{Start: pos(1, 3), End: pos(1, 5)},
			NewBytes: nil,
		},
		{ // insert newline splitting line 2
			Range:    buffer.Range{Start: pos(2, 10), End: pos(2, 10)},
			NewBytes: []byte("\n"),
		},
		{ // join two lines
			Range:    buffer.Range{Start: pos(0, 8), End: pos(1, 0)},
			NewBytes: nil,
		},
		{ // insert multi-line block
			Range:    buffer.Range{Start: pos(0, 0), End: pos(0, 0)},
			NewBytes: []byte("alpha\nbeta\ngamma\n"),
		},
	}

	for step, e := range edits {
		buf.Apply(e)
		if frame.visRowsDirty {
			// Observer bailed out (cache unsafe); skip
			// differential for this step — next amend would
			// rebuild.
			frame.totalVisRows, frame.lineRowsCache = buildLineRowsCache(
				buf, m, wrapWidth, nil, frame.lineRowsCache)
			frame.visRowsDirty = false
			continue
		}
		want := totalVisualRowsForBuffer(buf, m, wrapWidth, nil)
		if frame.totalVisRows != want {
			t.Fatalf("step %d: incremental=%d full=%d",
				step, frame.totalVisRows, want)
		}
		if len(frame.lineRowsCache) != buf.LineCount() {
			t.Fatalf("step %d: cache len %d, line count %d",
				step, len(frame.lineRowsCache), buf.LineCount())
		}
	}
}

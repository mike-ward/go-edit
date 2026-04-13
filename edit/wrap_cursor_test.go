package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// wrapWidth 80px with 8px advance = 10 chars per visual row.

func TestMoveDownVisual_SameLineSubRow(t *testing.T) {
	m := fakeMeasurer() // 8px advance
	// 20 chars → 2 sub-rows at wrapWidth=80 (10 chars each).
	buf := bufFromLines("01234567890123456789")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 3},
		DesiredX: m.XForColumn(buf.Line(0), 3), // 24px
	}
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 {
		t.Fatalf("line = %d, want 0", cs.Cursor.Line)
	}
	// Sub-row 1 starts at byte 10. DesiredX=24px → 3 chars into
	// sub-row → byte 13.
	if cs.Cursor.ByteCol != 13 {
		t.Fatalf("col = %d, want 13", cs.Cursor.ByteCol)
	}
}

func TestMoveUpVisual_SameLineSubRow(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("01234567890123456789")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 13},
		DesiredX: m.XForColumn(buf.Line(0)[10:], 3), // 24px
	}
	moveUpVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 {
		t.Fatalf("line = %d, want 0", cs.Cursor.Line)
	}
	// Sub-row 0: DesiredX=24px → byte 3.
	if cs.Cursor.ByteCol != 3 {
		t.Fatalf("col = %d, want 3", cs.Cursor.ByteCol)
	}
}

func TestMoveDownVisual_CrossLine(t *testing.T) {
	m := fakeMeasurer()
	// Line 0: 20 chars → 2 sub-rows. Line 1: 5 chars → 1 sub-row.
	buf := bufFromLines("01234567890123456789", "hello")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 12},
		DesiredX: m.XForColumn(buf.Line(0)[10:], 2), // 16px
	}
	// Currently on sub-row 1 (last). Down → line 1, sub-row 0.
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 1 {
		t.Fatalf("line = %d, want 1", cs.Cursor.Line)
	}
	// DesiredX=16px → 2 chars → byte 2.
	if cs.Cursor.ByteCol != 2 {
		t.Fatalf("col = %d, want 2", cs.Cursor.ByteCol)
	}
}

func TestMoveUpVisual_CrossLine(t *testing.T) {
	m := fakeMeasurer()
	// Line 0: 20 chars → 2 sub-rows. Line 1: 5 chars → 1 sub-row.
	buf := bufFromLines("01234567890123456789", "hello")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 1, ByteCol: 2},
		DesiredX: m.XForColumn(buf.Line(1), 2), // 16px
	}
	// Up from line 1 sub-row 0 → line 0 last sub-row (sub-row 1).
	moveUpVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 {
		t.Fatalf("line = %d, want 0", cs.Cursor.Line)
	}
	// Sub-row 1 starts at byte 10. DesiredX=16px → 2 chars → byte 12.
	if cs.Cursor.ByteCol != 12 {
		t.Fatalf("col = %d, want 12", cs.Cursor.ByteCol)
	}
}

func TestDesiredX_Preserved(t *testing.T) {
	m := fakeMeasurer()
	// Line 0: 20 chars (2 sub-rows). Line 1: 20 chars (2 sub-rows).
	buf := bufFromLines("01234567890123456789", "abcdefghijklmnopqrst")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 5},
		DesiredX: m.XForColumn(buf.Line(0), 5), // 40px
	}
	// Down 4 times: sub-row 0→1 (same line), 1→line1:0, 0→1, 1→end.
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 15 {
		t.Fatalf("step 1: (%d,%d) want (0,15)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 5 {
		t.Fatalf("step 2: (%d,%d) want (1,5)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 15 {
		t.Fatalf("step 3: (%d,%d) want (1,15)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
}

func TestDesiredX_Clamp(t *testing.T) {
	m := fakeMeasurer()
	// Line 0: 20 chars → 2 sub-rows. Line 1: 3 chars → 1 sub-row.
	buf := bufFromLines("01234567890123456789", "abc")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 17},
		DesiredX: m.XForColumn(buf.Line(0)[10:], 7), // 56px
	}
	// On sub-row 1 (last). Down → line 1 sub-row 0.
	// DesiredX=56px → 7 chars, but line 1 only has 3 → clamp to 3.
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 1 {
		t.Fatalf("line = %d, want 1", cs.Cursor.Line)
	}
	if cs.Cursor.ByteCol != 3 {
		t.Fatalf("col = %d, want 3", cs.Cursor.ByteCol)
	}
}

func TestMoveUpVisual_WithFolds(t *testing.T) {
	m := fakeMeasurer()
	// Line 0: short. Line 1: short (folded with 2). Line 2: short.
	// Line 3: short.
	buf := bufFromLines("aaa", "bbb", "ccc", "ddd")
	folds := []FoldRange{{StartLine: 1, EndLine: 2}}
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 3, ByteCol: 1},
		DesiredX: m.XForColumn(buf.Line(3), 1), // 8px
	}
	// Up from line 3 → should skip fold, land on line 1
	// (fold header).
	moveUpVisual(cs, buf, m, 80, folds)
	if cs.Cursor.Line != 1 {
		t.Fatalf("line = %d, want 1", cs.Cursor.Line)
	}
	if cs.Cursor.ByteCol != 1 {
		t.Fatalf("col = %d, want 1", cs.Cursor.ByteCol)
	}
}

func TestMoveDownVisual_WithFolds(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("aaa", "bbb", "ccc", "ddd")
	folds := []FoldRange{{StartLine: 1, EndLine: 2}}
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 1},
		DesiredX: m.XForColumn(buf.Line(0), 1),
	}
	// Down from line 0 → line 1 (fold header, visible).
	moveDownVisual(cs, buf, m, 80, folds)
	if cs.Cursor.Line != 1 {
		t.Fatalf("line = %d, want 1", cs.Cursor.Line)
	}
	// Down again → should skip to line 3 (past fold end).
	moveDownVisual(cs, buf, m, 80, folds)
	if cs.Cursor.Line != 3 {
		t.Fatalf("line = %d, want 3", cs.Cursor.Line)
	}
}

func TestNoWrap_MoveUpDown_Unchanged(t *testing.T) {
	buf := bufFromLines("aaa", "bbb", "ccc")
	cs := &CursorState{
		Cursor:     buffer.Position{Line: 1, ByteCol: 2},
		DesiredCol: 2,
	}
	moveUp(cs, buf, 1)
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 2 {
		t.Fatalf("moveUp: (%d,%d) want (0,2)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
	moveDown(cs, buf, 1)
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 2 {
		t.Fatalf("moveDown: (%d,%d) want (1,2)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
}

func TestCursorDesiredX(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("01234567890123456789") // 20 chars, 2 sub-rows
	// Cursor at byte 13 → sub-row 1 (start=10). Visual offset = 3 * 8 = 24.
	cs := &CursorState{Cursor: buffer.Position{Line: 0, ByteCol: 13}}
	got := cursorDesiredX(cs, buf, m, 80)
	want := float32(24) // (13-10) * 8px
	if got != want {
		t.Fatalf("cursorDesiredX = %v, want %v", got, want)
	}
}

func TestDesiredX_LazyInit(t *testing.T) {
	m := fakeMeasurer()
	// 20 chars → 2 sub-rows at wrapWidth=80.
	buf := bufFromLines("01234567890123456789", "abcdefghij")
	// Simulate post-typing state: DesiredX=0, cursor at byte 5.
	cs := &CursorState{
		Cursor:     buffer.Position{Line: 0, ByteCol: 5},
		DesiredCol: 5,
		DesiredX:   0, // stale — simulates charInsertPerCursor reset
	}
	moveDownVisual(cs, buf, m, 80, nil)
	// Should lazy-init DesiredX to 40px (5*8), then land on
	// sub-row 1 at byte 15 (10 + 5).
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 15 {
		t.Fatalf("lazy init down: (%d,%d) want (0,15)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
}

func TestMoveUpVisual_AtTopStays(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("hello")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 3},
		DesiredX: m.XForColumn(buf.Line(0), 3),
	}
	moveUpVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 0 {
		t.Fatalf("at top: (%d,%d) want (0,0)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
}

func TestMoveDownVisual_AtBottomStays(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("hello")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 3},
		DesiredX: m.XForColumn(buf.Line(0), 3),
	}
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line != 0 || cs.Cursor.ByteCol != 5 {
		t.Fatalf("at bottom: (%d,%d) want (0,5)",
			cs.Cursor.Line, cs.Cursor.ByteCol)
	}
}

// --- wrapAwareUpDown branching ---

func TestWrapAwareUpDown_BranchesOnWrapActive(t *testing.T) {
	buf := bufFromLines(
		"01234567890123456789", // 20 chars → 2 sub-rows at 80px
		"abcdefghij",
	)
	m := fakeMeasurer()
	st := editorState{Measurer: m}
	st.ensureCursors()
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 3}
	st.Cursors[0].DesiredX = m.XForColumn(buf.Line(0), 3)

	// Wrap OFF: down should jump to line 1 (logical).
	frameOff := &editorFrameData{wrapActive: false}
	a := wrapAwareUpDown("cursor.down", false, frameOff)
	a.Execute(EditorCfg{}, &st, buf, nil)
	if st.Cursors[0].Cursor.Line != 1 {
		t.Fatalf("wrap off: line=%d want 1",
			st.Cursors[0].Cursor.Line)
	}

	// Reset cursor.
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 3}
	st.Cursors[0].DesiredCol = 3
	st.Cursors[0].DesiredX = m.XForColumn(buf.Line(0), 3)

	// Wrap ON: down should stay on line 0, sub-row 1.
	frameOn := &editorFrameData{
		wrapActive: true,
		wrapWidth:  80,
	}
	a2 := wrapAwareUpDown("cursor.down", false, frameOn)
	a2.Execute(EditorCfg{}, &st, buf, nil)
	if st.Cursors[0].Cursor.Line != 0 {
		t.Fatalf("wrap on: line=%d want 0",
			st.Cursors[0].Cursor.Line)
	}
	if st.Cursors[0].Cursor.ByteCol != 13 {
		t.Fatalf("wrap on: col=%d want 13",
			st.Cursors[0].Cursor.ByteCol)
	}
}

// --- pageAction wrap path ---

func TestPageAction_WrapMovesVisualRows(t *testing.T) {
	m := fakeMeasurer()
	// 30 chars → 3 sub-rows at 80px. Second line short.
	buf := bufFromLines(
		"012345678901234567890123456789",
		"abc",
	)
	st := editorState{Measurer: m}
	st.ensureCursors()
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 3}
	st.Cursors[0].DesiredX = m.XForColumn(buf.Line(0), 3)

	frame := &editorFrameData{
		wrapActive: true,
		wrapWidth:  80,
		lineHeight: 16,
	}
	// Page size = 32px / 16px = 2 visual rows.
	a := pageAction("cursor.pagedown", moveDown, false,
		EditorCfg{Height: 32}, frame)
	a.Execute(EditorCfg{}, &st, buf, nil)
	// 2 visual rows down from sub-row 0: → sub-row 1, → sub-row 2.
	if st.Cursors[0].Cursor.Line != 0 {
		t.Fatalf("line=%d want 0", st.Cursors[0].Cursor.Line)
	}
	// Sub-row 2 starts at byte 20. DesiredX=24px → 3 chars → byte 23.
	if st.Cursors[0].Cursor.ByteCol != 23 {
		t.Fatalf("col=%d want 23",
			st.Cursors[0].Cursor.ByteCol)
	}
}

// --- applyPostAction DesiredX ---

func TestApplyPostAction_SetsDesiredX_WhenWrapActive(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("01234567890123456789")
	st := editorState{Measurer: m}
	st.ensureCursors()
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 5}
	frame := &editorFrameData{
		wrapActive: true,
		wrapWidth:  80,
	}
	action := Action{PreservesDesiredCol: false}
	applyPostAction(&st, action, buf, frame)
	// Byte 5 on sub-row 0 → X = 5*8 = 40.
	if st.Cursors[0].DesiredX != 40 {
		t.Fatalf("DesiredX=%v want 40",
			st.Cursors[0].DesiredX)
	}
}

func TestApplyPostAction_ClearsDesiredX_WhenNoWrap(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("hello")
	st := editorState{Measurer: m}
	st.ensureCursors()
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 3}
	st.Cursors[0].DesiredX = 99 // stale value
	frame := &editorFrameData{wrapActive: false}
	action := Action{PreservesDesiredCol: false}
	applyPostAction(&st, action, buf, frame)
	if st.Cursors[0].DesiredX != 0 {
		t.Fatalf("DesiredX=%v want 0",
			st.Cursors[0].DesiredX)
	}
}

// --- edge cases ---

func TestMoveDownVisual_ByteColPastLineEnd(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("01234567890123456789", "abc")
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 0, ByteCol: 99},
		DesiredX: 0, // will lazy-init
	}
	// Should not panic; ByteCol past end treated as last sub-row.
	moveDownVisual(cs, buf, m, 80, nil)
	if cs.Cursor.Line < 0 || cs.Cursor.Line >= buf.LineCount() {
		t.Fatalf("line out of range: %d", cs.Cursor.Line)
	}
}

func TestMoveUpVisual_DegenerateFoldAtCurrentLine(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("aaa", "bbb", "ccc")
	// Degenerate fold: startLine == endLine == 1.
	folds := []FoldRange{{StartLine: 1, EndLine: 1}}
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 2, ByteCol: 1},
		DesiredX: 8,
	}
	moveUpVisual(cs, buf, m, 80, folds)
	// prevVisible(folds, 1) returns 1 (fold header is visible).
	if cs.Cursor.Line != 1 {
		t.Fatalf("line=%d want 1", cs.Cursor.Line)
	}
}

func TestHitSubRow_NegativeDesiredX(t *testing.T) {
	m := fakeMeasurer()
	we := wrapEntry{BreakCols: []int{10}}
	got := hitSubRow([]byte("01234567890123456789"), &we,
		0, -100, m)
	// Negative X should clamp to start of sub-row.
	if got != 0 {
		t.Fatalf("hitSubRow(-100) = %d, want 0", got)
	}
}

func TestCharInsertPerCursor_ResetsDesiredX(t *testing.T) {
	buf := buffer.New()
	st := editorState{Measurer: fakeMeasurer()}
	st.ensureCursors()
	st.Cursors[0].DesiredX = 42
	charInsertPerCursor(&st, buf, []byte("x"))
	if st.Cursors[0].DesiredX != 0 {
		t.Fatalf("DesiredX=%v want 0 after insert",
			st.Cursors[0].DesiredX)
	}
}

func TestCharInsertPerCursor_MultiCursor_ResetsDesiredX(t *testing.T) {
	buf := bufFromLines("aaa", "bbb")
	st := editorState{Measurer: fakeMeasurer()}
	st.Cursors = []CursorState{
		{Cursor: buffer.Position{Line: 0, ByteCol: 1},
			Anchor: buffer.Position{Line: 0, ByteCol: 1},
			DesiredX: 50},
		{Cursor: buffer.Position{Line: 1, ByteCol: 1},
			Anchor: buffer.Position{Line: 1, ByteCol: 1},
			DesiredX: 60},
	}
	charInsertPerCursor(&st, buf, []byte("z"))
	for i, cs := range st.Cursors {
		if cs.DesiredX != 0 {
			t.Errorf("cursor %d: DesiredX=%v want 0", i, cs.DesiredX)
		}
	}
}

func TestCursorDesiredX_AtColumnZero(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("01234567890123456789")
	cs := &CursorState{Cursor: buffer.Position{Line: 0, ByteCol: 0}}
	got := cursorDesiredX(cs, buf, m, 80)
	if got != 0 {
		t.Fatalf("cursorDesiredX(col 0) = %v, want 0", got)
	}
}

func TestMoveDownVisual_FoldAtEOF(t *testing.T) {
	m := fakeMeasurer()
	buf := bufFromLines("aaa", "bbb", "ccc")
	// Fold covers last two lines.
	folds := []FoldRange{{StartLine: 1, EndLine: 2}}
	cs := &CursorState{
		Cursor:   buffer.Position{Line: 1, ByteCol: 1},
		DesiredX: 8,
	}
	// From fold header (line 1), down should try line 3 which
	// is past EOF → stay on current line, jump to end.
	moveDownVisual(cs, buf, m, 80, folds)
	if cs.Cursor.Line != 1 {
		t.Fatalf("line=%d want 1", cs.Cursor.Line)
	}
	if cs.Cursor.ByteCol != len(buf.Line(1)) {
		t.Fatalf("col=%d want %d",
			cs.Cursor.ByteCol, len(buf.Line(1)))
	}
}

// --- driver integration test ---

func TestDriverWrapUpDown_FullPipeline(t *testing.T) {
	// 25 chars → wraps at narrow width. Line numbers off to
	// eliminate gutter width variability.
	buf := buffer.FromBytes([]byte(
		"abcdefghijklmnopqrstuvwxy\nshort"))
	d := newDriver(EditorCfg{
		IDFocus:         42,
		Buffer:          buf,
		Width:           80,
		Height:          200,
		LineWrap:        true,
		ShowLineNumbers: false,
	})

	d.tick()
	if !d.frame.wrapActive {
		t.Fatal("wrap should be active")
	}

	// Move right to col 4.
	for range 4 {
		d.sendKey(gui.KeyRight)
	}
	startCol := d.cursor().Cursor.ByteCol
	if d.cursor().Cursor.Line != 0 || startCol != 4 {
		t.Fatalf("after right*4: (%d,%d) want (0,4)",
			d.cursor().Cursor.Line, startCol)
	}

	// Down — should stay on line 0 (visual sub-row movement,
	// not logical line jump).
	d.sendKey(gui.KeyDown)
	c := d.cursor()
	if c.Cursor.Line != 0 {
		t.Fatalf("after down: line=%d want 0", c.Cursor.Line)
	}
	if c.Cursor.ByteCol <= startCol {
		t.Fatalf("after down: col=%d should be > %d",
			c.Cursor.ByteCol, startCol)
	}
	downCol := c.Cursor.ByteCol

	// Up — should return to sub-row 0. Column may differ by 1
	// due to HitTest pixel-boundary resolution in the fake
	// measurer (exact char-edge X maps to the preceding char).
	d.sendKey(gui.KeyUp)
	c = d.cursor()
	if c.Cursor.Line != 0 {
		t.Fatalf("after up: line=%d want 0", c.Cursor.Line)
	}
	drift := c.Cursor.ByteCol - startCol
	if drift < -1 || drift > 0 {
		t.Fatalf("after up: col=%d want %d (±1 for boundary)",
			c.Cursor.ByteCol, startCol)
	}

	// Down again — must be stable (same result as first down).
	d.sendKey(gui.KeyDown)
	c = d.cursor()
	if c.Cursor.ByteCol != downCol {
		t.Fatalf("second down: col=%d want %d",
			c.Cursor.ByteCol, downCol)
	}
}

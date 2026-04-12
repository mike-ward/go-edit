package edit

import (
	"math"
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/internal/fakewin"
	"github.com/mike-ward/go-gui/gui"
)

// ---------- Editor factory ----------

func TestEditor_NilBufferSubstitutesEmpty(t *testing.T) {
	v := Editor(EditorCfg{
		IDFocus: 100, Buffer: nil, Width: 400, Height: 200,
	})
	if v == nil {
		t.Fatal("Editor returned nil")
	}
	// Drive a frame to confirm no panic reaches AmendLayout.
	d := &driver{
		cfg: EditorCfg{
			IDFocus: 100, Buffer: buffer.New(),
			Width: 400, Height: 200,
		},
		frame: &editorFrameData{},
		w:     fakewin.New(),
		ly:    &gui.Layout{},
	}
	d.amend = editorAmendLayout(d.cfg, d.frame)
	d.tick()
}

func TestEditor_NaNDimensions(t *testing.T) {
	nan := float32(math.NaN())
	v := Editor(EditorCfg{
		IDFocus: 101,
		Buffer:  mkBuf("hello"),
		Width:   nan,
		Height:  nan,
	})
	if v == nil {
		t.Fatal("Editor returned nil")
	}
}

// ---------- sanitizeDim ----------

func TestSanitizeDim_ClampsEdgeValues(t *testing.T) {
	nan := float32(math.NaN())
	pinf := float32(math.Inf(+1))
	ninf := float32(math.Inf(-1))
	cases := []struct {
		in, want float32
	}{
		{nan, minDimension},
		{pinf, maxDimension},
		{ninf, minDimension},
		{-100, minDimension},
		{0, minDimension},
		{0.5, minDimension},
		{1, 1},
		{100, 100},
		{maxDimension, maxDimension},
		{maxDimension + 1, maxDimension},
		{1e20, maxDimension},
	}
	for _, c := range cases {
		if got := sanitizeDim(c.in); got != c.want {
			t.Errorf("sanitizeDim(%v)=%v want %v", c.in, got, c.want)
		}
	}
}

// ---------- clampScroll edge cases ----------

func TestClampScroll_NaNIn(t *testing.T) {
	cfg := EditorCfg{Buffer: mkBuf("a\nb\nc"), Height: 10}
	st := editorState{ScrollY: float32(math.NaN())}
	clampScroll(&st, cfg, &editorFrameData{}, 10)
	if st.ScrollY != 0 || st.ScrollY != st.ScrollY {
		t.Errorf("ScrollY=%v want 0", st.ScrollY)
	}
}

func TestClampScroll_ZeroLineHeight(t *testing.T) {
	cfg := EditorCfg{Buffer: mkBuf("a\nb"), Height: 10}
	st := editorState{ScrollY: 500}
	clampScroll(&st, cfg, &editorFrameData{}, 0)
	if st.ScrollY != 0 {
		t.Errorf("ScrollY=%v want 0", st.ScrollY)
	}
}

// ---------- ensureCursorVisible edge cases ----------

func TestEnsureCursorVisible_NaNViewport(t *testing.T) {
	st := editorState{ScrollY: 42}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: float32(math.NaN())})
	if st.ScrollY != 42 {
		t.Errorf("ScrollY=%v want 42 (unchanged)", st.ScrollY)
	}
}

func TestEnsureCursorVisible_ZeroViewport(t *testing.T) {
	st := editorState{ScrollY: 42}
	fr := &editorFrameData{lineHeight: 10, valid: true}
	ensureCursorVisible(&st, fr, EditorCfg{Buffer: buffer.New(), Height: 0})
	if st.ScrollY != 42 {
		t.Errorf("ScrollY=%v want 42 (unchanged)", st.ScrollY)
	}
}

// ---------- driver: mouse scroll NaN/absurd ----------

func TestDriver_MouseScrollNaNDropped(t *testing.T) {
	buf := mkBuf("a\nb\nc\nd")
	d := newDriver(EditorCfg{
		IDFocus: 200, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	before := d.state().ScrollY
	d.wheel(d.ly, fakewin.NewScrollEvent(float32(math.NaN())), d.w)
	if d.state().ScrollY != before {
		t.Errorf("ScrollY changed on NaN event")
	}
}

func TestDriver_MouseScrollAbsurdDropped(t *testing.T) {
	buf := mkBuf("a\nb\nc\nd")
	d := newDriver(EditorCfg{
		IDFocus: 201, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	before := d.state().ScrollY
	d.wheel(d.ly, fakewin.NewScrollEvent(1e9), d.w)
	if d.state().ScrollY != before {
		t.Errorf("ScrollY changed on absurd event: %v→%v",
			before, d.state().ScrollY)
	}
}

// ---------- Phase 2 hardening ----------

func TestHitTestPosition_NaNCoords(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 202, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	nan := float32(math.NaN())
	e := &gui.Event{MouseX: nan, MouseY: nan}
	pos := hitTestPosition(e, d.frame, buf, -1)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("NaN coords → %+v, want {0 0}", pos)
	}
}

func TestHitTestPosition_NegativeCoords(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 203, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	e := &gui.Event{MouseX: -100, MouseY: -100}
	pos := hitTestPosition(e, d.frame, buf, -1)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("negative coords → %+v, want {0 0}", pos)
	}
}

func TestHitTestPosition_NilMeasurer(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	frame := &editorFrameData{
		lineHeight: 16,
		state:      editorState{Measurer: nil},
	}
	e := &gui.Event{MouseX: 10, MouseY: 10}
	pos := hitTestPosition(e, frame, buf, -1)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("nil measurer → %+v, want {0 0}", pos)
	}
}

func TestIndentUnit_HugeWidth(t *testing.T) {
	u := indentUnit(buffer.IndentStyle{UseTabs: false, Width: 1 << 20})
	if len(u) > maxIndentWidth {
		t.Errorf("len=%d exceeds cap %d", len(u), maxIndentWidth)
	}
}

func TestWordBoundsAtByte_NegativeCol(t *testing.T) {
	s, e := wordBoundsAtByte([]byte("hello"), -5)
	if s < 0 || e < 0 || e > 5 {
		t.Errorf("[%d,%d) out of range", s, e)
	}
}

func TestDrawSelectionBg_NilMeasurer(t *testing.T) {
	// Must not panic.
	sel := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 0},
		End:   buffer.Position{Line: 0, ByteCol: 3},
	}
	drawSelectionBg(nil, sel, 0, []byte("hello"),
		0, 0, 16, nil, gui.Color{})
}

func TestClickBeyondBuffer(t *testing.T) {
	buf := buffer.FromBytes([]byte("ab"))
	d := newDriver(EditorCfg{
		IDFocus: 204, Buffer: buf, Width: 400, Height: 200,
	})
	// Click way below the buffer (Y beyond last line).
	d.sendClick(0, 5000, 0)
	s := d.cursor()
	if s.Cursor.Line != 0 {
		t.Errorf("cursor line=%d want 0 (single-line buffer)", s.Cursor.Line)
	}
}

func TestDedentLine_HugeWidth(t *testing.T) {
	buf := buffer.FromBytes([]byte("    hello"))
	buf.Props.IndentStyle.Width = 1 << 20
	removed := dedentLine(buf, 0)
	if removed > maxIndentWidth {
		t.Errorf("removed=%d exceeds cap %d", removed, maxIndentWidth)
	}
}

// ---------- Multi-cursor hardening ----------

func TestMultiCursor_ClampAfterExternalTruncate(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	d := newDriver(EditorCfg{
		IDFocus: 210, Buffer: buf, Width: 400, Height: 200,
	})
	d.addCursorAt(1, 2)
	d.addCursorAt(2, 2)
	// Externally truncate to single line.
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{},
			End:   buffer.Position{Line: 2, ByteCol: 3},
		},
		NewBytes: []byte("x"),
	})
	// Tick should clamp without panic.
	d.tick()
	st := d.state()
	for i, c := range st.Cursors {
		if c.Cursor.Line != 0 {
			t.Errorf("cursor %d: line=%d want 0", i, c.Cursor.Line)
		}
	}
}

func TestMultiCursor_RestoreFromUndoCapExtra(t *testing.T) {
	// Corrupt undo record with more cursors than maxCursors.
	st := editorState{Cursors: []CursorState{{
		Cursor: buffer.Position{},
	}}}
	huge := make([]buffer.CursorPair, maxCursors+500)
	for i := range huge {
		huge[i] = buffer.CursorPair{
			Cursor: buffer.Position{Line: i},
		}
	}
	ucs := buffer.UndoCursorState{Extra: huge}
	restoreCursorsFromUndo(&st, ucs)
	if len(st.Cursors) > maxCursors {
		t.Errorf("cursors=%d exceeds cap %d", len(st.Cursors), maxCursors)
	}
}

func TestFindNext_NegativeFrom(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef"))
	r, ok := findNext(buf, []byte("abc"),
		buffer.Position{Line: -5, ByteCol: -3})
	if !ok {
		t.Fatal("should find 'abc'")
	}
	if r.Start.Line != 0 || r.Start.ByteCol != 0 {
		t.Errorf("range=%+v", r)
	}
}

func TestFindNext_FromBeyondBuffer(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef"))
	// from beyond buffer should wrap to start.
	r, ok := findNext(buf, []byte("abc"),
		buffer.Position{Line: 999, ByteCol: 0})
	if !ok {
		t.Fatal("should find 'abc' after wrap")
	}
	if r.Start.Line != 0 {
		t.Errorf("start=%+v", r.Start)
	}
}

func TestShiftPosition_NegativeResult(t *testing.T) {
	// Pathological: delEnd.ByteCol > p.ByteCol on same line.
	p := buffer.Position{Line: 0, ByteCol: 2}
	adjustPos(&p,
		buffer.Position{Line: 0, ByteCol: 0},  // delStart
		buffer.Position{Line: 0, ByteCol: 10}, // delEnd (past p)
		buffer.Position{Line: 0, ByteCol: 0},  // endPos
	)
	// p is inside deleted range → collapses to endPos.
	if p.ByteCol < 0 {
		t.Errorf("ByteCol=%d negative", p.ByteCol)
	}
}

func TestDispatchPerCursor_EmptyCursors(t *testing.T) {
	// Should not panic on empty cursor slice.
	st := editorState{Cursors: nil}
	st.ensureCursors()
	st.Cursors = st.Cursors[:0] // force empty
	buf := buffer.New()
	dispatchPerCursor(EditorCfg{Buffer: buf}, &st, buf, nil,
		Action{ID: "noop", Execute: func(_ EditorCfg, _ *editorState, _ *buffer.Buffer, _ *gui.Window) {}},
		false)
	// No panic = pass.
}

func TestCharInsertPerCursor_EmptyCursors(t *testing.T) {
	st := editorState{Cursors: nil}
	buf := buffer.New()
	charInsertPerCursor(&st, buf, []byte("x"))
	// No panic = pass.
}

func TestMultiCursor_MaxCursorsCap(t *testing.T) {
	st := editorState{Cursors: []CursorState{
		{Cursor: buffer.Position{}},
	}}
	for i := 1; i < maxCursors; i++ {
		st.Cursors = append(st.Cursors, CursorState{
			Cursor: buffer.Position{Line: i},
			Anchor: buffer.Position{Line: i},
		})
	}
	addCursor(&st, CursorState{
		Cursor: buffer.Position{Line: maxCursors + 1},
		Anchor: buffer.Position{Line: maxCursors + 1},
	})
	if len(st.Cursors) != maxCursors {
		t.Errorf("len=%d want %d", len(st.Cursors), maxCursors)
	}
}

// ---------- CursorPos public API ----------

func TestCursorPos_NilWindow(t *testing.T) {
	line, col, ok := CursorPos(nil, 1)
	if ok || line != 0 || col != 0 {
		t.Errorf("nil window → (%d,%d,%v), want (0,0,false)",
			line, col, ok)
	}
}

func TestCursorPos_NoState(t *testing.T) {
	w := fakewin.New()
	line, col, ok := CursorPos(w, 999)
	if ok {
		t.Errorf("no state → ok=true, want false")
	}
	if line != 0 || col != 0 {
		t.Errorf("no state → (%d,%d), want (0,0)", line, col)
	}
}

func TestCursorPos_EmptyCursors(t *testing.T) {
	w := fakewin.New()
	// Seed state with empty Cursors slice.
	st := editorState{Cursors: []CursorState{}}
	storeState(w, 50, st)
	_, _, ok := CursorPos(w, 50)
	if ok {
		t.Error("empty cursors → ok=true, want false")
	}
}

func TestCursorPos_ReturnsPosition(t *testing.T) {
	buf := mkBuf("aaa\nbbb\nccc")
	d := newDriver(EditorCfg{
		IDFocus: 51, Buffer: buf, Width: 400, Height: 200,
	})
	// Move cursor into line 1 by clicking mid-line.
	// Exact column depends on go-glyph HitTest boundary
	// resolution in the fake measurer; verify line and
	// that cursor landed in the interior (not at 0 or EOL).
	d.sendClick(
		d.frame.gutterW+d.frame.padLeft+20,
		16, // line 1 × 16px
		0,
	)
	line, col, ok := CursorPos(d.w, 51)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if line != 1 {
		t.Errorf("line=%d, want 1", line)
	}
	if col < 1 || col > 2 {
		t.Errorf("col=%d, want 1 or 2", col)
	}
}

// ---------- hitTestPosition additional coverage ----------

func TestHitTestPosition_EmptyBuffer(t *testing.T) {
	buf := buffer.New() // 0 lines
	d := newDriver(EditorCfg{
		IDFocus: 52, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	e := &gui.Event{MouseX: 50, MouseY: 50}
	pos := hitTestPosition(e, d.frame, buf, -1)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("empty buffer → %+v, want {0 0}", pos)
	}
}

func TestHitTestPosition_ScrollYOverride(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc\nddd\neee"))
	d := newDriver(EditorCfg{
		IDFocus: 53, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	// Click at y=0 with scrollY=0 → line 0.
	e := &gui.Event{MouseX: 0, MouseY: 0}
	pos0 := hitTestPosition(e, d.frame, buf, 0)
	// Click at y=0 with scrollY=32 (2 lines × 16px) → line 2.
	pos32 := hitTestPosition(e, d.frame, buf, 32)
	if pos0.Line != 0 {
		t.Errorf("scrollY=0 → line %d, want 0", pos0.Line)
	}
	if pos32.Line != 2 {
		t.Errorf("scrollY=32 → line %d, want 2", pos32.Line)
	}
}

func TestHitTestPosition_NegativeMouseY(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	d := newDriver(EditorCfg{
		IDFocus: 54, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	e := &gui.Event{MouseX: 0, MouseY: -100}
	pos := hitTestPosition(e, d.frame, buf, -1)
	if pos.Line != 0 {
		t.Errorf("negative mouseY → line %d, want 0", pos.Line)
	}
}

func TestHitTestPosition_NaNScrollYParam(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 55, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	nan := float32(math.NaN())
	e := &gui.Event{MouseX: 0, MouseY: 0}
	pos := hitTestPosition(e, d.frame, buf, nan)
	// NaN scrollY should fall back to frame snapshot (0).
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("NaN scrollY → %+v, want {0 0}", pos)
	}
}

// ---------- hitTestLocal ----------

func TestHitTestLocal_NilBuffer(t *testing.T) {
	frame := &editorFrameData{lineHeight: 16}
	var scratch gui.Event
	pos := hitTestLocal(10, 10, -1, frame, nil, &scratch)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("nil buf → %+v, want {0 0}", pos)
	}
}

func TestHitTestLocal_NilScratch(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	frame := &editorFrameData{lineHeight: 16}
	pos := hitTestLocal(10, 10, -1, frame, buf, nil)
	if pos.Line != 0 || pos.ByteCol != 0 {
		t.Errorf("nil scratch → %+v, want {0 0}", pos)
	}
}

func TestHitTestLocal_DelegatesToHitTest(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	d := newDriver(EditorCfg{
		IDFocus: 56, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	var scratch gui.Event
	// y=32 at 16px line height → line 2.
	pos := hitTestLocal(0, 32, -1, d.frame, buf, &scratch)
	if pos.Line != 2 {
		t.Errorf("line=%d, want 2", pos.Line)
	}
}

// ---------- canvasOrigin NaN guard ----------

func TestOnClick_CanvasOriginNaNGuard(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 57, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()

	// Set valid origins first.
	d.frame.canvasOriginX = 10
	d.frame.canvasOriginY = 20

	// Simulate click with NaN shape coordinates.
	nan := float32(math.NaN())
	ly := &gui.Layout{Shape: &gui.Shape{X: nan, Y: nan}}
	e := fakewin.NewClickEvent(0, 0, 0)
	d.click(ly, e, d.w)

	// Origins should retain prior valid values.
	if d.frame.canvasOriginX != 10 {
		t.Errorf("canvasOriginX=%v, want 10",
			d.frame.canvasOriginX)
	}
	if d.frame.canvasOriginY != 20 {
		t.Errorf("canvasOriginY=%v, want 20",
			d.frame.canvasOriginY)
	}
}

// ---------- TriggerAction hardening ----------

func TestTriggerAction_NilWindowNoPanic(t *testing.T) {
	// Must not panic.
	TriggerAction(nil, 1, "edit.undo")
}

func TestTriggerAction_EmptyActionIDNoPanic(t *testing.T) {
	w := fakewin.New()
	TriggerAction(w, 1, "")
	st := loadState(w, 1)
	if st.PendingAction != "" {
		t.Errorf("PendingAction=%q, want empty", st.PendingAction)
	}
}

func TestTriggerAction_UnknownIDStoredButNoOp(t *testing.T) {
	buf := mkBuf("hello")
	d := newDriver(EditorCfg{
		IDFocus: 300, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	TriggerAction(d.w, 300, "no.such.action")
	// tick must not panic; buffer must be unchanged.
	d.tick()
	if buf.LineCount() != 1 || string(buf.Line(0)) != "hello" {
		t.Errorf("buffer changed on unknown action")
	}
}

func TestTriggerAction_ReadOnlyBlocksEditAction(t *testing.T) {
	buf := mkBuf("hello")
	buf.EnableUndo(nil)
	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("x"),
	})
	// buffer is now "xhello"; undo would revert it.
	d := newDriver(EditorCfg{
		IDFocus:  301,
		Buffer:   buf,
		Width:    400,
		Height:   200,
		ReadOnly: true,
	})
	d.tick()
	TriggerAction(d.w, 301, "edit.undo")
	d.tick()
	// ReadOnly: undo must not have fired.
	if string(buf.Line(0)) != "xhello" {
		t.Errorf("undo ran in read-only mode: got %q", string(buf.Line(0)))
	}
}

// TestEditor_DoubleMountPanics verifies the W2 guard fires when
// the closure-shared frame struct receives two AmendLayout calls
// with distinct *gui.Layout pointers inside the same frame.
func TestEditor_DoubleMountPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on double-mount")
		}
	}()
	cfg := EditorCfg{
		IDFocus: 900, Buffer: buffer.New(),
		Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	// Two distinct Layout allocations simulate two mount sites
	// in the same render tree at the same frame.
	ly1 := &gui.Layout{}
	ly2 := &gui.Layout{}
	amend(ly1, w)
	amend(ly2, w) // must panic
}

// TestEditor_ReuseLayoutDoesNotPanic verifies the guard tolerates
// the test driver pattern of calling AmendLayout many times with
// the same *gui.Layout (simulating successive frames where the
// framework's frame counter is not advanced).
func TestEditor_ReuseLayoutDoesNotPanic(t *testing.T) {
	cfg := EditorCfg{
		IDFocus: 901, Buffer: buffer.New(),
		Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{}
	for range 5 {
		amend(ly, w)
	}
}

// TestEditor_DrawVersion_StableOnUnchangedFrame confirms the
// per-frame draw version fold is deterministic: two consecutive
// AmendLayout calls with no intervening state change produce the
// same drawVersion (go-gui's DrawCanvas cache will then hit).
func TestEditor_DrawVersion_StableOnUnchangedFrame(t *testing.T) {
	buf := buffer.New()
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 0},
		},
		NewBytes: []byte("hello"),
	})
	cfg := EditorCfg{
		IDFocus: 910, Buffer: buf, Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{Children: []gui.Layout{{Shape: &gui.Shape{}}}}

	amend(ly, w)
	v1 := frame.drawVersion
	if v1 == 0 {
		t.Fatal("drawVersion should never be 0 (collision with initial shape version)")
	}
	if got := ly.Children[0].Shape.Version; got != v1 {
		t.Fatalf("shape.Version = %d, want %d", got, v1)
	}

	amend(ly, w)
	v2 := frame.drawVersion
	if v2 != v1 {
		t.Fatalf("drawVersion drifted with no state change: v1=%d v2=%d", v1, v2)
	}
}

// TestEditor_DrawVersion_ChangesOnEdit verifies the fold picks
// up Buffer.Version changes — any edit forces a re-render.
func TestEditor_DrawVersion_ChangesOnEdit(t *testing.T) {
	buf := buffer.New()
	cfg := EditorCfg{
		IDFocus: 911, Buffer: buf, Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{Children: []gui.Layout{{Shape: &gui.Shape{}}}}

	amend(ly, w)
	v1 := frame.drawVersion

	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 0},
		},
		NewBytes: []byte("x"),
	})
	amend(ly, w)
	v2 := frame.drawVersion
	if v1 == v2 {
		t.Fatalf("drawVersion did not change after buffer edit: %d", v1)
	}
}

// TestFloatBitsStable_PosNegZero verifies +0 and -0 fold to
// the same value so cache keys don't drift across sign flips.
func TestFloatBitsStable_PosNegZero(t *testing.T) {
	pos := floatBitsStable(0.0)
	neg := floatBitsStable(float32(math.Copysign(0, -1)))
	if pos != neg {
		t.Errorf("+0/-0 fold differently: %d vs %d", pos, neg)
	}
}

// TestFloatBitsStable_NaN verifies every NaN bit pattern folds
// to the canonical quiet NaN constant.
func TestFloatBitsStable_NaN(t *testing.T) {
	quiet := floatBitsStable(float32(math.NaN()))
	// A different NaN bit pattern constructed via Float64bits.
	altBits := math.Float64bits(math.NaN()) | 0x1
	alt := floatBitsStable(float32(math.Float64frombits(altBits)))
	if quiet != alt {
		t.Errorf("NaN patterns differ: %d vs %d", quiet, alt)
	}
	if quiet == 0 {
		t.Error("NaN should not fold to zero")
	}
}

// TestFloatBitsStable_DistinctFinite verifies two distinct finite
// floats produce distinct bits.
func TestFloatBitsStable_DistinctFinite(t *testing.T) {
	a := floatBitsStable(1.5)
	b := floatBitsStable(2.5)
	if a == b {
		t.Errorf("1.5 and 2.5 fold identically: %d", a)
	}
}

// TestEditor_DrawVersion_StableUnderNaNScroll confirms the
// cache key fold normalizes NaN float bits so an upstream
// sanitize failure cannot cause cache thrashing.
func TestEditor_DrawVersion_StableUnderNaNScroll(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	cfg := EditorCfg{
		IDFocus: 920, Buffer: buf, Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{Children: []gui.Layout{{Shape: &gui.Shape{}}}}

	// Inject a NaN directly via state — bypassing the normal
	// clampScroll path — to verify the fold is stable.
	st := loadState(w, cfg.IDFocus)
	nan := float32(math.NaN())
	st.ScrollY = nan
	storeState(w, cfg.IDFocus, st)
	amend(ly, w)
	v1 := frame.drawVersion

	// Re-inject a *different* NaN (multiple bit patterns) and
	// confirm the hash is identical.
	st = loadState(w, cfg.IDFocus)
	st.ScrollY = float32(math.Float64frombits(
		math.Float64bits(math.NaN()) | 1))
	storeState(w, cfg.IDFocus, st)
	amend(ly, w)
	v2 := frame.drawVersion
	if v1 != v2 {
		t.Errorf("drawVersion drifted across NaN bit patterns: %d vs %d",
			v1, v2)
	}
}

// TestEditor_DrawVersion_ChangesOnFindBarState confirms every
// find-bar visual knob (caret within same-length query, focus,
// replace toggle, current match index) invalidates the draw
// cache. Without this, the DrawCanvas cache would serve stale
// find-bar pixels on those interactions.
func TestEditor_DrawVersion_ChangesOnFindBarState(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	cfg := EditorCfg{
		IDFocus: 915, Buffer: buf, Width: 400, Height: 200,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{Children: []gui.Layout{{Shape: &gui.Shape{}}}}

	// Prime with search active + a query.
	{
		st := loadState(w, cfg.IDFocus)
		st.Search.Active = true
		st.Search.Query = "hello"
		st.Search.FieldCursor = 0
		storeState(w, cfg.IDFocus, st)
	}
	amend(ly, w)
	base := frame.drawVersion

	mutate := func(name string, fn func(*editorState)) {
		t.Helper()
		st := loadState(w, cfg.IDFocus)
		fn(&st)
		storeState(w, cfg.IDFocus, st)
		amend(ly, w)
		if frame.drawVersion == base {
			t.Fatalf("%s: drawVersion did not change", name)
		}
		base = frame.drawVersion
	}

	mutate("FieldCursor moves within same-length query",
		func(st *editorState) { st.Search.FieldCursor = 3 })
	mutate("ShowReplace toggles",
		func(st *editorState) { st.Search.ShowReplace = true })
	mutate("FocusReplace toggles",
		func(st *editorState) { st.Search.FocusReplace = true })
	mutate("ReplaceText grows",
		func(st *editorState) { st.Search.ReplaceText = "x" })
	mutate("CaseSensitive toggles",
		func(st *editorState) { st.Search.CaseSensitive = true })
	mutate("IsRegex toggles",
		func(st *editorState) { st.Search.IsRegex = true })
	mutate("InSelection toggles",
		func(st *editorState) { st.Search.InSelection = true })
	mutate("CurrentMatch changes",
		func(st *editorState) { st.Search.CurrentMatch = 1 })
}

// TestEditor_DrawVersion_ChangesOnScroll verifies scroll updates
// invalidate the cache. Scroll is a pure visual change, not a
// buffer change, so it must flow into the fold independently.
func TestEditor_DrawVersion_ChangesOnScroll(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj"))
	cfg := EditorCfg{
		IDFocus: 912, Buffer: buf, Width: 400, Height: 32,
	}
	frame := &editorFrameData{}
	amend := editorAmendLayout(cfg, frame)
	w := fakewin.New()
	ly := &gui.Layout{Children: []gui.Layout{{Shape: &gui.Shape{}}}}

	amend(ly, w)
	v1 := frame.drawVersion

	// Mutate persisted scroll directly via StateMap.
	st := loadState(w, cfg.IDFocus)
	st.ScrollY = 50
	storeState(w, cfg.IDFocus, st)

	amend(ly, w)
	v2 := frame.drawVersion
	if v1 == v2 {
		t.Fatalf("drawVersion did not change after scroll: %d", v1)
	}
}

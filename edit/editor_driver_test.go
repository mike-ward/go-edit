package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/internal/fakewin"
	"github.com/mike-ward/go-gui/gui"
)

// driver wraps Editor's closures so tests can drive them without
// going through go-gui's layout pipeline.
type driver struct {
	cfg   EditorCfg
	frame *editorFrameData
	amend func(*gui.Layout, *gui.Window)
	key   func(*gui.Layout, *gui.Event, *gui.Window)
	char  func(*gui.Layout, *gui.Event, *gui.Window)
	wheel func(*gui.Layout, *gui.Event, *gui.Window)
	click func(*gui.Layout, *gui.Event, *gui.Window)
	w     *gui.Window
	ly    *gui.Layout
}

func newDriver(cfg EditorCfg) *driver {
	frame := &editorFrameData{}
	return &driver{
		cfg:   cfg,
		frame: frame,
		amend: editorAmendLayout(cfg, frame),
		key:   editorOnKeyDown(cfg, frame),
		char:  editorOnChar(cfg, frame),
		wheel: editorOnMouseScroll(cfg, frame),
		click: editorOnClick(cfg, frame),
		w:     fakewin.New(),
		ly:    &gui.Layout{},
	}
}

// tick runs the amend pass so frame is populated and persistent state
// is synced from the window's StateMap. Call before each event.
func (d *driver) tick() { d.amend(d.ly, d.w) }

func (d *driver) sendKey(code gui.KeyCode) {
	d.tick()
	d.key(d.ly, fakewin.NewKeyEvent(code, 0), d.w)
}

func (d *driver) sendKeyMod(code gui.KeyCode, mods gui.Modifier) {
	d.tick()
	d.key(d.ly, fakewin.NewKeyEvent(code, mods), d.w)
}

func (d *driver) sendClick(x, y float32, mods gui.Modifier) {
	d.tick()
	d.click(d.ly, fakewin.NewClickEvent(x, y, mods), d.w)
}

func (d *driver) sendChar(r rune) {
	d.tick()
	d.char(d.ly, fakewin.NewCharEvent(r), d.w)
}

func (d *driver) sendScroll(dy float32) {
	d.tick()
	d.wheel(d.ly, fakewin.NewScrollEvent(dy), d.w)
}

func (d *driver) state() editorState {
	return loadState(d.w, d.cfg.IDFocus)
}

// cursor returns the primary cursor state for test assertions.
func (d *driver) cursor() CursorState {
	return d.state().Cursors[0]
}

// addCursorAt adds a cursor at the given position for multi-cursor
// testing.
func (d *driver) addCursorAt(line, col int) {
	st := loadState(d.w, d.cfg.IDFocus)
	addCursor(&st, CursorState{
		Cursor:     buffer.Position{Line: line, ByteCol: col},
		Anchor:     buffer.Position{Line: line, ByteCol: col},
		DesiredCol: col,
	})
	storeState(d.w, d.cfg.IDFocus, st)
}

// cursorCount returns the number of active cursors.
func (d *driver) cursorCount() int {
	return len(d.state().Cursors)
}

// ---------- tests ----------

func TestDriver_TypeSequenceUpdatesBuffer(t *testing.T) {
	buf := buffer.New()
	d := newDriver(EditorCfg{
		IDFocus: 1, Buffer: buf, Width: 400, Height: 200,
	})
	for _, r := range "hello" {
		d.sendChar(r)
	}
	if buf.String() != "hello" {
		t.Errorf("buffer=%q", buf.String())
	}
	if d.cursor().Cursor.ByteCol != 5 {
		t.Errorf("col=%d", d.cursor().Cursor.ByteCol)
	}
}

func TestDriver_EnterSplitsLine(t *testing.T) {
	buf := buffer.FromBytes([]byte("foo"))
	d := newDriver(EditorCfg{
		IDFocus: 2, Buffer: buf, Width: 400, Height: 200,
	})
	// Place cursor at end of "foo".
	d.sendKey(gui.KeyEnd)
	d.sendKey(gui.KeyEnter)
	if buf.LineCount() != 2 {
		t.Errorf("LineCount=%d", buf.LineCount())
	}
	if buf.String() != "foo\n" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_BackspaceAtLineStartJoinsLines(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb"))
	d := newDriver(EditorCfg{
		IDFocus: 3, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyDown) // to line 1
	d.sendKey(gui.KeyHome) // col 0
	d.sendKey(gui.KeyBackspace)
	if buf.String() != "ab" {
		t.Errorf("buffer=%q", buf.String())
	}
	if buf.LineCount() != 1 {
		t.Errorf("LineCount=%d", buf.LineCount())
	}
}

func TestDriver_ArrowsNavigate(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef"))
	d := newDriver(EditorCfg{
		IDFocus: 4, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyRight)
	d.sendKey(gui.KeyRight)
	d.sendKey(gui.KeyDown)
	s := d.cursor()
	if s.Cursor.Line != 1 || s.Cursor.ByteCol != 2 {
		t.Errorf("cursor=%+v want {1 2}", s.Cursor)
	}
}

func TestDriver_PgDnScrollsIntoView(t *testing.T) {
	// Build a 100-line buffer (99 "x" lines + trailing newline
	// splits into a 100th empty line), viewport = 5 lines tall.
	var bytes []byte
	for range 99 {
		bytes = append(bytes, 'x', '\n')
	}
	bytes = append(bytes, 'x')
	buf := buffer.FromBytes(bytes)
	d := newDriver(EditorCfg{
		IDFocus: 5, Buffer: buf,
		Width:  400,
		Height: 5 * fakewin.LineHeight, // 5 lines
	})
	d.sendKey(gui.KeyPageDown)
	if d.state().ScrollY <= 0 {
		t.Errorf("ScrollY=%v want >0", d.state().ScrollY)
	}
}

func TestDriver_MouseScrollClamps(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb\nc"))
	d := newDriver(EditorCfg{
		IDFocus: 6, Buffer: buf,
		Width:  400,
		Height: 10 * fakewin.LineHeight, // bigger than buffer
	})
	d.sendScroll(-1000) // scroll way down
	if d.state().ScrollY != 0 {
		t.Errorf("ScrollY=%v want 0 (clamped)", d.state().ScrollY)
	}
}

func TestDriver_ExternalBufferTruncateHealsCursor(t *testing.T) {
	buf := buffer.FromBytes([]byte("a\nb\nc\nd\ne"))
	d := newDriver(EditorCfg{
		IDFocus: 7, Buffer: buf, Width: 400, Height: 200,
	})
	// Move cursor to line 4.
	for range 4 {
		d.sendKey(gui.KeyDown)
	}
	if d.cursor().Cursor.Line != 4 {
		t.Fatalf("setup: cursor=%+v", d.cursor().Cursor)
	}
	// Externally truncate buffer: replace everything with "x".
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 4, ByteCol: 1},
		},
		NewBytes: []byte("x"),
	})
	// Tick amend — should clamp cursor to line 0 without panic.
	d.tick()
	s := d.cursor()
	if s.Cursor.Line != 0 || s.Cursor.ByteCol > 1 {
		t.Errorf("cursor=%+v want {0,<=1}", s.Cursor)
	}
}

func TestDriver_ReadOnlyBlocksEdits(t *testing.T) {
	buf := buffer.FromBytes([]byte("locked"))
	d := newDriver(EditorCfg{
		IDFocus: 8, Buffer: buf, Width: 400, Height: 200,
		ReadOnly: true,
	})
	for _, r := range "XYZ" {
		d.sendChar(r)
	}
	d.sendKey(gui.KeyBackspace)
	d.sendKey(gui.KeyDelete)
	d.sendKey(gui.KeyEnter)
	if buf.String() != "locked" {
		t.Errorf("buffer=%q want unchanged", buf.String())
	}
}

// ---------- Phase 2: selection ----------

func TestDriver_ShiftRightExtendsSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	d := newDriver(EditorCfg{
		IDFocus: 10, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	s := d.cursor()
	if s.Anchor != (buffer.Position{}) {
		t.Errorf("anchor=%+v want {0 0}", s.Anchor)
	}
	if s.Cursor != (buffer.Position{ByteCol: 2}) {
		t.Errorf("cursor=%+v want {0 2}", s.Cursor)
	}
}

func TestDriver_RightCollapsesSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcd"))
	d := newDriver(EditorCfg{
		IDFocus: 11, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "ab".
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	// Right arrow collapses to end of selection.
	d.sendKey(gui.KeyRight)
	s := d.cursor()
	if s.Cursor != (buffer.Position{ByteCol: 2}) {
		t.Errorf("cursor=%+v want {0 2}", s.Cursor)
	}
	if s.Anchor != s.Cursor {
		t.Errorf("selection not collapsed: anchor=%+v cursor=%+v",
			s.Anchor, s.Cursor)
	}
}

func TestDriver_LeftCollapsesSelectionToStart(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcd"))
	d := newDriver(EditorCfg{
		IDFocus: 12, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKey(gui.KeyLeft)
	s := d.cursor()
	if s.Cursor != (buffer.Position{}) {
		t.Errorf("cursor=%+v want {0 0}", s.Cursor)
	}
}

func TestDriver_ShiftDownMultiLineSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef\nghi"))
	d := newDriver(EditorCfg{
		IDFocus: 13, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyDown, gui.ModShift)
	s := d.cursor()
	if s.Anchor != (buffer.Position{}) {
		t.Errorf("anchor=%+v", s.Anchor)
	}
	if s.Cursor.Line != 1 {
		t.Errorf("cursor=%+v", s.Cursor)
	}
}

func TestDriver_TypeReplacesSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 14, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "hel".
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendChar('X')
	if buf.String() != "Xlo" {
		t.Errorf("buffer=%q want Xlo", buf.String())
	}
}

func TestDriver_BackspaceDeletesSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 15, Buffer: buf, Width: 400, Height: 200,
	})
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKey(gui.KeyBackspace)
	if buf.String() != "lo" {
		t.Errorf("buffer=%q want lo", buf.String())
	}
}

func TestDriver_SelectAll(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc\ndef"))
	d := newDriver(EditorCfg{
		IDFocus: 16, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyA, gui.ModCtrl)
	s := d.cursor()
	if s.Anchor != (buffer.Position{}) {
		t.Errorf("anchor=%+v", s.Anchor)
	}
	if s.Cursor != (buffer.Position{Line: 1, ByteCol: 3}) {
		t.Errorf("cursor=%+v", s.Cursor)
	}
}

// ---------- Phase 2: clipboard ----------

func TestDriver_CopyPasteRoundTrip(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	d := newDriver(EditorCfg{
		IDFocus: 17, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "hello".
	for range 5 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKeyMod(gui.KeyC, gui.ModCtrl) // copy
	d.sendKey(gui.KeyEnd)               // move to end
	d.sendKeyMod(gui.KeyV, gui.ModCtrl) // paste
	if buf.String() != "hello worldhello" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_CutRemovesText(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcdef"))
	d := newDriver(EditorCfg{
		IDFocus: 18, Buffer: buf, Width: 400, Height: 200,
	})
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKeyMod(gui.KeyX, gui.ModCtrl)
	if buf.String() != "def" {
		t.Errorf("buffer=%q want def", buf.String())
	}
	// Paste back.
	d.sendKeyMod(gui.KeyV, gui.ModCtrl)
	if buf.String() != "abcdef" {
		t.Errorf("buffer=%q want abcdef", buf.String())
	}
}

func TestDriver_CutNoSelectionIsNoop(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	d := newDriver(EditorCfg{
		IDFocus: 19, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyX, gui.ModCtrl)
	if buf.String() != "abc" {
		t.Errorf("buffer=%q want abc", buf.String())
	}
}

func TestDriver_PasteEmptyClipboard(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	d := newDriver(EditorCfg{
		IDFocus: 20, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyV, gui.ModCtrl) // empty clipboard
	if buf.String() != "abc" {
		t.Errorf("buffer=%q want abc", buf.String())
	}
}

// ---------- Phase 2: indent ----------

func TestDriver_TabInsertsIndent(t *testing.T) {
	buf := buffer.New()
	buf.Props.IndentStyle.UseTabs = true
	buf.Props.IndentStyle.Width = 4
	d := newDriver(EditorCfg{
		IDFocus: 21, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyTab)
	if buf.String() != "\t" {
		t.Errorf("buffer=%q want tab", buf.String())
	}
}

func TestDriver_TabIndentsSelectedLines(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	buf.Props.IndentStyle.UseTabs = true
	buf.Props.IndentStyle.Width = 4
	d := newDriver(EditorCfg{
		IDFocus: 22, Buffer: buf, Width: 400, Height: 200,
	})
	// Select all 3 lines.
	d.sendKeyMod(gui.KeyA, gui.ModCtrl)
	d.sendKey(gui.KeyTab)
	if buf.String() != "\taaa\n\tbbb\n\tccc" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_ShiftTabDedents(t *testing.T) {
	buf := buffer.FromBytes([]byte("\thello"))
	d := newDriver(EditorCfg{
		IDFocus: 23, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyTab, gui.ModShift)
	if buf.String() != "hello" {
		t.Errorf("buffer=%q want hello", buf.String())
	}
}

func TestDriver_DedentNoIndent(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 24, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyTab, gui.ModShift)
	if buf.String() != "hello" {
		t.Errorf("buffer=%q want hello (unchanged)", buf.String())
	}
}

// ---------- Phase 2: auto-indent ----------

func TestDriver_EnterAutoIndent(t *testing.T) {
	buf := buffer.FromBytes([]byte("\thello"))
	d := newDriver(EditorCfg{
		IDFocus: 25, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyEnd)
	d.sendKey(gui.KeyEnter)
	if buf.String() != "\thello\n\t" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_EnterAfterBraceAddsIndent(t *testing.T) {
	buf := buffer.FromBytes([]byte("func() {"))
	buf.Props.IndentStyle.UseTabs = true
	buf.Props.IndentStyle.Width = 4
	d := newDriver(EditorCfg{
		IDFocus: 26, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyEnd)
	d.sendKey(gui.KeyEnter)
	if buf.String() != "func() {\n\t" {
		t.Errorf("buffer=%q", buf.String())
	}
}

// ---------- Phase 2: mouse ----------

func TestDriver_ClickSetsCursor(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcdef"))
	d := newDriver(EditorCfg{
		IDFocus: 27, Buffer: buf, Width: 400, Height: 200,
	})
	// Click inside char at index 3 (padLeft=4, so mx=25).
	// HitTest(25, ...) lands inside char 3 (X=24..32).
	d.sendClick(29, 0, 0)
	s := d.cursor()
	if s.Cursor.ByteCol != 3 {
		t.Errorf("cursor=%+v want col 3", s.Cursor)
	}
	if s.Anchor != s.Cursor {
		t.Errorf("selection not cleared")
	}
}

func TestDriver_ClickBeyondLineClamps(t *testing.T) {
	buf := buffer.FromBytes([]byte("ab"))
	d := newDriver(EditorCfg{
		IDFocus: 28, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendClick(400, 0, 0) // way past end of "ab"
	s := d.cursor()
	if s.Cursor.ByteCol != 2 {
		t.Errorf("cursor=%+v want col 2", s.Cursor)
	}
}

// ---------- Phase 3: undo / redo ----------

func TestDriver_UndoRedoTyping(t *testing.T) {
	buf := buffer.New()
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 30, Buffer: buf, Width: 400, Height: 200,
	})
	for _, r := range "hello" {
		d.sendChar(r)
	}
	if buf.String() != "hello" {
		t.Fatalf("after typing: %q", buf.String())
	}
	// Undo coalesced typing.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "" {
		t.Fatalf("after undo: %q", buf.String())
	}
	if d.cursor().Cursor != (buffer.Position{}) {
		t.Errorf("cursor after undo: %+v", d.cursor().Cursor)
	}
	// Redo.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl|gui.ModShift)
	if buf.String() != "hello" {
		t.Fatalf("after redo: %q", buf.String())
	}
}

func TestDriver_UndoNewlineGroup(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 31, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKey(gui.KeyEnd)
	d.sendKey(gui.KeyEnter)
	if buf.LineCount() != 2 {
		t.Fatalf("lines=%d", buf.LineCount())
	}
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "abc" {
		t.Fatalf("after undo: %q", buf.String())
	}
}

func TestDriver_UndoPaste(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 32, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "hel", copy, move to end, paste.
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKeyMod(gui.KeyC, gui.ModCtrl)
	d.sendKey(gui.KeyEnd)
	d.sendKeyMod(gui.KeyV, gui.ModCtrl)
	if buf.String() != "hellohel" {
		t.Fatalf("after paste: %q", buf.String())
	}
	// Undo should revert the paste as one step.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "hello" {
		t.Fatalf("after undo paste: %q", buf.String())
	}
}

func TestDriver_UndoIndentGroup(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	buf.Props.IndentStyle.UseTabs = true
	buf.Props.IndentStyle.Width = 4
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 33, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyA, gui.ModCtrl) // select all
	d.sendKey(gui.KeyTab)               // indent
	if buf.String() != "\taaa\n\tbbb\n\tccc" {
		t.Fatalf("after indent: %q", buf.String())
	}
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl) // undo all at once
	if buf.String() != "aaa\nbbb\nccc" {
		t.Fatalf("after undo indent: %q", buf.String())
	}
}

func TestDriver_UndoRedoReadOnlyBlocked(t *testing.T) {
	buf := buffer.FromBytes([]byte("locked"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 34, Buffer: buf, Width: 400, Height: 200,
		ReadOnly: true,
	})
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "locked" {
		t.Errorf("undo should be blocked in read-only")
	}
}

func TestDriver_UndoCutGroup(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcdef"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 35, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "abc", cut.
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKeyMod(gui.KeyX, gui.ModCtrl)
	if buf.String() != "def" {
		t.Fatalf("after cut: %q", buf.String())
	}
	// Single undo restores.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "abcdef" {
		t.Fatalf("after undo cut: %q", buf.String())
	}
}

func TestDriver_UndoDeleteSelectionGroup(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 36, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "hello", press Delete.
	for range 5 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendKey(gui.KeyDelete)
	if buf.String() != " world" {
		t.Fatalf("after delete: %q", buf.String())
	}
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "hello world" {
		t.Fatalf("after undo: %q", buf.String())
	}
}

func TestDriver_UndoDedentGroup(t *testing.T) {
	buf := buffer.FromBytes([]byte("\taaa\n\tbbb\n\tccc"))
	buf.Props.IndentStyle.UseTabs = true
	buf.Props.IndentStyle.Width = 4
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 37, Buffer: buf, Width: 400, Height: 200,
	})
	d.sendKeyMod(gui.KeyA, gui.ModCtrl)    // select all
	d.sendKeyMod(gui.KeyTab, gui.ModShift) // dedent
	if buf.String() != "aaa\nbbb\nccc" {
		t.Fatalf("after dedent: %q", buf.String())
	}
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "\taaa\n\tbbb\n\tccc" {
		t.Fatalf("after undo dedent: %q", buf.String())
	}
}

func TestDriver_UndoTypeOverSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 38, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "hel", type 'X' to replace.
	for range 3 {
		d.sendKeyMod(gui.KeyRight, gui.ModShift)
	}
	d.sendChar('X')
	if buf.String() != "Xlo" {
		t.Fatalf("after type over: %q", buf.String())
	}
	// Single undo should restore "hello" (grouped).
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "hello" {
		t.Fatalf("after undo type-over: %q", buf.String())
	}
}

// ---------- Phase 5: multi-cursor ----------

func TestDriver_MultiCursorTypeChar(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 50, Buffer: buf, Width: 400, Height: 200,
	})
	// Place primary at (0,0), add cursors at (1,0) and (2,0).
	d.addCursorAt(1, 0)
	d.addCursorAt(2, 0)
	if d.cursorCount() != 3 {
		t.Fatalf("cursors=%d want 3", d.cursorCount())
	}
	d.sendChar('X')
	if buf.String() != "Xaaa\nXbbb\nXccc" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_MultiCursorBackspace(t *testing.T) {
	buf := buffer.FromBytes([]byte("Xaaa\nXbbb\nXccc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 51, Buffer: buf, Width: 400, Height: 200,
	})
	// Place cursors at col 1 on each line (after the X).
	d.sendKey(gui.KeyRight) // primary to (0,1)
	d.addCursorAt(1, 1)
	d.addCursorAt(2, 1)
	d.sendKey(gui.KeyBackspace)
	if buf.String() != "aaa\nbbb\nccc" {
		t.Errorf("buffer=%q", buf.String())
	}
}

func TestDriver_MultiCursorUndo(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 52, Buffer: buf, Width: 400, Height: 200,
	})
	d.addCursorAt(1, 0)
	d.addCursorAt(2, 0)
	d.sendChar('X')
	if buf.String() != "Xaaa\nXbbb\nXccc" {
		t.Fatalf("after type: %q", buf.String())
	}
	// Undo should revert all three inserts.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "aaa\nbbb\nccc" {
		t.Errorf("after undo: %q", buf.String())
	}
	// Should restore all 3 cursors.
	if d.cursorCount() != 3 {
		t.Errorf("cursors after undo=%d want 3", d.cursorCount())
	}
}

func TestDriver_MultiCursorMovement(t *testing.T) {
	buf := buffer.FromBytes([]byte("abcd\nefgh\nijkl"))
	d := newDriver(EditorCfg{
		IDFocus: 53, Buffer: buf, Width: 400, Height: 200,
	})
	// Two cursors at (0,0) and (2,0).
	d.addCursorAt(2, 0)
	d.sendKey(gui.KeyRight)
	// Both should have moved right.
	st := d.state()
	if len(st.Cursors) != 2 {
		t.Fatalf("cursors=%d", len(st.Cursors))
	}
	if st.Cursors[0].Cursor.ByteCol != 1 {
		t.Errorf("cursor0 col=%d want 1", st.Cursors[0].Cursor.ByteCol)
	}
	if st.Cursors[1].Cursor.ByteCol != 1 {
		t.Errorf("cursor1 col=%d want 1", st.Cursors[1].Cursor.ByteCol)
	}
}

func TestDriver_MultiCursorMergesOnOverlap(t *testing.T) {
	buf := buffer.FromBytes([]byte("abc"))
	d := newDriver(EditorCfg{
		IDFocus: 54, Buffer: buf, Width: 400, Height: 200,
	})
	// Two cursors at (0,0) and (0,1). Move right → both at (0,1)
	// and (0,2). Should remain 2 (different positions).
	d.addCursorAt(0, 1)
	d.sendKey(gui.KeyRight)
	if d.cursorCount() != 2 {
		t.Errorf("cursors=%d want 2", d.cursorCount())
	}
	// Move both to end of line → should merge.
	d.sendKey(gui.KeyEnd)
	if d.cursorCount() != 1 {
		t.Errorf("cursors=%d want 1 (merged at EOL)", d.cursorCount())
	}
}

func TestDriver_CtrlD_SelectsWord(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello world"))
	d := newDriver(EditorCfg{
		IDFocus: 55, Buffer: buf, Width: 400, Height: 200,
	})
	// Cursor at start of "hello". Ctrl+D should select it.
	d.sendKeyMod(gui.KeyD, gui.ModCtrl)
	s := d.cursor()
	if s.Anchor != (buffer.Position{Line: 0, ByteCol: 0}) {
		t.Errorf("anchor=%+v want (0,0)", s.Anchor)
	}
	if s.Cursor != (buffer.Position{Line: 0, ByteCol: 5}) {
		t.Errorf("cursor=%+v want (0,5)", s.Cursor)
	}
}

func TestDriver_CtrlD_FindsNext(t *testing.T) {
	buf := buffer.FromBytes([]byte("foo bar foo baz foo"))
	d := newDriver(EditorCfg{
		IDFocus: 56, Buffer: buf, Width: 400, Height: 200,
	})
	// First Ctrl+D selects "foo".
	d.sendKeyMod(gui.KeyD, gui.ModCtrl)
	// Second Ctrl+D finds next "foo" and adds cursor.
	d.sendKeyMod(gui.KeyD, gui.ModCtrl)
	if d.cursorCount() != 2 {
		t.Fatalf("cursors=%d want 2", d.cursorCount())
	}
	// Third Ctrl+D finds the last "foo".
	d.sendKeyMod(gui.KeyD, gui.ModCtrl)
	if d.cursorCount() != 3 {
		t.Fatalf("cursors=%d want 3", d.cursorCount())
	}
}

func TestDriver_Escape_CollapsesCursors(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	d := newDriver(EditorCfg{
		IDFocus: 57, Buffer: buf, Width: 400, Height: 200,
	})
	d.addCursorAt(1, 0)
	d.addCursorAt(2, 0)
	if d.cursorCount() != 3 {
		t.Fatalf("setup: cursors=%d", d.cursorCount())
	}
	d.sendKey(gui.KeyEscape)
	if d.cursorCount() != 1 {
		t.Errorf("after escape: cursors=%d want 1", d.cursorCount())
	}
}

func TestDriver_Escape_ClearsSelection(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	d := newDriver(EditorCfg{
		IDFocus: 58, Buffer: buf, Width: 400, Height: 200,
	})
	// Select some text.
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	s := d.cursor()
	if !s.HasSelection() {
		t.Fatal("should have selection")
	}
	d.sendKey(gui.KeyEscape)
	s = d.cursor()
	if s.HasSelection() {
		t.Error("escape should clear selection")
	}
}

func TestDriver_MultiCursorCopyPaste(t *testing.T) {
	buf := buffer.FromBytes([]byte("aaa\nbbb\nccc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 59, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "aaa" on line 0.
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	d.sendKeyMod(gui.KeyRight, gui.ModShift)
	// Add cursor at (2,0) and select "ccc".
	d.addCursorAt(2, 0)
	// For simplicity, manually set selection on second cursor.
	st := loadState(d.w, d.cfg.IDFocus)
	st.Cursors[1].Anchor = buffer.Position{Line: 2, ByteCol: 0}
	st.Cursors[1].Cursor = buffer.Position{Line: 2, ByteCol: 3}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyC, gui.ModCtrl) // copy
	// Clipboard should be "aaa\nccc".
	d.sendKey(gui.KeyEnd) // collapse selections, cursor at end
	// Move to end of buffer and paste.
	d.sendKey(gui.KeyDown)
	d.sendKey(gui.KeyDown)
	d.sendKey(gui.KeyEnd)
	d.sendKeyMod(gui.KeyV, gui.ModCtrl)
	want := "aaa\nbbb\ncccaaa\nccc"
	if buf.String() != want {
		t.Errorf("buffer=%q want %q", buf.String(), want)
	}
}

func TestDriver_MultiCursorCut(t *testing.T) {
	buf := buffer.FromBytes([]byte("Xaa\nXbb\nXcc"))
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 60, Buffer: buf, Width: 400, Height: 200,
	})
	// Select "X" on each line.
	st := loadState(d.w, d.cfg.IDFocus)
	st.Cursors = []CursorState{
		{Cursor: buffer.Position{Line: 0, ByteCol: 1}, Anchor: buffer.Position{Line: 0, ByteCol: 0}},
		{Cursor: buffer.Position{Line: 1, ByteCol: 1}, Anchor: buffer.Position{Line: 1, ByteCol: 0}},
		{Cursor: buffer.Position{Line: 2, ByteCol: 1}, Anchor: buffer.Position{Line: 2, ByteCol: 0}},
	}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyX, gui.ModCtrl) // cut
	if buf.String() != "aa\nbb\ncc" {
		t.Errorf("buffer=%q want aa\\nbb\\ncc", buf.String())
	}
	// Undo should restore.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	if buf.String() != "Xaa\nXbb\nXcc" {
		t.Errorf("after undo: %q", buf.String())
	}
}

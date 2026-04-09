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
	if d.state().Cursor.ByteCol != 5 {
		t.Errorf("col=%d", d.state().Cursor.ByteCol)
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
	s := d.state()
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
	if d.state().Cursor.Line != 4 {
		t.Fatalf("setup: cursor=%+v", d.state().Cursor)
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
	s := d.state()
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

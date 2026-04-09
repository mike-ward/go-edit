package edit

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// editorAmendLayout runs each frame with *Window access. It loads
// persistent state, lazily builds the text Measurer, recomputes
// per-frame layout metrics, and publishes them via the frame struct
// so OnDraw can read them.
func editorAmendLayout(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Window) {
	return func(layout *gui.Layout, w *gui.Window) {
		st := loadState(w, cfg.IDFocus)
		if st.Measurer == nil {
			st.Measurer = text.New(w, gui.CurrentTheme().M3)
			if st.Measurer == nil {
				// No backend (headless). Bail; draw will no-op.
				frame.valid = false
				return
			}
		}

		lh := st.Measurer.LineHeight()
		advance := st.Measurer.Advance()

		var gutterW float32
		if cfg.ShowLineNumbers {
			digits := len(strconv.Itoa(cfg.Buffer.LineCount()))
			digits = max(digits, 3)
			gutterW = float32(digits)*advance + 2*advance
		}

		// Clamp cursor against current buffer size — the buffer
		// may have been mutated externally between frames.
		clampCursor(&st, cfg.Buffer)
		clampScroll(&st, cfg, lh)

		frame.state = st
		frame.lineHeight = lh
		frame.gutterW = gutterW
		frame.padLeft = advance / 2
		frame.valid = true

		storeState(w, cfg.IDFocus, st)
	}
}

func editorOnKeyDown(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		st := loadState(w, cfg.IDFocus)
		buf := cfg.Buffer

		handled := true
		moved := false
		resetDesired := true

		switch e.KeyCode {
		case gui.KeyLeft:
			moveLeft(&st, buf)
			moved = true
		case gui.KeyRight:
			moveRight(&st, buf)
			moved = true
		case gui.KeyUp:
			moveUp(&st, buf, 1)
			moved = true
			resetDesired = false
		case gui.KeyDown:
			moveDown(&st, buf, 1)
			moved = true
			resetDesired = false
		case gui.KeyHome:
			st.Cursor.ByteCol = 0
			moved = true
		case gui.KeyEnd:
			st.Cursor.ByteCol = len(buf.Line(st.Cursor.Line))
			moved = true
		case gui.KeyPageUp:
			moveUp(&st, buf, pageLines(frame, cfg.Height))
			moved = true
			resetDesired = false
		case gui.KeyPageDown:
			moveDown(&st, buf, pageLines(frame, cfg.Height))
			moved = true
			resetDesired = false
		case gui.KeyBackspace:
			if !cfg.ReadOnly {
				backspace(&st, buf)
				moved = true
			}
		case gui.KeyDelete:
			if !cfg.ReadOnly {
				deleteForward(&st, buf)
				moved = true
			}
		case gui.KeyEnter:
			if !cfg.ReadOnly {
				insertNewline(&st, buf)
				moved = true
			}
		default:
			handled = false
		}

		if !handled {
			return
		}
		if resetDesired {
			st.DesiredCol = st.Cursor.ByteCol
		}
		if moved {
			ensureCursorVisible(&st, frame, cfg.Height)
		}
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnChar(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		if cfg.ReadOnly {
			return
		}
		r := rune(e.CharCode)
		if !acceptChar(r) {
			return
		}
		var buf2 [4]byte
		n := utf8.EncodeRune(buf2[:], r)

		st := loadState(w, cfg.IDFocus)
		pos := st.Cursor
		c := cfg.Buffer.Apply(buffer.Edit{
			Range:    buffer.Range{Start: pos, End: pos},
			NewBytes: buf2[:n],
		})
		st.Cursor = c.AppliedRange.End
		st.DesiredCol = st.Cursor.ByteCol
		ensureCursorVisible(&st, frame, cfg.Height)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnMouseScroll(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		// Guard NaN/Inf from a misbehaving backend.
		dy := e.ScrollY
		if dy != dy || dy > 1e6 || dy < -1e6 {
			return
		}
		st := loadState(w, cfg.IDFocus)
		// Positive ScrollY means scroll up; invert for natural feel.
		st.ScrollY -= dy * frame.lineHeight * 3
		clampScroll(&st, cfg, frame.lineHeight)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

// ---------- Pure cursor math (testable without *Window) ----------

func moveLeft(st *editorState, buf *buffer.Buffer) {
	if st.Cursor.ByteCol > 0 {
		st.Cursor.ByteCol--
		return
	}
	if st.Cursor.Line > 0 {
		st.Cursor.Line--
		st.Cursor.ByteCol = len(buf.Line(st.Cursor.Line))
	}
}

func moveRight(st *editorState, buf *buffer.Buffer) {
	line := buf.Line(st.Cursor.Line)
	if st.Cursor.ByteCol < len(line) {
		st.Cursor.ByteCol++
		return
	}
	if st.Cursor.Line < buf.LineCount()-1 {
		st.Cursor.Line++
		st.Cursor.ByteCol = 0
	}
}

func moveUp(st *editorState, buf *buffer.Buffer, n int) {
	st.Cursor.Line -= n
	if st.Cursor.Line < 0 {
		st.Cursor.Line = 0
	}
	clampCol(st, buf)
}

func moveDown(st *editorState, buf *buffer.Buffer, n int) {
	st.Cursor.Line += n
	if st.Cursor.Line >= buf.LineCount() {
		st.Cursor.Line = buf.LineCount() - 1
	}
	clampCol(st, buf)
}

func clampCol(st *editorState, buf *buffer.Buffer) {
	ll := len(buf.Line(st.Cursor.Line))
	want := st.DesiredCol
	want = min(want, ll)
	st.Cursor.ByteCol = want
}

func backspace(st *editorState, buf *buffer.Buffer) {
	pos := st.Cursor
	if pos.Line == 0 && pos.ByteCol == 0 {
		return
	}
	var start buffer.Position
	if pos.ByteCol > 0 {
		start = buffer.Position{Line: pos.Line, ByteCol: pos.ByteCol - 1}
	} else {
		prevLen := len(buf.Line(pos.Line - 1))
		start = buffer.Position{Line: pos.Line - 1, ByteCol: prevLen}
	}
	c := buf.Apply(buffer.Edit{Range: buffer.Range{Start: start, End: pos}})
	st.Cursor = c.AppliedRange.End
}

func deleteForward(st *editorState, buf *buffer.Buffer) {
	pos := st.Cursor
	lineLen := len(buf.Line(pos.Line))
	var end buffer.Position
	if pos.ByteCol < lineLen {
		end = buffer.Position{Line: pos.Line, ByteCol: pos.ByteCol + 1}
	} else if pos.Line < buf.LineCount()-1 {
		end = buffer.Position{Line: pos.Line + 1, ByteCol: 0}
	} else {
		return
	}
	_ = buf.Apply(buffer.Edit{Range: buffer.Range{Start: pos, End: end}})
}

func insertNewline(st *editorState, buf *buffer.Buffer) {
	pos := st.Cursor
	c := buf.Apply(buffer.Edit{
		Range:    buffer.Range{Start: pos, End: pos},
		NewBytes: []byte{'\n'},
	})
	st.Cursor = c.AppliedRange.End
}

// acceptChar reports whether r should be inserted into the buffer
// when received as a character event. Printable runes and tab pass;
// everything else (control chars, \n/\r, null) is rejected.
func acceptChar(r rune) bool {
	return r == '\t' || unicode.IsPrint(r)
}

func pageLines(frame *editorFrameData, viewportH float32) int {
	if frame.lineHeight <= 0 {
		return 1
	}
	n := int(viewportH / frame.lineHeight)
	n = max(n, 1)
	return n
}

// clampCursor clamps st.Cursor to valid coordinates within buf.
// Called from AmendLayout to recover gracefully from external
// buffer mutations.
func clampCursor(st *editorState, buf *buffer.Buffer) {
	if st.Cursor.Line < 0 {
		st.Cursor.Line = 0
	}
	if st.Cursor.Line >= buf.LineCount() {
		st.Cursor.Line = buf.LineCount() - 1
	}
	ll := len(buf.Line(st.Cursor.Line))
	if st.Cursor.ByteCol < 0 {
		st.Cursor.ByteCol = 0
	}
	if st.Cursor.ByteCol > ll {
		st.Cursor.ByteCol = ll
	}
}

// clampScroll keeps ScrollY within [0, maxScroll]. Also sanitizes
// NaN — if ScrollY went NaN from bad input upstream, snap to 0.
func clampScroll(st *editorState, cfg EditorCfg, lh float32) {
	if st.ScrollY != st.ScrollY { // NaN
		st.ScrollY = 0
	}
	if lh <= 0 {
		st.ScrollY = 0
		return
	}
	maxScroll := float32(cfg.Buffer.LineCount())*lh - cfg.Height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if st.ScrollY > maxScroll {
		st.ScrollY = maxScroll
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}
}

func ensureCursorVisible(st *editorState, frame *editorFrameData, viewportH float32) {
	if !frame.valid || frame.lineHeight <= 0 {
		return
	}
	if viewportH != viewportH || viewportH <= 0 { // NaN or non-positive
		return
	}
	lh := frame.lineHeight
	cy := float32(st.Cursor.Line) * lh
	if cy < st.ScrollY {
		st.ScrollY = cy
	}
	if cy+lh > st.ScrollY+viewportH {
		st.ScrollY = cy + lh - viewportH
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}
}

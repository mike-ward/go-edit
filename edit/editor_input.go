package edit

import (
	"maps"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// resetBlink stamps the current time onto editorState as
// "user activity just happened", restarting the blink cycle so the
// cursor is solid for one full visible half-period before flicking
// off again. No-op when blink is disabled, so callers can sprinkle
// it freely on every input path.
func resetBlink(cfg EditorCfg, st *editorState) {
	if blinkPeriod(cfg) <= 0 {
		return
	}
	st.LastActivityUnixNano = nowOf(cfg).UnixNano()
}

// computeBlink derives frame.cursorVisible from cfg / state and
// schedules the next redraw at the next blink transition. Real-time
// scheduling via time.AfterFunc only fires when cfg.Now is nil
// (production); injecting a fake clock implies the test drives
// AmendLayout manually and does not want background timer firings.
func computeBlink(
	cfg EditorCfg, st *editorState, frame *editorFrameData,
	w *gui.Window,
) {
	period := blinkPeriod(cfg)
	// Seed activity timestamp on first frame so blink starts
	// immediately without waiting for a keystroke.
	if period > 0 && st.LastActivityUnixNano == 0 {
		st.LastActivityUnixNano = nowOf(cfg).UnixNano()
	}
	if period <= 0 {
		frame.cursorVisible = true
		if frame.blinkTimer != nil {
			frame.blinkTimer.Stop()
			frame.blinkTimer = nil
		}
		return
	}
	half := period.Nanoseconds()
	dt := max(nowOf(cfg).UnixNano()-st.LastActivityUnixNano, 0)
	frame.cursorVisible = (dt/half)%2 == 0
	if cfg.Now != nil || w == nil {
		// Test mode: don't schedule background redraws.
		return
	}
	nextIn := max(period-time.Duration(dt%half), minBlinkPeriod)
	if frame.blinkTimer != nil {
		frame.blinkTimer.Stop()
	}
	// QueueCommand + UpdateWindow (not RequestRedraw) for two
	// reasons: (1) QueueCommand calls wakeMain() to post an OS
	// event that wakes the sleeping backend event loop — plain
	// RequestRedraw only sets a flag; (2) UpdateWindow triggers
	// a full layout rebuild so AmendLayout fires and
	// computeBlink recalculates cursorVisible — render-only
	// refreshes skip AmendLayout entirely.
	frame.blinkTimer = time.AfterFunc(nextIn, func() {
		w.QueueCommand(func(w *gui.Window) {
			w.UpdateWindow()
		})
	})
}

// isEditAction reports whether an action ID is a mutating action
// that should be blocked in read-only mode.
func isEditAction(id string) bool {
	if id == "edit.copy" {
		return false
	}
	return strings.HasPrefix(id, "edit.")
}

func editorOnKeyDown(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	// Build keymap stack and action registry once at closure
	// creation time.
	stack := &KeymapStack{}
	stack.Push(DefaultKeymap)
	for _, km := range cfg.Keymaps {
		stack.Push(km)
	}

	actions := make(map[string]Action, len(defaultActions)+4+len(cfg.Actions))
	maps.Copy(actions, defaultActions)
	// Page actions need frame for viewport height.
	for _, a := range []Action{
		pageAction("cursor.pageup", moveUp, false, cfg, frame),
		pageAction("cursor.pagedown", moveDown, false, cfg, frame),
		pageAction("select.pageup", moveUp, true, cfg, frame),
		pageAction("select.pagedown", moveDown, true, cfg, frame),
	} {
		actions[a.ID] = a
	}
	maps.Copy(actions, cfg.Actions)

	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		// While an IME composition is active (or was active
		// this frame), suppress all keymap actions. The IME
		// owns the keyboard; keys like Enter (accept) and
		// Escape (cancel) are consumed by the platform.
		// frame.imeComposing reflects the previous
		// AmendLayout; imeCommitted is set by editorOnChar
		// on commit (the EventChar may fire after KeyDown).
		if frame.imeComposing || frame.imeCommitted {
			e.IsHandled = true
			return
		}

		st := loadState(w, cfg.IDFocus)

		// Overlay intercepts: help and find bar get first crack.
		if st.HelpActive {
			handleHelpKey(&st, e, frame.lineHeight,
				cfg.Height, frame.helpEntries)
			resetBlink(cfg, &st)
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}
		if st.Search.Active {
			if handleSearchKey(cfg, &st, cfg.Buffer, e) {
				ensureCursorVisible(&st, frame, cfg)
				resetBlink(cfg, &st)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
		}

		actionID, ok := stack.Resolve(e.KeyCode, e.Modifiers)
		if !ok {
			return
		}
		action, ok := actions[actionID]
		if !ok {
			return
		}

		// Block edit actions in read-only mode.
		if cfg.ReadOnly && isEditAction(actionID) {
			return
		}

		resetBlink(cfg, &st)

		// Record cursor before edit for undo (skip for undo/redo
		// themselves — they restore cursor from their own records).
		isEdit := isEditAction(actionID)
		if isEdit && actionID != "edit.undo" &&
			actionID != "edit.redo" {
			cfg.Buffer.SetUndoCursorState(buildUndoCursorState(&st))
		}

		if action.PerCursor && len(st.Cursors) > 1 {
			dispatchPerCursor(cfg, &st, cfg.Buffer, w, action, isEdit)
		} else {
			action.Execute(cfg, &st, cfg.Buffer, w)
			applyPostAction(&st, action)
		}

		// Skip cursors past folded ranges after movement.
		if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
			isDown := actionID == "cursor.down" ||
				actionID == "select.down" ||
				actionID == "cursor.pagedown" ||
				actionID == "select.pagedown"
			for i := range st.Cursors {
				if isDown {
					skipFoldsDown(&st.Cursors[i],
						st.FoldedRanges)
				} else {
					skipFoldsUp(&st.Cursors[i],
						st.FoldedRanges)
				}
			}
			clampCursors(&st, cfg.Buffer)
		}

		sortAndMerge(&st)
		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnChar(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		// While composing, the OS sends EventChar for each raw
		// phonetic keystroke (e.g. "k", "a", "n" while
		// building "かん"). These must not be inserted into the
		// buffer — the preedit is visual only. On commit the
		// OS clears composition state before firing the final
		// EventChar, so w.IMEComposing() is false for the
		// commit event but frame.imeComposing (from the
		// previous AmendLayout) is still true.
		//
		// Detect commit vs. mid-composition: w.IMEComposing()
		// is false on commit, true mid-composition.
		if w.IMEComposing() {
			e.IsHandled = true
			return
		}
		if frame.imeComposing {
			// This is the commit event. Mark the frame so
			// editorOnKeyDown suppresses the trailing
			// Enter/Escape the OS sends after the commit.
			frame.imeCommitted = true
		}

		// IME commit: insert the full committed string when it
		// contains multiple codepoints (e.g. Chinese "漢字").
		// Single-rune commits are handled by the normal CharCode
		// path below. All backends set IMEText on every
		// EventChar, so checking len > 1 rune distinguishes
		// true multi-codepoint IME commits from normal typing.
		if utf8.RuneCountInString(e.IMEText) > 1 {
			st := loadState(w, cfg.IDFocus)
			if st.HelpActive {
				e.IsHandled = true
				return
			}
			if st.Search.Active {
				handleSearchString(
					&st, cfg.Buffer, e.IMEText)
				resetBlink(cfg, &st)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
			if cfg.ReadOnly {
				return
			}
			resetBlink(cfg, &st)
			cfg.Buffer.SetUndoCursorState(
				buildUndoCursorState(&st))
			charInsertPerCursor(
				&st, cfg.Buffer, []byte(e.IMEText))
			sortAndMerge(&st)
			ensureCursorVisible(&st, frame, cfg)
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}

		r := rune(e.CharCode)
		if !acceptChar(r) {
			return
		}

		st := loadState(w, cfg.IDFocus)

		// Overlay intercepts: help consumes all chars;
		// find bar routes to search input.
		if st.HelpActive {
			e.IsHandled = true
			return
		}
		if st.Search.Active {
			handleSearchChar(&st, cfg.Buffer, r)
			resetBlink(cfg, &st)
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}

		if cfg.ReadOnly {
			return
		}
		var buf2 [4]byte
		n := utf8.EncodeRune(buf2[:], r)
		resetBlink(cfg, &st)

		// Auto-close: skip over existing closer instead of
		// inserting a duplicate. Check each cursor individually.
		if n == 1 && len(st.Cursors) > 0 {
			pairs := cfg.AutoClosePairs
			if pairs == nil {
				pairs = DefaultAutoClosePairs
			}
			allSkip := true
			for i := range st.Cursors {
				if !shouldSkipCloser(cfg.Buffer,
					st.Cursors[i].Cursor, buf2[0], pairs) {
					allSkip = false
					break
				}
			}
			if allSkip {
				for i := range st.Cursors {
					cs := &st.Cursors[i]
					ll := len(cfg.Buffer.Line(cs.Cursor.Line))
					if cs.Cursor.ByteCol < ll {
						cs.Cursor.ByteCol++
					}
					cs.ClearSelection()
					cs.DesiredCol = cs.Cursor.ByteCol
				}
				sortAndMerge(&st)
				ensureCursorVisible(&st, frame, cfg)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
		}

		cfg.Buffer.SetUndoCursorState(buildUndoCursorState(&st))

		charInsertPerCursor(&st, cfg.Buffer, buf2[:n])

		sortAndMerge(&st)
		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

// ---------- Pure cursor math (testable without *Window) ----------

// cursorNext returns the byte offset of the next valid cursor
// position on the current line. Delegates to go-glyph via
// Measurer when available; falls back to rune-based advance.
func cursorNext(
	line []byte, col int, m *text.Measurer,
) int {
	if m != nil {
		return m.NextCursorPos(line, col)
	}
	_, sz := utf8.DecodeRune(line[col:])
	if sz == 0 {
		sz = 1
	}
	return col + sz
}

// cursorPrev returns the byte offset of the previous valid cursor
// position on the current line. Mirrors cursorNext.
func cursorPrev(
	line []byte, col int, m *text.Measurer,
) int {
	if m != nil {
		return m.PrevCursorPos(line, col)
	}
	_, sz := utf8.DecodeLastRune(line[:col])
	if sz == 0 {
		sz = 1
	}
	return col - sz
}

func moveLeft(
	cs *CursorState, buf *buffer.Buffer, m *text.Measurer,
) {
	if cs.Cursor.ByteCol > 0 {
		line := buf.Line(cs.Cursor.Line)
		cs.Cursor.ByteCol = cursorPrev(
			line, cs.Cursor.ByteCol, m)
		return
	}
	if cs.Cursor.Line > 0 {
		cs.Cursor.Line--
		cs.Cursor.ByteCol = len(buf.Line(cs.Cursor.Line))
	}
}

func moveRight(
	cs *CursorState, buf *buffer.Buffer, m *text.Measurer,
) {
	line := buf.Line(cs.Cursor.Line)
	if cs.Cursor.ByteCol < len(line) {
		cs.Cursor.ByteCol = cursorNext(
			line, cs.Cursor.ByteCol, m)
		return
	}
	if cs.Cursor.Line < buf.LineCount()-1 {
		cs.Cursor.Line++
		cs.Cursor.ByteCol = 0
	}
}

func moveUp(cs *CursorState, buf *buffer.Buffer, n int) {
	cs.Cursor.Line -= n
	if cs.Cursor.Line < 0 {
		cs.Cursor.Line = 0
	}
	clampCol(cs, buf)
}

func moveDown(cs *CursorState, buf *buffer.Buffer, n int) {
	cs.Cursor.Line += n
	if cs.Cursor.Line >= buf.LineCount() {
		cs.Cursor.Line = buf.LineCount() - 1
	}
	clampCol(cs, buf)
}

func clampCol(cs *CursorState, buf *buffer.Buffer) {
	ll := len(buf.Line(cs.Cursor.Line))
	want := cs.DesiredCol
	want = min(want, ll)
	cs.Cursor.ByteCol = want
}

func backspace(
	cs *CursorState, buf *buffer.Buffer, m *text.Measurer,
) {
	pos := cs.Cursor
	if pos.Line == 0 && pos.ByteCol == 0 {
		return
	}
	var start buffer.Position
	if pos.ByteCol > 0 {
		line := buf.Line(pos.Line)
		start = buffer.Position{
			Line:    pos.Line,
			ByteCol: cursorPrev(line, pos.ByteCol, m),
		}
	} else {
		prevLen := len(buf.Line(pos.Line - 1))
		start = buffer.Position{Line: pos.Line - 1, ByteCol: prevLen}
	}
	c := buf.Apply(buffer.Edit{Range: buffer.Range{Start: start, End: pos}})
	cs.Cursor = c.AppliedRange.End
}

func deleteForward(
	cs *CursorState, buf *buffer.Buffer, m *text.Measurer,
) {
	pos := cs.Cursor
	line := buf.Line(pos.Line)
	lineLen := len(line)
	var end buffer.Position
	if pos.ByteCol < lineLen {
		end = buffer.Position{
			Line:    pos.Line,
			ByteCol: cursorNext(line, pos.ByteCol, m),
		}
	} else if pos.Line < buf.LineCount()-1 {
		end = buffer.Position{Line: pos.Line + 1, ByteCol: 0}
	} else {
		return
	}
	_ = buf.Apply(buffer.Edit{Range: buffer.Range{Start: pos, End: end}})
}

func insertNewline(cfg EditorCfg, cs *CursorState, buf *buffer.Buffer) {
	deleteCursorSelection(cs, buf)
	pos := cs.Cursor
	line := buf.Line(pos.Line)

	// Auto-indent: copy leading whitespace from current line.
	indent := leadingWhitespace(line)
	// Open-brace heuristic: add one indent level.
	if pos.ByteCol > 0 && pos.ByteCol <= len(line) &&
		line[pos.ByteCol-1] == '{' {
		indent = append(indent, indentUnit(cfg.Buffer.Props.IndentStyle)...)
	}

	newBytes := make([]byte, 0, 1+len(indent))
	newBytes = append(newBytes, '\n')
	newBytes = append(newBytes, indent...)
	c := buf.Apply(buffer.Edit{
		Range:    buffer.Range{Start: pos, End: pos},
		NewBytes: newBytes,
	})
	cs.Cursor = c.AppliedRange.End
	cs.ClearSelection()
}

// acceptChar reports whether r should be inserted into the buffer
// when received as a character event. Printable runes and tab pass;
// everything else (control chars, \n/\r, null) is rejected.
func acceptChar(r rune) bool {
	return r == '\t' || unicode.IsPrint(r)
}

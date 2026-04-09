package edit

import (
	"maps"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// isEditAction reports whether an action ID is a mutating action
// that should be blocked in read-only mode.
func isEditAction(id string) bool {
	if id == "edit.copy" {
		return false
	}
	return strings.HasPrefix(id, "edit.")
}

// editorAmendLayout runs each frame with *Window access. It loads
// persistent state, lazily builds the text Measurer, recomputes
// per-frame layout metrics, and publishes them via the frame struct
// so OnDraw can read them.
func editorAmendLayout(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Window) {
	invalidateSent := false
	var searchEditRemove func()

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

		// Provide RequestRedraw thunk to async decoration
		// providers once.
		if !invalidateSent && cfg.OnInvalidate != nil {
			cfg.OnInvalidate(w.RequestRedraw)
			invalidateSent = true
		}

		// Sync tab width from buffer indent config each frame.
		if tw := cfg.Buffer.Props.IndentStyle.Width; tw > 0 {
			st.Measurer.TabWidth = tw
		}

		lh := st.Measurer.LineHeight()
		advance := st.Measurer.Advance()

		var gutterW float32
		if cfg.ShowLineNumbers {
			digits := len(strconv.Itoa(cfg.Buffer.LineCount()))
			digits = max(digits, 3)
			gutterW = float32(digits)*advance + 2*advance
		}

		// Clamp cursors against current buffer size — the buffer
		// may have been mutated externally between frames.
		clampCursors(&st, cfg.Buffer)
		clampScroll(&st, cfg, lh)

		// Recompute search matches when query/flags changed or
		// buffer was edited.
		if st.Search.Active && len(st.Search.Query) > 0 &&
			needsRecompute(&st.Search) {
			recomputeMatches(&st, cfg.Buffer)
		}
		// Register/remove buffer edit observer for match
		// invalidation.
		if st.Search.Active && searchEditRemove == nil {
			searchEditRemove = cfg.Buffer.OnEdit(func(_ buffer.Change) {
				// Mark dirty; recompute on next AmendLayout.
				st := loadState(w, cfg.IDFocus)
				st.Search.matchesDirty = true
				storeState(w, cfg.IDFocus, st)
			})
		} else if !st.Search.Active && searchEditRemove != nil {
			searchEditRemove()
			searchEditRemove = nil
		}

		frame.state = st
		frame.lineHeight = lh
		frame.gutterW = gutterW
		frame.padLeft = advance / 2
		frame.valid = true

		storeState(w, cfg.IDFocus, st)
	}
}

func editorOnKeyDown(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	// Build keymap stack and action registry once at closure
	// creation time.
	stack := &KeymapStack{}
	stack.Push(DefaultKeymap)
	for _, km := range cfg.Keymaps {
		stack.Push(km)
	}

	actions := make(map[string]Action, len(defaultActions)+6)
	maps.Copy(actions, defaultActions)
	// Page actions need frame for viewport height.
	for _, a := range []Action{
		pageUpAction(cfg, frame),
		pageDownAction(cfg, frame),
		selectPageUpAction(cfg, frame),
		selectPageDownAction(cfg, frame),
	} {
		actions[a.ID] = a
	}
	maps.Copy(actions, cfg.Actions)

	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		// When find bar is active, route keys there first.
		{
			st := loadState(w, cfg.IDFocus)
			if st.Search.Active {
				if handleSearchKey(cfg, &st, cfg.Buffer, e) {
					ensureCursorVisible(&st, frame, cfg.Height)
					storeState(w, cfg.IDFocus, st)
					e.IsHandled = true
					return
				}
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

		st := loadState(w, cfg.IDFocus)

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

		sortAndMerge(&st)
		ensureCursorVisible(&st, frame, cfg.Height)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnChar(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		r := rune(e.CharCode)
		if !acceptChar(r) {
			return
		}

		// When find bar is active, route chars there.
		{
			st := loadState(w, cfg.IDFocus)
			if st.Search.Active {
				handleSearchChar(&st, cfg.Buffer, r)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
		}

		if cfg.ReadOnly {
			return
		}
		var buf2 [4]byte
		n := utf8.EncodeRune(buf2[:], r)

		st := loadState(w, cfg.IDFocus)
		cfg.Buffer.SetUndoCursorState(buildUndoCursorState(&st))

		charInsertPerCursor(&st, cfg.Buffer, buf2[:n])

		sortAndMerge(&st)
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

func moveLeft(cs *CursorState, buf *buffer.Buffer) {
	if cs.Cursor.ByteCol > 0 {
		cs.Cursor.ByteCol--
		return
	}
	if cs.Cursor.Line > 0 {
		cs.Cursor.Line--
		cs.Cursor.ByteCol = len(buf.Line(cs.Cursor.Line))
	}
}

func moveRight(cs *CursorState, buf *buffer.Buffer) {
	line := buf.Line(cs.Cursor.Line)
	if cs.Cursor.ByteCol < len(line) {
		cs.Cursor.ByteCol++
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

func backspace(cs *CursorState, buf *buffer.Buffer) {
	pos := cs.Cursor
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
	cs.Cursor = c.AppliedRange.End
}

func deleteForward(cs *CursorState, buf *buffer.Buffer) {
	pos := cs.Cursor
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

func pageLines(frame *editorFrameData, viewportH float32) int {
	if frame.lineHeight <= 0 {
		return 1
	}
	n := int(viewportH / frame.lineHeight)
	n = max(n, 1)
	return n
}

// clampCursors clamps all cursors to valid coordinates within buf.
// Called from AmendLayout to recover gracefully from external
// buffer mutations.
func clampCursors(st *editorState, buf *buffer.Buffer) {
	for i := range st.Cursors {
		clampPos(&st.Cursors[i].Cursor, buf)
		clampPos(&st.Cursors[i].Anchor, buf)
	}
}

func clampPos(p *buffer.Position, buf *buffer.Buffer) {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= buf.LineCount() {
		p.Line = buf.LineCount() - 1
	}
	ll := len(buf.Line(p.Line))
	if p.ByteCol < 0 {
		p.ByteCol = 0
	}
	if p.ByteCol > ll {
		p.ByteCol = ll
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
	p := st.primary()
	lh := frame.lineHeight
	cy := float32(p.Cursor.Line) * lh
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

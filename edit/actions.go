package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// defaultActions maps action IDs to their implementations.
// This is the single source of truth for built-in editor
// actions; the default keymap and any user keymaps reference
// these by string ID.
//
// Actions without PreservesAnchor have Anchor = Cursor applied
// automatically after execution by the dispatch in editorOnKeyDown.
var defaultActions = map[string]Action{
	// ---- cursor movement ----

	"cursor.left": {
		ID:        "cursor.left",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if p.HasSelection() {
				p.Cursor = p.SelectionRange().Start
				return
			}
			moveLeft(p, buf, st.Measurer)
		},
	},
	"cursor.right": {
		ID:        "cursor.right",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if p.HasSelection() {
				p.Cursor = p.SelectionRange().End
				return
			}
			moveRight(p, buf, st.Measurer)
		},
	},
	// cursor.up, cursor.down, select.up, select.down are
	// registered as closure overrides in editorOnKeyDown so they
	// can branch on frame.wrapActive.

	"cursor.home": {
		ID:        "cursor.home",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.primary().Cursor.ByteCol = 0
		},
	},
	"cursor.end": {
		ID:        "cursor.end",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			p.Cursor.ByteCol = len(buf.Line(p.Cursor.Line))
		},
	},

	// ---- selection (extends from Anchor) ----

	"select.left": {
		ID:              "select.left",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveLeft(st.primary(), buf, st.Measurer)
		},
	},
	"select.right": {
		ID:              "select.right",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveRight(st.primary(), buf, st.Measurer)
		},
	},
	"select.home": {
		ID:              "select.home",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.primary().Cursor.ByteCol = 0
		},
	},
	"select.end": {
		ID:              "select.end",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			p.Cursor.ByteCol = len(buf.Line(p.Cursor.Line))
		},
	},
	"select.all": {
		ID:              "select.all",
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			p.Anchor = buffer.Position{}
			lastLine := buf.LineCount() - 1
			p.Cursor = buffer.Position{
				Line:    lastLine,
				ByteCol: len(buf.Line(lastLine)),
			}
		},
	},

	// ---- editing ----

	"edit.backspace": {
		ID:        "edit.backspace",
		PerCursor: true,
		Execute: func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if p.HasSelection() {
				buf.BeginGroup()
				deleteCursorSelection(p, buf)
				buf.EndGroup()
				return
			}
			// Auto-close: delete both opener and closer.
			pairs := cfg.AutoClosePairs
			if pairs == nil {
				pairs = DefaultAutoClosePairs
			}
			if len(pairs) > 0 &&
				shouldDeletePair(buf, p.Cursor, pairs) {
				start := buffer.Position{
					Line:    p.Cursor.Line,
					ByteCol: p.Cursor.ByteCol - 1,
				}
				end := buffer.Position{
					Line:    p.Cursor.Line,
					ByteCol: p.Cursor.ByteCol + 1,
				}
				c := buf.Apply(buffer.Edit{
					Range: buffer.Range{Start: start, End: end},
				})
				p.Cursor = c.AppliedRange.Start
				return
			}
			backspace(p, buf, st.Measurer)
		},
	},
	"edit.delete": {
		ID:        "edit.delete",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if p.HasSelection() {
				buf.BeginGroup()
				deleteCursorSelection(p, buf)
				buf.EndGroup()
				return
			}
			deleteForward(p, buf, st.Measurer)
		},
	},
	"edit.newline": {
		ID:        "edit.newline",
		PerCursor: true,
		Execute: func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			buf.BeginGroup()
			insertNewline(cfg, st.primary(), buf)
			buf.EndGroup()
		},
	},
	"edit.undo": {
		ID: "edit.undo",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			r := buf.Undo()
			if r.OK {
				restoreCursorsFromUndo(st, r.Cursor)
			}
		},
	},
	"edit.redo": {
		ID: "edit.redo",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			r := buf.Redo()
			if r.OK {
				restoreCursorsFromUndo(st, r.Cursor)
			}
		},
	},

	// ---- clipboard ----

	"edit.copy": {
		ID: "edit.copy",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, w *gui.Window) {
			text := collectSelections(st, buf)
			if len(text) > 0 {
				w.SetClipboard(text)
			}
		},
		PreservesAnchor: true,
	},
	"edit.cut": {
		ID: "edit.cut",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, w *gui.Window) {
			text := collectSelections(st, buf)
			if len(text) == 0 {
				return
			}
			w.SetClipboard(text)
			multiCursorDeleteSelections(st, buf)
		},
	},
	"edit.paste": {
		ID: "edit.paste",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, w *gui.Window) {
			text := w.GetClipboard()
			if len(text) == 0 {
				return
			}
			if len(text) > buffer.MaxLoadBytes {
				text = text[:buffer.MaxLoadBytes]
			}
			multiCursorPaste(st, buf, text)
		},
	},

	// ---- multi-cursor ----

	"cursor.addNext": {
		ID:              "cursor.addNext",
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if !p.HasSelection() {
				// First press: select word under cursor.
				line := buf.Line(p.Cursor.Line)
				start, end := wordBoundsAtByte(line, p.Cursor.ByteCol)
				p.Anchor = buffer.Position{
					Line: p.Cursor.Line, ByteCol: start,
				}
				p.Cursor = buffer.Position{
					Line: p.Cursor.Line, ByteCol: end,
				}
				return
			}
			// Find next occurrence of the selected text.
			needle := []byte(buf.TextInRange(p.SelectionRange()))
			// Search from the last cursor's position.
			last := st.Cursors[len(st.Cursors)-1]
			found, ok := findNext(buf, needle, last.Cursor)
			if !ok {
				return
			}
			addCursor(st, CursorState{
				Cursor:     found.End,
				Anchor:     found.Start,
				DesiredCol: found.End.ByteCol,
			})
		},
	},
	"cursor.escape": {
		ID: "cursor.escape",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			if len(st.Cursors) > 1 {
				collapseToPrimary(st)
			} else {
				// Single cursor: clear selection.
				st.primary().ClearSelection()
			}
		},
	},

	// ---- bracket ----

	"cursor.matchBracket": matchBracketAction("cursor.matchBracket", false),
	"select.matchBracket": matchBracketAction("select.matchBracket", true),

	// ---- find ----

	"find.open": {
		ID: "find.open",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			openFindBar(st, buf, false)
		},
	},
	"find.openReplace": {
		ID: "find.openReplace",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			openFindBar(st, buf, true)
		},
	},

	// ---- folding ----

	"fold.toggle": {
		ID: "fold.toggle",
		Execute: func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			if !cfg.EnableFolding {
				return
			}
			st.FoldedRanges = toggleFold(
				st.FoldedRanges, buf,
				st.primary().Cursor.Line,
				resolveTabWidth(st.Measurer))
		},
	},
	"fold.all": {
		ID: "fold.all",
		Execute: func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			if !cfg.EnableFolding {
				return
			}
			st.FoldedRanges = foldAll(buf,
				resolveTabWidth(st.Measurer))
		},
	},
	"fold.unfoldAll": {
		ID: "fold.unfoldAll",
		Execute: func(cfg EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			if !cfg.EnableFolding {
				return
			}
			st.FoldedRanges = nil
		},
	},

	// ---- view ----

	"view.toggleStickyScroll": {
		ID: "view.toggleStickyScroll",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.StickyScrollOverride = toggleOverride(st.StickyScrollOverride)
		},
	},
	"view.toggleWrap": {
		ID: "view.toggleWrap",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.WrapOverride = toggleOverride(st.WrapOverride)
		},
	},
	"view.toggleWhitespace": {
		ID: "view.toggleWhitespace",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.WhitespaceOverride = cycleWhitespace(
				st.WhitespaceOverride)
		},
	},

	// ---- comment ----

	"edit.toggleComment": {
		ID:              "edit.toggleComment",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			toggleComment(cfg, st.primary(), buf)
		},
	},

	// ---- help ----

	"help.show": {
		ID: "help.show",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			st.HelpActive = !st.HelpActive
			if !st.HelpActive {
				st.HelpScrollY = 0
			}
		},
	},

	// ---- indent ----

	"edit.indent": {
		ID:        "edit.indent",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			indentAction(st, buf)
		},
		PreservesAnchor: true,
	},
	"edit.dedent": {
		ID:        "edit.dedent",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			dedentAction(st, buf)
		},
		PreservesAnchor: true,
	},
}

// toggleOverride cycles a bool override: 0→1 (on), 1→2 (off),
// *→1 (on).
func toggleOverride(v int) int {
	if v == 1 {
		return 2
	}
	return 1
}

// matchBracketAction builds a bracket-match action.
func matchBracketAction(id string, preservesAnchor bool) Action {
	return Action{
		ID:              id,
		PerCursor:       true,
		PreservesAnchor: preservesAnchor,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if _, m, ok, _ := findMatchingBracket(buf, p.Cursor); ok {
				p.Cursor = m
			}
		},
	}
}

// wrapAwareUpDown builds a closure-based Up/Down action that
// branches on frame.wrapActive: visual sub-row movement when
// wrap is on, logical line movement otherwise.
func wrapAwareUpDown(
	id string,
	preservesAnchor bool,
	frame *editorFrameData,
) Action {
	isDown := id == "cursor.down" || id == "select.down"
	return Action{
		ID:              id,
		PerCursor:       true,
		PreservesAnchor: preservesAnchor,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if !frame.wrapActive {
				if isDown {
					moveDown(p, buf, 1)
				} else {
					moveUp(p, buf, 1)
				}
				return
			}
			if isDown {
				moveDownVisual(p, buf, st.Measurer,
					frame.wrapWidth, st.FoldedRanges)
			} else {
				moveUpVisual(p, buf, st.Measurer,
					frame.wrapWidth, st.FoldedRanges)
			}
		},
		PreservesDesiredCol: true,
	}
}

// pageAction builds a page-movement action. moveFn is moveUp or
// moveDown; preservesAnchor distinguishes cursor vs select variants.
// When wrap is active, moves by visual rows instead of logical lines.
func pageAction(
	id string,
	moveFn func(*CursorState, *buffer.Buffer, int),
	preservesAnchor bool,
	cfg EditorCfg,
	frame *editorFrameData,
) Action {
	isDown := id == "cursor.pagedown" || id == "select.pagedown"
	return Action{
		ID:              id,
		PerCursor:       true,
		PreservesAnchor: preservesAnchor,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			n := pageLines(frame, cfg.Height)
			p := st.primary()
			if !frame.wrapActive {
				moveFn(p, buf, n)
				return
			}
			// Move by N visual rows (capped to prevent DoS from
			// degenerate viewport/lineHeight combinations).
			n = min(n, 1000)
			for range n {
				if isDown {
					moveDownVisual(p, buf, st.Measurer,
						frame.wrapWidth, st.FoldedRanges)
				} else {
					moveUpVisual(p, buf, st.Measurer,
						frame.wrapWidth, st.FoldedRanges)
				}
			}
		},
		PreservesDesiredCol: true,
	}
}

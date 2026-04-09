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
			moveLeft(p, buf)
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
			moveRight(p, buf)
		},
	},
	"cursor.up": {
		ID:        "cursor.up",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveUp(st.primary(), buf, 1)
		},
		PreservesDesiredCol: true,
	},
	"cursor.down": {
		ID:        "cursor.down",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveDown(st.primary(), buf, 1)
		},
		PreservesDesiredCol: true,
	},
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
			moveLeft(st.primary(), buf)
		},
	},
	"select.right": {
		ID:              "select.right",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveRight(st.primary(), buf)
		},
	},
	"select.up": {
		ID:                  "select.up",
		PerCursor:           true,
		PreservesAnchor:     true,
		PreservesDesiredCol: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveUp(st.primary(), buf, 1)
		},
	},
	"select.down": {
		ID:                  "select.down",
		PerCursor:           true,
		PreservesAnchor:     true,
		PreservesDesiredCol: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveDown(st.primary(), buf, 1)
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
			backspace(p, buf)
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
			deleteForward(p, buf)
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

	"cursor.matchBracket": {
		ID:        "cursor.matchBracket",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if m, ok := findMatchingBracket(buf, p.Cursor); ok {
				p.Cursor = m
			}
		},
	},
	"select.matchBracket": {
		ID:              "select.matchBracket",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			p := st.primary()
			if m, ok := findMatchingBracket(buf, p.Cursor); ok {
				p.Cursor = m
			}
		},
	},

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
			switch st.StickyScrollOverride {
			case 0:
				st.StickyScrollOverride = 1
			case 1:
				st.StickyScrollOverride = 2
			default:
				st.StickyScrollOverride = 1
			}
		},
	},
	"view.toggleWrap": {
		ID: "view.toggleWrap",
		Execute: func(_ EditorCfg, st *editorState, _ *buffer.Buffer, _ *gui.Window) {
			switch st.WrapOverride {
			case 0:
				st.WrapOverride = 1 // force on
			case 1:
				st.WrapOverride = 2 // force off
			default:
				st.WrapOverride = 1 // back to on
			}
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

// pageUpAction and pageDownAction need EditorCfg for viewport
// height, so they're registered separately as closures.
func pageUpAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID:        "cursor.pageup",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveUp(st.primary(), buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

func pageDownAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID:        "cursor.pagedown",
		PerCursor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveDown(st.primary(), buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

// selectPageUpAction and selectPageDownAction extend selection.
func selectPageUpAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID:              "select.pageup",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveUp(st.primary(), buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

func selectPageDownAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID:              "select.pagedown",
		PerCursor:       true,
		PreservesAnchor: true,
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer, _ *gui.Window) {
			moveDown(st.primary(), buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

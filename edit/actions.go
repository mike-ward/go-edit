package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// defaultActions maps action IDs to their implementations.
// This is the single source of truth for built-in editor
// actions; the default keymap and any user keymaps reference
// these by string ID.
var defaultActions = map[string]Action{
	"cursor.left": {
		ID:      "cursor.left",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { moveLeft(st, buf) },
	},
	"cursor.right": {
		ID:      "cursor.right",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { moveRight(st, buf) },
	},
	"cursor.up": {
		ID:                  "cursor.up",
		Execute:             func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { moveUp(st, buf, 1) },
		PreservesDesiredCol: true,
	},
	"cursor.down": {
		ID:                  "cursor.down",
		Execute:             func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { moveDown(st, buf, 1) },
		PreservesDesiredCol: true,
	},
	"cursor.home": {
		ID:      "cursor.home",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { st.Cursor.ByteCol = 0 },
	},
	"cursor.end": {
		ID: "cursor.end",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) {
			st.Cursor.ByteCol = len(buf.Line(st.Cursor.Line))
		},
	},
	"edit.backspace": {
		ID:      "edit.backspace",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { backspace(st, buf) },
	},
	"edit.delete": {
		ID:      "edit.delete",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { deleteForward(st, buf) },
	},
	"edit.newline": {
		ID:      "edit.newline",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) { insertNewline(st, buf) },
	},
}

// pageUpAction and pageDownAction need EditorCfg for viewport
// height, so they're registered separately as closures.
func pageUpAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID: "cursor.pageup",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) {
			moveUp(st, buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

func pageDownAction(cfg EditorCfg, frame *editorFrameData) Action {
	return Action{
		ID: "cursor.pagedown",
		Execute: func(_ EditorCfg, st *editorState, buf *buffer.Buffer) {
			moveDown(st, buf, pageLines(frame, cfg.Height))
		},
		PreservesDesiredCol: true,
	}
}

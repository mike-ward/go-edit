package edit

import "github.com/mike-ward/go-gui/gui"

// CursorPos returns the primary cursor's line and byte-column
// for the editor instance identified by focusID. Returns
// (0, 0, false) if no state exists for that ID.
func CursorPos(w *gui.Window, focusID uint32) (line, col int, ok bool) {
	if w == nil {
		return 0, 0, false
	}
	sm := gui.StateMapRead[uint32, editorState](w, nsEdit)
	if sm == nil {
		return 0, 0, false
	}
	st, exists := sm.Get(focusID)
	if !exists || len(st.Cursors) == 0 {
		return 0, 0, false
	}
	p := st.Cursors[0].Cursor
	return p.Line, p.ByteCol, true
}

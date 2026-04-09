package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// CursorState holds position, selection anchor, and sticky column
// for one cursor. Multiple CursorStates enable multi-cursor editing.
type CursorState struct {
	Cursor     buffer.Position
	Anchor     buffer.Position // Anchor == Cursor → no selection
	DesiredCol int             // sticky col for Up/Down movement
}

// HasSelection reports whether this cursor has an active selection.
func (cs *CursorState) HasSelection() bool {
	return cs.Anchor != cs.Cursor
}

// SelectionRange returns the ordered [Start, End) range of the
// selection. If no selection, returns an empty range at Cursor.
func (cs *CursorState) SelectionRange() buffer.Range {
	return orderedRange(cs.Anchor, cs.Cursor)
}

// ClearSelection collapses the selection by moving Anchor to Cursor.
func (cs *CursorState) ClearSelection() {
	cs.Anchor = cs.Cursor
}

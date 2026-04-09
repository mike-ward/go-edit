package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// orderedRange returns a range with Start <= End.
func orderedRange(a, b buffer.Position) buffer.Range {
	if a.After(b) {
		a, b = b, a
	}
	return buffer.Range{Start: a, End: b}
}

// deleteCursorSelection deletes the selected text of a single
// CursorState. Returns true if a selection existed and was deleted.
func deleteCursorSelection(cs *CursorState, buf *buffer.Buffer) bool {
	if !cs.HasSelection() {
		return false
	}
	sel := cs.SelectionRange()
	buf.Apply(buffer.Edit{Range: sel})
	cs.Cursor = sel.Start
	cs.Anchor = sel.Start
	return true
}

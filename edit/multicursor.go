package edit

import (
	"bytes"
	"slices"
	"strings"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// maxCursors caps the number of simultaneous cursors to prevent
// unbounded growth from programmatic abuse.
const maxCursors = 1000

// sortCursors sorts cursors by position ascending (line, then col).
// Stable sort preserves the primary cursor's relative order among
// equal positions.
func sortCursors(cs []CursorState) {
	slices.SortStableFunc(cs, func(a, b CursorState) int {
		if a.Cursor.Line != b.Cursor.Line {
			return a.Cursor.Line - b.Cursor.Line
		}
		return a.Cursor.ByteCol - b.Cursor.ByteCol
	})
}

// mergeCursors merges overlapping or touching cursors in a sorted
// slice. Returns the (possibly shorter) result slice. The merged
// cursor inherits the union of both selection ranges.
func mergeCursors(cs []CursorState) []CursorState {
	if len(cs) <= 1 {
		return cs
	}
	out := cs[:1]
	for i := 1; i < len(cs); i++ {
		prev := &out[len(out)-1]
		cur := cs[i]
		pr := prev.SelectionRange()
		cr := cur.SelectionRange()
		if overlapsOrTouches(pr, cr) {
			// Merge: take union of ranges.
			unionStart := pr.Start
			if cr.Start.Before(unionStart) {
				unionStart = cr.Start
			}
			unionEnd := pr.End
			if cr.End.After(unionEnd) {
				unionEnd = cr.End
			}
			// Keep the later cursor position as the active end.
			prev.Cursor = unionEnd
			prev.Anchor = unionStart
			if prev.DesiredCol < cur.DesiredCol {
				prev.DesiredCol = cur.DesiredCol
			}
		} else {
			out = append(out, cur)
		}
	}
	return out
}

// overlapsOrTouches reports whether two ordered ranges overlap or
// are immediately adjacent (touching).
func overlapsOrTouches(a, b buffer.Range) bool {
	// a.End >= b.Start and b.End >= a.Start → overlap or touch.
	return !a.End.Before(b.Start) && !b.End.Before(a.Start)
}

// sortAndMerge sorts cursors by position and merges overlapping
// ones. The primary cursor (index 0 before sort) may move; this
// is acceptable because ensureCursorVisible always uses index 0.
func sortAndMerge(st *editorState) {
	if len(st.Cursors) <= 1 {
		return
	}
	sortCursors(st.Cursors)
	st.Cursors = mergeCursors(st.Cursors)
}

// addCursor appends a cursor, sorts, and merges. Respects
// maxCursors cap.
func addCursor(st *editorState, c CursorState) {
	if len(st.Cursors) >= maxCursors {
		return
	}
	st.Cursors = append(st.Cursors, c)
	sortAndMerge(st)
}

// collapseToPrimary drops all cursors except index 0.
func collapseToPrimary(st *editorState) {
	if len(st.Cursors) <= 1 {
		return
	}
	st.Cursors = st.Cursors[:1]
}

// dispatchPerCursor executes an action on each cursor independently.
// Edit actions run in reverse position order with a PostEditFunc
// observer that auto-adjusts non-active cursors after each Apply.
// Movement actions run on all cursors without position adjustment.
//
// Invariant: actions must not append/remove entries in st.Cursors.
// The swap trick relies on a fixed-length slice throughout the loop.
func dispatchPerCursor(
	cfg EditorCfg,
	st *editorState,
	buf *buffer.Buffer,
	w *gui.Window,
	action Action,
	isEdit bool,
) {
	if len(st.Cursors) == 0 {
		return
	}
	if isEdit {
		buf.BeginGroup()
	}

	// Track which cursor index is currently executing so the
	// observer can skip it during adjustment.
	activeIdx := -1

	// Register observer to adjust non-active cursors after each
	// Apply call inside the action.
	if isEdit {
		remove := buf.OnEdit(func(c buffer.Change) {
			adjustCursorsAfterEdit(st.Cursors, activeIdx, c)
		})
		defer remove()
	}

	indices := reversePositionOrder(st.Cursors)
	for _, idx := range indices {
		activeIdx = idx
		// Temporarily make this cursor the "primary" (index 0)
		// for the action, then swap back.
		if idx != 0 {
			st.Cursors[0], st.Cursors[idx] = st.Cursors[idx], st.Cursors[0]
			activeIdx = 0 // after swap, the active is now at 0
		}

		action.Execute(cfg, st, buf, w)
		applyPostAction(st, action)

		if idx != 0 {
			st.Cursors[0], st.Cursors[idx] = st.Cursors[idx], st.Cursors[0]
		}
	}

	if isEdit {
		buf.EndGroup()
	}
}

// applyPostAction applies anchor-collapse and desiredCol reset
// to the primary cursor after action execution.
func applyPostAction(st *editorState, action Action) {
	p := st.primary()
	if !action.PreservesAnchor {
		p.Anchor = p.Cursor
	}
	if !action.PreservesDesiredCol {
		p.DesiredCol = p.Cursor.ByteCol
	}
}

// adjustCursorsAfterEdit shifts cursor positions in response to a
// buffer edit. Called after each per-cursor edit to keep remaining
// cursors in sync. skipIdx is the cursor that just edited (already
// positioned correctly).
//
// Inlines adjustPos's early-return check so cursors strictly
// before the edit avoid two function calls on the hot path —
// important for large multi-cursor groups where this observer
// fires once per edit per cursor (O(N*M) for N cursors × M edits).
func adjustCursorsAfterEdit(cursors []CursorState, skipIdx int, c buffer.Change) {
	delStart := c.Applied.Range.Start
	delEnd := c.Applied.Range.End
	endPos := c.AppliedRange.End
	for i := range cursors {
		if i == skipIdx {
			continue
		}
		cs := &cursors[i]
		// Fast skip: both cursor and anchor are strictly before
		// the edit → no shift, no selection crossing.
		if cs.Cursor.Before(delStart) && cs.Anchor.Before(delStart) {
			continue
		}
		adjustPos(&cs.Cursor, delStart, delEnd, endPos)
		adjustPos(&cs.Anchor, delStart, delEnd, endPos)
	}
}

// charInsertPerCursor inserts bytes at each cursor position,
// processing in reverse position order. Multi-cursor edits are
// wrapped in a single undo group; single-cursor edits are not
// grouped so typing coalescing still works.
func charInsertPerCursor(st *editorState, buf *buffer.Buffer, data []byte) {
	if len(st.Cursors) == 0 {
		return
	}
	if len(st.Cursors) == 1 {
		// Single cursor: preserve coalescing behavior.
		cs := &st.Cursors[0]
		grouped := cs.HasSelection()
		if grouped {
			buf.BeginGroup()
			deleteCursorSelection(cs, buf)
		}
		pos := cs.Cursor
		c := buf.Apply(buffer.Edit{
			Range:    buffer.Range{Start: pos, End: pos},
			NewBytes: data,
		})
		if grouped {
			buf.EndGroup()
		}
		cs.Cursor = c.AppliedRange.End
		cs.ClearSelection()
		cs.DesiredCol = cs.Cursor.ByteCol
		return
	}

	// Multi-cursor: group all edits into one undo entry.
	activeIdx := -1
	buf.BeginGroup()
	remove := buf.OnEdit(func(c buffer.Change) {
		adjustCursorsAfterEdit(st.Cursors, activeIdx, c)
	})

	indices := reversePositionOrder(st.Cursors)
	for _, idx := range indices {
		activeIdx = idx
		cs := &st.Cursors[idx]
		if cs.HasSelection() {
			deleteCursorSelection(cs, buf)
		}
		pos := cs.Cursor
		c := buf.Apply(buffer.Edit{
			Range:    buffer.Range{Start: pos, End: pos},
			NewBytes: data,
		})
		cs.Cursor = c.AppliedRange.End
		cs.ClearSelection()
		cs.DesiredCol = cs.Cursor.ByteCol
	}

	remove()
	buf.EndGroup()
}

// buildUndoCursorState captures all cursors into an UndoCursorState
// for the undo system.
func buildUndoCursorState(st *editorState) buffer.UndoCursorState {
	p := st.primary()
	ucs := buffer.UndoCursorState{
		Cursor: p.Cursor,
		Anchor: p.Anchor,
	}
	if len(st.Cursors) > 1 {
		ucs.Extra = make([]buffer.CursorPair, len(st.Cursors)-1)
		for i := 1; i < len(st.Cursors); i++ {
			ucs.Extra[i-1] = buffer.CursorPair{
				Cursor: st.Cursors[i].Cursor,
				Anchor: st.Cursors[i].Anchor,
			}
		}
	}
	return ucs
}

// restoreCursorsFromUndo restores all cursors from an
// UndoCursorState returned by Undo/Redo. Caps at maxCursors
// to guard against corrupt undo records.
func restoreCursorsFromUndo(st *editorState, ucs buffer.UndoCursorState) {
	st.Cursors = st.Cursors[:0]
	st.Cursors = append(st.Cursors, CursorState{
		Cursor:     ucs.Cursor,
		Anchor:     ucs.Anchor,
		DesiredCol: ucs.Cursor.ByteCol,
	})
	extra := ucs.Extra
	if len(extra) > maxCursors-1 {
		extra = extra[:maxCursors-1]
	}
	for _, e := range extra {
		st.Cursors = append(st.Cursors, CursorState{
			Cursor:     e.Cursor,
			Anchor:     e.Anchor,
			DesiredCol: e.Cursor.ByteCol,
		})
	}
}

// reversePositionOrder returns cursor indices sorted by position
// descending (highest position first). Used for per-cursor edits
// to avoid position invalidation. Stack-allocated for <= 8 cursors.
func reversePositionOrder(cursors []CursorState) []int {
	var buf [8]int
	var indices []int
	if len(cursors) <= len(buf) {
		indices = buf[:len(cursors)]
	} else {
		indices = make([]int, len(cursors))
	}
	for i := range indices {
		indices[i] = i
	}
	slices.SortFunc(indices, func(a, b int) int {
		ca, cb := cursors[a].Cursor, cursors[b].Cursor
		if ca.Line != cb.Line {
			return cb.Line - ca.Line // descending
		}
		return cb.ByteCol - ca.ByteCol // descending
	})
	return indices
}

// collectSelections returns the concatenation of all cursor
// selections separated by newlines. Returns "" if no selection.
func collectSelections(st *editorState, buf *buffer.Buffer) string {
	var parts []string
	for i := range st.Cursors {
		cs := &st.Cursors[i]
		if cs.HasSelection() {
			parts = append(parts, buf.TextInRange(cs.SelectionRange()))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// multiCursorDeleteSelections deletes all cursor selections in
// reverse position order, wrapped in one undo group.
func multiCursorDeleteSelections(st *editorState, buf *buffer.Buffer) {
	if len(st.Cursors) == 0 {
		return
	}
	activeIdx := -1
	buf.BeginGroup()
	remove := buf.OnEdit(func(c buffer.Change) {
		adjustCursorsAfterEdit(st.Cursors, activeIdx, c)
	})

	indices := reversePositionOrder(st.Cursors)
	for _, idx := range indices {
		activeIdx = idx
		deleteCursorSelection(&st.Cursors[idx], buf)
	}

	remove()
	buf.EndGroup()
	sortAndMerge(st)
}

// multiCursorPaste pastes text at each cursor. If the clipboard
// has exactly len(cursors)-1 newlines, each cursor gets one line.
// Otherwise each cursor gets the full text.
func multiCursorPaste(st *editorState, buf *buffer.Buffer, text string) {
	if len(st.Cursors) == 0 {
		return
	}
	n := len(st.Cursors)
	lines := strings.SplitN(text, "\n", n+1)
	perCursor := len(lines) == n

	activeIdx := -1
	buf.BeginGroup()
	remove := buf.OnEdit(func(c buffer.Change) {
		adjustCursorsAfterEdit(st.Cursors, activeIdx, c)
	})

	// Pre-convert once for the broadcast case to avoid
	// repeated []byte(text) allocations per cursor.
	var fullData []byte
	if !perCursor {
		fullData = []byte(text)
	}

	indices := reversePositionOrder(st.Cursors)
	for _, idx := range indices {
		activeIdx = idx
		cs := &st.Cursors[idx]
		deleteCursorSelection(cs, buf)
		var data []byte
		if perCursor {
			data = []byte(lines[idx])
		} else {
			data = fullData
		}
		pos := cs.Cursor
		c := buf.Apply(buffer.Edit{
			Range:    buffer.Range{Start: pos, End: pos},
			NewBytes: data,
		})
		cs.Cursor = c.AppliedRange.End
		cs.ClearSelection()
	}

	remove()
	buf.EndGroup()
	sortAndMerge(st)
}

// findNext searches for needle in buf starting after from,
// wrapping around to the start. Returns the range of the match
// and true, or zero Range and false if not found.
func findNext(buf *buffer.Buffer, needle []byte, from buffer.Position) (buffer.Range, bool) {
	if len(needle) == 0 {
		return buffer.Range{}, false
	}
	total := buf.LineCount()

	// Guard negative / out-of-range from position.
	if from.Line < 0 {
		from.Line = 0
	}
	if from.Line >= total {
		from.Line = 0
		from.ByteCol = 0
	}
	if from.ByteCol < 0 {
		from.ByteCol = 0
	}

	// Search from the starting line to end of buffer.
	for li := from.Line; li < total; li++ {
		line := buf.Line(li)
		startCol := 0
		if li == from.Line {
			startCol = from.ByteCol
		}
		if startCol > len(line) {
			continue
		}
		if idx := bytes.Index(line[startCol:], needle); idx >= 0 {
			col := startCol + idx
			r := buffer.Range{
				Start: buffer.Position{Line: li, ByteCol: col},
				End:   buffer.Position{Line: li, ByteCol: col + len(needle)},
			}
			return r, true
		}
	}

	// Wrap around: search from start of buffer to from position.
	for li := 0; li <= from.Line && li < total; li++ {
		line := buf.Line(li)
		endCol := len(line)
		if li == from.Line {
			endCol = from.ByteCol
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		searchArea := line[:endCol]
		if idx := bytes.Index(searchArea, needle); idx >= 0 {
			r := buffer.Range{
				Start: buffer.Position{Line: li, ByteCol: idx},
				End:   buffer.Position{Line: li, ByteCol: idx + len(needle)},
			}
			return r, true
		}
	}

	return buffer.Range{}, false
}

// adjustPos shifts a position after a delete [delStart, delEnd)
// replaced by content ending at endPos. Mirrors the mark
// adjustment logic in buffer/markset.go.
func adjustPos(p *buffer.Position, delStart, delEnd, endPos buffer.Position) {
	// Before the edit: unchanged.
	if p.Before(delStart) {
		return
	}
	// Inside deleted range: collapse to endPos.
	if !p.After(delEnd) {
		*p = endPos
		return
	}
	// After deleted range: shift by delta.
	shiftPosition(p, delStart, delEnd, endPos)
}

// shiftPosition adjusts a position that's strictly after delEnd.
func shiftPosition(p *buffer.Position, _, delEnd, endPos buffer.Position) {
	lineDelta := endPos.Line - delEnd.Line
	if p.Line == delEnd.Line {
		// Same line as delete end: adjust column.
		p.ByteCol = max(p.ByteCol-delEnd.ByteCol+endPos.ByteCol, 0)
		p.Line = endPos.Line
	} else {
		p.Line += lineDelta
	}
	if p.Line < 0 {
		p.Line = 0
		p.ByteCol = 0
	}
}

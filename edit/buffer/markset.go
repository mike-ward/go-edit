package buffer

// MaxMarks caps the number of marks per buffer. Prevents
// unbounded memory growth from a runaway consumer.
const MaxMarks = 1 << 20 // ~1M marks

// MarkSet holds all marks for a buffer. Marks are stored in a flat
// slice and adjusted in O(n) per Apply call where n = len(marks).
// Designed for < 1K marks (cursors, bracket pairs, search hits).
// For higher counts (LSP diagnostics, git blame), consider batching
// or replacing with an interval tree. See ROADMAP "Future (post-1.0)".
type MarkSet struct {
	nextID uint32
	marks  []*Mark
}

// NewMark creates a mark at pos with the given gravity. Returns
// nil if the mark set has reached MaxMarks.
func (ms *MarkSet) NewMark(pos Position, g Gravity) *Mark {
	if len(ms.marks) >= MaxMarks {
		return nil
	}
	ms.nextID++
	if ms.nextID == 0 {
		ms.nextID = 1 // skip zero on uint32 wrap
	}
	m := &Mark{pos: pos, gravity: g, id: ms.nextID}
	ms.marks = append(ms.marks, m)
	return m
}

// NewRange creates a tracked range [start, end). Start gets
// GravityRight (inserts at start push it right, expanding the
// range). End gets GravityLeft (inserts at end stay inside).
func (ms *MarkSet) NewRange(start, end Position) TrackedRange {
	return TrackedRange{
		Start: ms.NewMark(start, GravityRight),
		End:   ms.NewMark(end, GravityLeft),
	}
}

// Remove deletes a mark from the set. No-op if nil or not found.
func (ms *MarkSet) Remove(m *Mark) {
	if m == nil {
		return
	}
	for i, candidate := range ms.marks {
		if candidate == m {
			ms.marks[i] = ms.marks[len(ms.marks)-1]
			ms.marks[len(ms.marks)-1] = nil
			ms.marks = ms.marks[:len(ms.marks)-1]
			return
		}
	}
}

// RemoveRange removes both marks of a TrackedRange.
func (ms *MarkSet) RemoveRange(tr TrackedRange) {
	ms.Remove(tr.Start)
	ms.Remove(tr.End)
}

// Len returns the number of marks.
func (ms *MarkSet) Len() int { return len(ms.marks) }

// adjust updates all marks after a successful edit. Called by
// Buffer.Apply after the mutation.
//
// Parameters:
//   - e: the (clamped) Edit that was applied
//   - endPos: the position after the inserted bytes
func (ms *MarkSet) adjust(e Edit, endPos Position) {
	if ms == nil {
		return
	}

	delStart := e.Range.Start
	delEnd := e.Range.End
	hasDelete := !e.Range.Empty()
	hasInsert := endPos != delStart // something was inserted

	for _, m := range ms.marks {
		switch {
		case m.pos.Before(delStart):
			// Before edit: unchanged.

		case hasDelete && posInRange(m.pos, delStart, delEnd):
			// Inside deleted range: collapse.
			if m.gravity == GravityLeft {
				m.pos = delStart
			} else {
				m.pos = endPos
			}

		case hasDelete && !m.pos.Before(delEnd):
			// After deleted range: shift by delta.
			m.pos = shiftPos(m.pos, delStart, delEnd, endPos)

		case !hasDelete && hasInsert && m.pos == delStart:
			// Pure insert at mark position: gravity decides.
			if m.gravity == GravityRight {
				m.pos = endPos
			}
			// GravityLeft: stays at delStart.

		case !hasDelete && hasInsert && m.pos.After(delStart):
			// After insert point (no delete): shift.
			m.pos = shiftPos(m.pos, delStart, delStart, endPos)
		}
	}
}

// posInRange reports whether p is strictly inside (start, end).
func posInRange(p, start, end Position) bool {
	return p.After(start) && p.Before(end)
}

// shiftPos adjusts pos for the replacement of [delStart, delEnd)
// with content ending at endPos.
func shiftPos(pos, delStart, delEnd, endPos Position) Position {
	if pos.Line == delEnd.Line {
		// Same line as delete-end: adjust column.
		colAfterDel := pos.ByteCol - delEnd.ByteCol
		pos.Line = endPos.Line
		pos.ByteCol = endPos.ByteCol + colAfterDel
	} else {
		// Lines after delete-end: shift line number.
		lineDelta := endPos.Line - delEnd.Line
		pos.Line += lineDelta
	}
	return pos
}

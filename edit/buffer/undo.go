package buffer

import "time"

// CursorPair holds a cursor and anchor position for one cursor
// in a multi-cursor undo record.
type CursorPair struct {
	Cursor Position
	Anchor Position
}

// UndoCursorState captures cursor and selection anchor for undo
// restore. Opaque to the buffer package; set by the editor layer
// via SetUndoCursor before Apply. Extra holds additional cursors
// beyond the primary; nil for single-cursor edits.
type UndoCursorState struct {
	Cursor Position
	Anchor Position
	Extra  []CursorPair // additional cursors; nil = single cursor
}

// UndoResult is returned by Undo/Redo.
type UndoResult struct {
	OK     bool
	Cursor UndoCursorState
}

// undoEntry is one logical undo step. Compound edits (e.g.
// paste = deleteSelection + insert) produce a single entry with
// multiple changes.
type undoEntry struct {
	changes      []Change
	cursorBefore UndoCursorState
	cursorAfter  UndoCursorState
	coalescable  bool      // next single-char edit may coalesce
	timestamp    time.Time // for coalesce timeout
}

// coalesceTimeout is the maximum duration between edits that can
// be coalesced into one undo entry.
const coalesceTimeout = 500 * time.Millisecond

// maxCoalesceLen caps the number of changes in a single coalesced
// undo entry. Prevents unbounded growth when typing without pause.
const maxCoalesceLen = 4096

// maxUndoEntries caps total undo stack depth. Oldest entries are
// silently discarded when exceeded. Prevents unbounded memory
// growth in long sessions.
const maxUndoEntries = 10_000

// maxGroupNesting caps BeginGroup nesting to guard against
// runaway or buggy callers.
const maxGroupNesting = 64

// undoStack is the linear undo/redo history.
type undoStack struct {
	undo     []undoEntry
	redo     []undoEntry
	grouping int      // >0 → inside BeginGroup/EndGroup
	pending  []Change // changes during current group
	cleanIdx int      // len(undo) at last MarkClean; -1 = never
	now      func() time.Time

	// Cursor state set by the editor layer before Apply.
	curBefore    UndoCursorState
	hasCurBefore bool
}

// EnableUndo activates undo tracking. now is the clock source
// (pass time.Now for production, a fake for tests). Safe to call
// multiple times; subsequent calls are no-ops.
func (b *Buffer) EnableUndo(now func() time.Time) {
	if b.undo != nil {
		return
	}
	if now == nil {
		now = time.Now
	}
	b.undo = &undoStack{
		cleanIdx: -1,
		now:      now,
	}
}

// CanUndo reports whether an undo step is available.
func (b *Buffer) CanUndo() bool {
	return b.undo != nil && len(b.undo.undo) > 0
}

// CanRedo reports whether a redo step is available.
func (b *Buffer) CanRedo() bool {
	return b.undo != nil && len(b.undo.redo) > 0
}

// BeginGroup starts a compound edit. All Apply calls until the
// matching EndGroup are collapsed into a single undo entry.
// Nestable: only the outermost EndGroup flushes.
func (b *Buffer) BeginGroup() {
	if b.undo == nil {
		return
	}
	if b.undo.grouping >= maxGroupNesting {
		return
	}
	b.undo.grouping++
}

// EndGroup ends a compound edit. Panics are avoided: unmatched
// EndGroup is a no-op.
func (b *Buffer) EndGroup() {
	if b.undo == nil {
		return
	}
	if b.undo.grouping <= 0 {
		return
	}
	b.undo.grouping--
	if b.undo.grouping == 0 && len(b.undo.pending) > 0 {
		b.undo.flush()
	}
}

// SetUndoCursor records cursor state before the next Apply. The
// editor layer calls this before each edit action so undo can
// restore the cursor position.
func (b *Buffer) SetUndoCursor(cursor, anchor Position) {
	if b.undo == nil {
		return
	}
	b.undo.curBefore = UndoCursorState{Cursor: cursor, Anchor: anchor}
	b.undo.hasCurBefore = true
}

// maxUndoCursors caps the number of cursor pairs stored per undo
// entry. Prevents unbounded memory growth from pathological input.
const maxUndoCursors = 1024

// SetUndoCursorState records full multi-cursor state before Apply.
func (b *Buffer) SetUndoCursorState(state UndoCursorState) {
	if b.undo == nil {
		return
	}
	if len(state.Extra) > maxUndoCursors {
		state.Extra = state.Extra[:maxUndoCursors]
	}
	b.undo.curBefore = state
	b.undo.hasCurBefore = true
}

// Undo reverts the most recent undo entry and pushes it onto the
// redo stack. Returns the cursor state to restore.
func (b *Buffer) Undo() UndoResult {
	if b.undo == nil || len(b.undo.undo) == 0 {
		return UndoResult{}
	}
	us := b.undo
	entry := us.undo[len(us.undo)-1]
	us.undo = us.undo[:len(us.undo)-1]

	// Replay changes in reverse, inverting each.
	for i := len(entry.changes) - 1; i >= 0; i-- {
		c := entry.changes[i]
		b.applyCore(Edit{
			Range:    c.AppliedRange,
			NewBytes: c.OldBytes,
		}, false)
	}

	us.redo = append(us.redo, entry)
	b.updateDirtyFromUndo()
	return UndoResult{
		OK:     true,
		Cursor: entry.cursorBefore,
	}
}

// Redo replays the most recently undone entry and pushes it back
// onto the undo stack. Returns the cursor state to restore.
func (b *Buffer) Redo() UndoResult {
	if b.undo == nil || len(b.undo.redo) == 0 {
		return UndoResult{}
	}
	us := b.undo
	entry := us.redo[len(us.redo)-1]
	us.redo = us.redo[:len(us.redo)-1]

	// Replay changes forward.
	for i := range entry.changes {
		c := entry.changes[i]
		entry.changes[i] = b.applyCore(c.Applied, false)
	}

	us.undo = append(us.undo, entry)
	b.updateDirtyFromUndo()
	return UndoResult{
		OK:     true,
		Cursor: entry.cursorAfter,
	}
}

// record is called from Apply (when not replaying undo/redo) to
// push a change onto the undo stack.
func (us *undoStack) record(c Change, curBefore UndoCursorState, hasCur bool) {
	// Any new edit clears the redo stack.
	us.redo = us.redo[:0]

	if us.grouping > 0 {
		us.pending = append(us.pending, c)
		return
	}

	now := us.now()
	if us.tryCoalesce(c, now) {
		return
	}

	before := curBefore
	if !hasCur {
		// Fallback: use the edit start as cursor position.
		before = UndoCursorState{
			Cursor: c.Applied.Range.Start,
			Anchor: c.Applied.Range.Start,
		}
	}

	entry := undoEntry{
		changes:      []Change{c},
		cursorBefore: before,
		cursorAfter: UndoCursorState{
			Cursor: c.AppliedRange.End,
			Anchor: c.AppliedRange.End,
		},
		coalescable: isCoalescable(c),
		timestamp:   now,
	}
	us.pushUndo(entry)
}

// pushUndo appends entry and evicts the oldest when the stack
// exceeds maxUndoEntries. Adjusts cleanIdx accordingly.
func (us *undoStack) pushUndo(entry undoEntry) {
	us.undo = append(us.undo, entry)
	if len(us.undo) > maxUndoEntries {
		// Drop oldest entry.
		copy(us.undo, us.undo[1:])
		us.undo = us.undo[:len(us.undo)-1]
		// Clean mark shifts with the eviction.
		if us.cleanIdx > 0 {
			us.cleanIdx--
		} else {
			// Clean point evicted; buffer is permanently dirty
			// until next MarkClean.
			us.cleanIdx = -1
		}
	}
}

// flush pushes accumulated grouped changes as one undo entry.
func (us *undoStack) flush() {
	if len(us.pending) == 0 {
		return
	}
	last := us.pending[len(us.pending)-1]
	entry := undoEntry{
		changes:      us.pending,
		cursorBefore: us.curBefore,
		cursorAfter: UndoCursorState{
			Cursor: last.AppliedRange.End,
			Anchor: last.AppliedRange.End,
		},
		coalescable: false,
		timestamp:   us.now(),
	}
	us.pushUndo(entry)
	us.pending = nil
}

// tryCoalesce attempts to merge c into the top undo entry.
// Returns true if coalesced.
func (us *undoStack) tryCoalesce(c Change, now time.Time) bool {
	if len(us.undo) == 0 {
		return false
	}
	top := &us.undo[len(us.undo)-1]
	if !top.coalescable {
		return false
	}
	if now.Sub(top.timestamp) > coalesceTimeout {
		return false
	}
	if len(top.changes) == 0 || len(top.changes) >= maxCoalesceLen {
		return false
	}
	prev := top.changes[len(top.changes)-1]

	// Coalesce adjacent single-char inserts (typing).
	if isSingleCharInsert(c) && isSingleCharInsert(prev) {
		if c.Applied.NewBytes[0] == '\n' {
			return false
		}
		if c.Applied.Range.Start == prev.AppliedRange.End {
			top.changes = append(top.changes, c)
			top.cursorAfter = UndoCursorState{
				Cursor: c.AppliedRange.End,
				Anchor: c.AppliedRange.End,
			}
			top.timestamp = now
			return true
		}
	}

	// Coalesce adjacent single-char deletes (backspace or forward).
	if isSingleCharDelete(c) && isSingleCharDelete(prev) {
		if c.OldBytes[0] == '\n' {
			return false
		}
		// Backspace: new end == previous start.
		// Forward delete: same start position.
		if c.Applied.Range.End == prev.Applied.Range.Start ||
			c.Applied.Range.Start == prev.Applied.Range.Start {
			top.changes = append(top.changes, c)
			top.cursorAfter = UndoCursorState{
				Cursor: c.AppliedRange.End,
				Anchor: c.AppliedRange.End,
			}
			top.timestamp = now
			return true
		}
	}

	return false
}

// isCoalescable reports whether a change is eligible to start a
// coalesce chain (single-char insert or single-char delete, no
// newline).
func isCoalescable(c Change) bool {
	if isSingleCharInsert(c) {
		return c.Applied.NewBytes[0] != '\n'
	}
	if isSingleCharDelete(c) {
		return c.OldBytes[0] != '\n'
	}
	return false
}

func isSingleCharInsert(c Change) bool {
	return c.Applied.Range.Empty() && len(c.Applied.NewBytes) == 1
}

func isSingleCharDelete(c Change) bool {
	return !c.Applied.Range.Empty() && len(c.Applied.NewBytes) == 0 &&
		len(c.OldBytes) == 1
}

// updateDirtyFromUndo sets the dirty flag based on whether the
// current undo stack depth matches the clean mark.
func (b *Buffer) updateDirtyFromUndo() {
	if b.undo == nil || b.undo.cleanIdx < 0 {
		b.dirty = true
		return
	}
	b.dirty = len(b.undo.undo) != b.undo.cleanIdx
}

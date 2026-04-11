package edit

import (
	"slices"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// FoldRange represents a folded region. StartLine is the visible
// header; lines (StartLine+1) through EndLine (inclusive) are hidden.
type FoldRange struct {
	StartLine int
	EndLine   int
}

// foldRangeAt computes the foldable range starting at line,
// using indent-based detection. Returns the range and true, or
// false if line is not foldable.
func foldRangeAt(buf *buffer.Buffer, line int, tabWidth int) (FoldRange, bool) {
	if buf == nil || line < 0 || line >= buf.LineCount()-1 {
		return FoldRange{}, false
	}
	if tabWidth <= 0 {
		tabWidth = text.DefaultTabWidth
	}
	baseIndent := lineIndent(buf.Line(line), tabWidth)
	// A line is foldable if the next non-blank line is more indented.
	end := -1
	for i := line + 1; i < buf.LineCount(); i++ {
		lb := buf.Line(i)
		if isBlankLine(lb) {
			continue
		}
		ind := lineIndent(lb, tabWidth)
		if ind <= baseIndent {
			break
		}
		end = i
	}
	if end < 0 {
		return FoldRange{}, false
	}
	// Include trailing blank lines within the fold.
	for end+1 < buf.LineCount() && isBlankLine(buf.Line(end+1)) {
		// Only include if followed by a line at base indent or less
		// (i.e., the blank line separates from the next block).
		if end+2 < buf.LineCount() {
			nextInd := lineIndent(buf.Line(end+2), tabWidth)
			if nextInd > baseIndent {
				break
			}
		}
		end++
	}
	return FoldRange{StartLine: line, EndLine: end}, true
}

// toggleFold adds or removes a fold at line. Preserves the
// foldRangeInvariant: returned slice is sorted by StartLine with no
// overlapping ranges.
func toggleFold(folds []FoldRange, buf *buffer.Buffer, line int, tabWidth int) []FoldRange {
	if buf == nil || line < 0 || line >= buf.LineCount() {
		return folds
	}
	// If line is a fold header, remove it (preserves sort order).
	if idx, found := findFoldByStart(folds, line); found {
		return slices.Delete(folds, idx, idx+1)
	}
	// Reject creation inside an existing fold to keep ranges
	// non-overlapping. Under normal UI flow the cursor is snapped out
	// of folds so this is defensive.
	if isFolded(folds, line) {
		return folds
	}
	// Otherwise, try to create a fold.
	fr, ok := foldRangeAt(buf, line, tabWidth)
	if !ok {
		return folds
	}
	// Reject creation if the new range would overlap an existing
	// fold (e.g. fr.EndLine reaches into or past the next header).
	if nextFoldStart, ok := nextFoldStartAfter(folds, line); ok {
		if fr.EndLine >= nextFoldStart {
			return folds
		}
	}
	folds = append(folds, fr)
	sortFolds(folds)
	return folds
}

// foldAll folds all top-level foldable blocks.
func foldAll(buf *buffer.Buffer, tabWidth int) []FoldRange {
	if buf == nil {
		return nil
	}
	var folds []FoldRange
	for i := 0; i < buf.LineCount(); {
		fr, ok := foldRangeAt(buf, i, tabWidth)
		if ok {
			folds = append(folds, fr)
			i = fr.EndLine + 1
		} else {
			i++
		}
	}
	return folds
}

// foldRangeInvariant: callers of binary-search helpers below must
// uphold "folds sorted by StartLine ascending, no overlapping
// ranges." All mutators (toggleFold, foldAll, invalidateFolds,
// unfoldAt) preserve it; sortFolds restores it. Tests assert it
// via checkFoldInvariant after every mutation.

// checkFoldInvariant reports the first invariant break in folds,
// or "" when the slice is sorted and non-overlapping. Intended
// for test assertions, not runtime checks — mutators are trusted.
func checkFoldInvariant(folds []FoldRange) string {
	for i := range folds {
		f := folds[i]
		if f.EndLine < f.StartLine {
			return "end < start"
		}
		if i > 0 {
			prev := folds[i-1]
			if f.StartLine <= prev.StartLine {
				return "unsorted StartLine"
			}
			if f.StartLine <= prev.EndLine {
				return "overlapping ranges"
			}
		}
	}
	return ""
}

// sortFolds sorts fold ranges by StartLine.
func sortFolds(folds []FoldRange) {
	slices.SortFunc(folds, func(a, b FoldRange) int {
		return a.StartLine - b.StartLine
	})
}

// findFoldByStart returns the index of the fold whose StartLine
// equals line. Binary search; assumes foldRangeInvariant.
func findFoldByStart(folds []FoldRange, line int) (int, bool) {
	return slices.BinarySearchFunc(folds, line,
		func(f FoldRange, target int) int {
			return f.StartLine - target
		})
}

// foldContaining returns the index of the fold whose range covers
// line (header or interior). Binary search; assumes foldRangeInvariant.
func foldContaining(folds []FoldRange, line int) (int, bool) {
	// Find the greatest StartLine <= line.
	idx, exact := slices.BinarySearchFunc(folds, line,
		func(f FoldRange, target int) int {
			return f.StartLine - target
		})
	if !exact {
		idx--
	}
	if idx < 0 || idx >= len(folds) {
		return -1, false
	}
	if line >= folds[idx].StartLine && line <= folds[idx].EndLine {
		return idx, true
	}
	return -1, false
}

// nextFoldStartAfter returns the StartLine of the first fold with
// StartLine > line, if any.
func nextFoldStartAfter(folds []FoldRange, line int) (int, bool) {
	idx, _ := slices.BinarySearchFunc(folds, line+1,
		func(f FoldRange, target int) int {
			return f.StartLine - target
		})
	if idx >= len(folds) {
		return 0, false
	}
	return folds[idx].StartLine, true
}

// isFoldHeader reports whether line is the start of a fold.
func isFoldHeader(folds []FoldRange, line int) bool {
	_, ok := findFoldByStart(folds, line)
	return ok
}

// isFolded reports whether line is hidden (inside a fold, not
// the header).
func isFolded(folds []FoldRange, line int) bool {
	idx, ok := foldContaining(folds, line)
	if !ok {
		return false
	}
	return line > folds[idx].StartLine
}

// nextVisible returns the next visible line at or after line,
// skipping folded ranges. If line itself is visible, returns line.
func nextVisible(folds []FoldRange, line int) int {
	idx, ok := foldContaining(folds, line)
	if !ok || line == folds[idx].StartLine {
		return line
	}
	return folds[idx].EndLine + 1
}

// prevVisible returns the previous visible line at or before line.
func prevVisible(folds []FoldRange, line int) int {
	idx, ok := foldContaining(folds, line)
	if !ok || line == folds[idx].StartLine {
		return line
	}
	return folds[idx].StartLine
}

// visibleLineCount returns the number of visible lines,
// accounting for folds.
func visibleLineCount(total int, folds []FoldRange) int {
	hidden := 0
	for _, f := range folds {
		if f.EndLine > f.StartLine {
			hidden += f.EndLine - f.StartLine
		}
	}
	result := total - hidden
	if result < 1 {
		return 1
	}
	return result
}

// visibleToLogical converts a visible line index (0-based,
// counting only visible lines) to a logical line index.
func visibleToLogical(visLine int, folds []FoldRange) int {
	if visLine < 0 {
		return 0
	}
	logical := 0
	vis := 0
	for _, f := range folds {
		// Lines before this fold header (exclusive).
		gap := f.StartLine - logical
		if vis+gap > visLine {
			return logical + (visLine - vis)
		}
		vis += gap
		// The header itself is visible: +1.
		if vis == visLine {
			return f.StartLine
		}
		vis++ // count the header
		// Skip the hidden lines.
		logical = f.EndLine + 1
	}
	// After all folds.
	return logical + (visLine - vis)
}

// logicalToVisible converts a logical line to a visible line index.
func logicalToVisible(line int, folds []FoldRange) int {
	if line < 0 {
		return 0
	}
	vis := line
	for _, f := range folds {
		if f.StartLine >= line {
			break
		}
		hidden := min(f.EndLine, line-1) - f.StartLine
		vis -= hidden
	}
	return vis
}

// snapCursorOutOfFold moves a cursor to the fold header if it's
// inside a folded range.
func snapCursorOutOfFold(cs *CursorState, folds []FoldRange) {
	if cs == nil {
		return
	}
	idx, ok := foldContaining(folds, cs.Cursor.Line)
	if !ok || cs.Cursor.Line == folds[idx].StartLine {
		return
	}
	cs.Cursor.Line = folds[idx].StartLine
	cs.Cursor.ByteCol = 0
	cs.ClearSelection()
}

// skipFoldsDown moves a cursor past folded ranges when moving
// down. If the cursor landed inside a fold, jump to the line
// after the fold.
func skipFoldsDown(cs *CursorState, folds []FoldRange) {
	if cs == nil {
		return
	}
	idx, ok := foldContaining(folds, cs.Cursor.Line)
	if !ok || cs.Cursor.Line == folds[idx].StartLine {
		return
	}
	cs.Cursor.Line = folds[idx].EndLine + 1
}

// skipFoldsUp moves a cursor before folded ranges when moving up.
// If the cursor landed inside a fold, jump to the fold header.
func skipFoldsUp(cs *CursorState, folds []FoldRange) {
	snapCursorOutOfFold(cs, folds)
}

// unfoldAt removes any fold that contains line (as header or
// interior). Returns the updated slice.
func unfoldAt(folds []FoldRange, line int) []FoldRange {
	return slices.DeleteFunc(folds, func(f FoldRange) bool {
		return line >= f.StartLine && line <= f.EndLine
	})
}

// invalidateFolds removes folds overlapping the edited range.
func invalidateFolds(folds []FoldRange, c buffer.Change) []FoldRange {
	r := c.AppliedRange
	return slices.DeleteFunc(folds, func(f FoldRange) bool {
		// If the edit touches any line in the fold range,
		// unfold it.
		return r.Start.Line <= f.EndLine && r.End.Line >= f.StartLine
	})
}

// isFoldable reports whether line can be the start of a fold.
func isFoldable(buf *buffer.Buffer, line int, tabWidth int) bool {
	_, ok := foldRangeAt(buf, line, tabWidth)
	return ok
}

// resolveTabWidth returns the effective tab width from a measurer,
// falling back to DefaultTabWidth if nil or zero.
func resolveTabWidth(m *text.Measurer) int {
	if m != nil && m.TabWidth > 0 {
		return m.TabWidth
	}
	return text.DefaultTabWidth
}

// lineIndent returns the visual indent level (columns) of a line.
func lineIndent(line []byte, tabWidth int) int {
	if tabWidth <= 0 {
		tabWidth = text.DefaultTabWidth
	}
	return text.VisualCols(line, leadingWhitespaceLen(line), tabWidth)
}

// leadingWhitespaceLen returns the byte length of leading
// whitespace.
func leadingWhitespaceLen(line []byte) int {
	for i, b := range line {
		if b != ' ' && b != '\t' {
			return i
		}
	}
	return len(line)
}

// isBlankLine reports whether a line is empty or all whitespace.
func isBlankLine(line []byte) bool {
	return leadingWhitespaceLen(line) == len(line)
}

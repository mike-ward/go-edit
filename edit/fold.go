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

// toggleFold adds or removes a fold at line.
func toggleFold(folds []FoldRange, buf *buffer.Buffer, line int, tabWidth int) []FoldRange {
	if buf == nil || line < 0 || line >= buf.LineCount() {
		return folds
	}
	// If line is a fold header, remove it.
	for i, f := range folds {
		if f.StartLine == line {
			return slices.Delete(folds, i, i+1)
		}
	}
	// Otherwise, try to create a fold.
	fr, ok := foldRangeAt(buf, line, tabWidth)
	if !ok {
		return folds
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

// sortFolds sorts fold ranges by StartLine.
func sortFolds(folds []FoldRange) {
	slices.SortFunc(folds, func(a, b FoldRange) int {
		return a.StartLine - b.StartLine
	})
}

// isFoldHeader reports whether line is the start of a fold.
func isFoldHeader(folds []FoldRange, line int) bool {
	for _, f := range folds {
		if f.StartLine == line {
			return true
		}
	}
	return false
}

// isFolded reports whether line is hidden (inside a fold, not
// the header).
func isFolded(folds []FoldRange, line int) bool {
	for _, f := range folds {
		if line > f.StartLine && line <= f.EndLine {
			return true
		}
	}
	return false
}

// nextVisible returns the next visible line at or after line,
// skipping folded ranges. If line itself is visible, returns line.
func nextVisible(folds []FoldRange, line int) int {
	for _, f := range folds {
		if line > f.StartLine && line <= f.EndLine {
			return f.EndLine + 1
		}
	}
	return line
}

// prevVisible returns the previous visible line at or before line.
func prevVisible(folds []FoldRange, line int) int {
	for i := len(folds) - 1; i >= 0; i-- {
		f := folds[i]
		if line > f.StartLine && line <= f.EndLine {
			return f.StartLine
		}
	}
	return line
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
	for _, f := range folds {
		if cs.Cursor.Line > f.StartLine &&
			cs.Cursor.Line <= f.EndLine {
			cs.Cursor.Line = f.StartLine
			cs.Cursor.ByteCol = 0
			cs.ClearSelection()
			return
		}
	}
}

// skipFoldsDown moves a cursor past folded ranges when moving
// down. If the cursor landed inside a fold, jump to the line
// after the fold.
func skipFoldsDown(cs *CursorState, folds []FoldRange) {
	if cs == nil {
		return
	}
	for _, f := range folds {
		if cs.Cursor.Line > f.StartLine &&
			cs.Cursor.Line <= f.EndLine {
			cs.Cursor.Line = f.EndLine + 1
			return
		}
	}
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

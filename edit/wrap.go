package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// wrapEntry describes wrap breaks for a single logical line.
type wrapEntry struct {
	Line      int   // logical line index
	BreakCols []int // byte offsets where sub-rows start (excl. 0)
}

// subRows returns the number of visual rows this line occupies.
func (we *wrapEntry) subRows() int { return len(we.BreakCols) + 1 }

// wrapMap caches the logical-to-visual-row mapping for a range
// of visible lines. Built per-frame in AmendLayout.
type wrapMap struct {
	entries   []wrapEntry
	firstLine int     // logical line of entries[0]
	wrapWidth float32 // pixel width used for wrapping
}

// buildWrapMap computes wrap entries for logical lines in
// [firstLine, lastLine]. Only lines that exceed wrapWidth are
// wrapped. folds are applied first (folded lines are skipped).
func buildWrapMap(
	buf *buffer.Buffer,
	m *text.Measurer,
	wrapWidth float32,
	firstLine, lastLine int,
	folds []FoldRange,
) *wrapMap {
	if buf == nil || m == nil || wrapWidth <= 0 ||
		wrapWidth != wrapWidth { // NaN
		return nil
	}
	wm := &wrapMap{
		firstLine: firstLine,
		wrapWidth: wrapWidth,
	}
	for line := firstLine; line <= lastLine && line < buf.LineCount(); {
		if len(folds) > 0 && isFolded(folds, line) {
			line = nextVisible(folds, line)
			continue
		}
		lb := buf.Line(line)
		breaks := computeBreaks(lb, m, wrapWidth)
		wm.entries = append(wm.entries, wrapEntry{
			Line:      line,
			BreakCols: breaks,
		})
		line++
		if len(folds) > 0 {
			line = nextVisible(folds, line)
		}
	}
	return wm
}

// computeBreaks finds byte offsets where a line should wrap.
// Prefers breaking at word boundaries (space/tab); falls back to
// hard break at wrapWidth.
func computeBreaks(
	lineBytes []byte,
	m *text.Measurer,
	wrapWidth float32,
) []int {
	if len(lineBytes) == 0 || m == nil ||
		wrapWidth <= 0 || wrapWidth != wrapWidth {
		return nil
	}
	// Quick check: if the entire line fits, no breaks.
	totalW := m.XForColumn(lineBytes, len(lineBytes))
	if totalW <= wrapWidth {
		return nil
	}

	const maxBreaks = 10_000
	var breaks []int
	subStart := 0 // byte offset of current sub-row start

	for subStart < len(lineBytes) && len(breaks) < maxBreaks {
		// Find byte column where this sub-row exceeds wrapWidth.
		// Measure from subStart to find the break point.
		breakCol := findBreakCol(lineBytes, m, wrapWidth, subStart)
		if breakCol <= 0 {
			// Can't break (e.g., single char wider than
			// wrapWidth). Force at least 1 char.
			breakCol = 1
		}
		nextStart := subStart + breakCol
		if nextStart >= len(lineBytes) {
			break // rest fits
		}
		breaks = append(breaks, nextStart)
		subStart = nextStart
	}
	return breaks
}

// findBreakCol finds where to break lineBytes starting at
// subStart. Returns a byte offset relative to subStart.
func findBreakCol(
	lineBytes []byte,
	m *text.Measurer,
	wrapWidth float32,
	subStart int,
) int {
	sub := lineBytes[subStart:]
	// Use XForColumn on the full line for accuracy (tabs depend
	// on absolute position).
	baseX := m.XForColumn(lineBytes, subStart)
	lastSpace := -1
	for col := 1; col <= len(sub); col++ {
		x := m.XForColumn(lineBytes, subStart+col) - baseX
		if x > wrapWidth {
			if lastSpace > 0 {
				return lastSpace
			}
			if col > 1 {
				return col - 1
			}
			return col
		}
		if col < len(sub) && isWordBreak(sub[col]) {
			lastSpace = col + 1
		}
	}
	return len(sub)
}

// isWordBreak reports whether b is a suitable word-break point.
func isWordBreak(b byte) bool {
	return b == ' ' || b == '\t'
}

// wrapMapTotalVisRows returns total visual rows covered by the
// wrap map entries.
func wrapMapTotalVisRows(wm *wrapMap) int {
	if wm == nil {
		return 0
	}
	total := 0
	for i := range wm.entries {
		total += wm.entries[i].subRows()
	}
	return total
}

// totalVisualRowsForBuffer returns the total visual rows for the
// entire buffer with wrapping. This is expensive for large buffers;
// used for scroll clamping.
func totalVisualRowsForBuffer(
	buf *buffer.Buffer,
	m *text.Measurer,
	wrapWidth float32,
	folds []FoldRange,
) int {
	if buf == nil {
		return 0
	}
	if m == nil || wrapWidth <= 0 || wrapWidth != wrapWidth {
		return buf.LineCount()
	}
	total := 0
	for line := 0; line < buf.LineCount(); {
		if len(folds) > 0 && isFolded(folds, line) {
			line = nextVisible(folds, line)
			continue
		}
		total += wrapLineVisualRowCount(buf.Line(line), m, wrapWidth)
		line++
		if len(folds) > 0 {
			line = nextVisible(folds, line)
		}
	}
	return total
}

// wrapVisualRowToLogical converts a visual row (counting wrapped
// sub-rows) to (logical line, sub-row index within that line).
// Uses the wrap map for the visible range.
func wrapVisualRowToLogical(
	wm *wrapMap, visRow int,
) (line, subRow int) {
	if wm == nil {
		return visRow, 0
	}
	vr := 0
	for i := range wm.entries {
		sr := wm.entries[i].subRows()
		if vr+sr > visRow {
			return wm.entries[i].Line, visRow - vr
		}
		vr += sr
	}
	// Past end of map.
	if len(wm.entries) > 0 {
		last := wm.entries[len(wm.entries)-1]
		return last.Line + 1, 0
	}
	return visRow, 0
}

// wrapLogicalToVisualRow converts a logical line to the first
// visual row for that line.
func wrapLogicalToVisualRow(wm *wrapMap, line int) int {
	if wm == nil {
		return line
	}
	vr := 0
	for i := range wm.entries {
		if wm.entries[i].Line == line {
			return vr
		}
		if wm.entries[i].Line > line {
			break
		}
		vr += wm.entries[i].subRows()
	}
	return vr
}

// wrapSubRowRange returns the [startCol, endCol) byte range for
// sub-row sr of a line entry. sr=0 is the first sub-row.
func wrapSubRowRange(we *wrapEntry, lineLen int, sr int) (int, int) {
	if we == nil {
		return 0, max(lineLen, 0)
	}
	return subRowByteRange(we.BreakCols, sr, lineLen)
}

// wrapEntryForLine returns the wrap entry for a logical line,
// or nil if not in the map.
func wrapEntryForLine(wm *wrapMap, line int) *wrapEntry {
	if wm == nil {
		return nil
	}
	for i := range wm.entries {
		if wm.entries[i].Line == line {
			return &wm.entries[i]
		}
		if wm.entries[i].Line > line {
			break
		}
	}
	return nil
}

// resolveBoolOverride applies a runtime override (0=use cfg,
// 1=force on, 2=force off) to a config bool.
func resolveBoolOverride(cfg bool, override int) bool {
	switch override {
	case 1:
		return true
	case 2:
		return false
	default:
		return cfg
	}
}

// wrapCursorVisualRow returns the visual row offset (sub-row)
// of a cursor within a wrapped line.
func wrapCursorVisualRow(
	we *wrapEntry, byteCol int,
) int {
	if we == nil {
		return 0
	}
	for i, bc := range we.BreakCols {
		if byteCol < bc {
			return i
		}
	}
	return len(we.BreakCols)
}

// wrapLineHitTest converts an x position within a wrapped sub-row
// to a byte column in the full line.
func wrapLineHitTest(
	we *wrapEntry,
	lineBytes []byte,
	subRow int,
	x float32,
	m *text.Measurer,
) int {
	if we == nil || m == nil || x != x { // NaN
		return 0
	}
	startCol, endCol := wrapSubRowRange(we, len(lineBytes), subRow)
	// x is relative to textX. Need to offset by the sub-row start.
	startX := m.XForColumn(lineBytes, startCol)
	col := m.ColumnForX(lineBytes, x+startX)
	return max(min(col, endCol), startCol)
}

// wrapLineVisualRowCount counts how many visual rows a line
// occupies at the given wrap width (0 = no wrap → 1 row).
func wrapLineVisualRowCount(
	lineBytes []byte,
	m *text.Measurer,
	wrapWidth float32,
) int {
	if m == nil || wrapWidth <= 0 || len(lineBytes) == 0 {
		return 1
	}
	totalW := m.XForColumn(lineBytes, len(lineBytes))
	if totalW <= wrapWidth {
		return 1
	}
	breaks := computeBreaks(lineBytes, m, wrapWidth)
	return len(breaks) + 1
}

// globalLogicalToVisualRow converts a logical line + sub-row
// to a global visual row index, accounting for folds and wraps.
// This is O(n) in the number of visible lines up to `line`.
func globalLogicalToVisualRow(
	buf *buffer.Buffer,
	m *text.Measurer,
	wrapWidth float32,
	folds []FoldRange,
	line int,
) int {
	if buf == nil || m == nil || line < 0 {
		return 0
	}
	vr := 0
	for l := 0; l < line && l < buf.LineCount(); {
		if len(folds) > 0 && isFolded(folds, l) {
			l = nextVisible(folds, l)
			continue
		}
		lb := buf.Line(l)
		vr += wrapLineVisualRowCount(lb, m, wrapWidth)
		l++
		if len(folds) > 0 {
			l = nextVisible(folds, l)
		}
	}
	return vr
}

// globalVisualRowToLogical converts a global visual row to a
// logical line. O(n) in visible lines.
func globalVisualRowToLogical(
	buf *buffer.Buffer,
	m *text.Measurer,
	wrapWidth float32,
	folds []FoldRange,
	visRow int,
) (line, subRow int) {
	if buf == nil || m == nil || visRow < 0 {
		return 0, 0
	}
	vr := 0
	for l := 0; l < buf.LineCount(); {
		if len(folds) > 0 && isFolded(folds, l) {
			l = nextVisible(folds, l)
			continue
		}
		lb := buf.Line(l)
		sr := wrapLineVisualRowCount(lb, m, wrapWidth)
		if vr+sr > visRow {
			return l, visRow - vr
		}
		vr += sr
		l++
		if len(folds) > 0 {
			l = nextVisible(folds, l)
		}
	}
	return buf.LineCount() - 1, 0
}

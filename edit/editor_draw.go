package edit

import (
	"slices"
	"strconv"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// selInfo caches a cursor's selection range for the draw loop.
type selInfo struct {
	sel    buffer.Range
	hasSel bool
}

// bracketMatchColor highlights the matching bracket pair.
var bracketMatchColor = gui.RGBA(255, 255, 0, 40)

// selectionBgColor is the background fill for selected text.
var selectionBgColor = gui.RGBA(51, 144, 255, 96)

// matchBgColor highlights all search matches.
var matchBgColor = gui.RGBA(255, 200, 0, 60)

// currentMatchBgColor highlights the current/active search match.
var currentMatchBgColor = gui.RGBA(255, 150, 0, 120)

// findBarBgColor is the find bar background.
var findBarBgColor = gui.RGBA(40, 40, 40, 230)

// findBarBorderColor is the find bar border.
var findBarBorderColor = gui.RGBA(80, 80, 80, 255)

// editorOnDraw returns a DrawCanvas OnDraw closure. The closure reads
// per-frame data from frame (populated by AmendLayout) and renders
// only the visible line range to dc.
func editorOnDraw(cfg EditorCfg, frame *editorFrameData) func(*gui.DrawContext) {
	return func(dc *gui.DrawContext) {
		if !frame.valid {
			return
		}
		st := frame.state
		lh := frame.lineHeight
		if lh <= 0 {
			return
		}
		theme := gui.CurrentTheme()
		monoStyle := theme.M3
		gutterStyle := monoStyle
		gutterStyle.Color = theme.ColorBorder

		buf := cfg.Buffer
		total := buf.LineCount()
		folds := st.FoldedRanges
		hasFolds := cfg.EnableFolding && len(folds) > 0
		wrapOn := frame.wrapActive

		// Use cached total visual rows from AmendLayout.
		visTot := frame.totalVisRows
		if visTot <= 0 {
			visTot = total
		}

		// Visible row range in visual-row space.
		firstVis := max(int(st.ScrollY/lh), 0)
		lastVis := int((st.ScrollY + dc.Height) / lh)
		if lastVis >= visTot {
			lastVis = visTot - 1
		}

		textX := frame.gutterW + frame.padLeft

		firstLogical, lastLogical := visRangeToLogical(
			buf, st.Measurer, frame, folds,
			hasFolds, wrapOn, firstVis, lastVis)
		decos := collectDecos(cfg, firstLogical, lastLogical)
		sels := buildSelInfos(st.Cursors)

		foldStyle := gutterStyle

		startLine, startSubRow := visRowToStartLine(
			buf, st.Measurer, frame, folds,
			hasFolds, wrapOn, firstVis)
		wsMode := resolveWhitespace(
			cfg.ShowWhitespace, st.WhitespaceOverride)
		visRow := firstVis
		i := startLine
		curSubRow := startSubRow

		for visRow <= lastVis && i < total {
			lineBytes := buf.Line(i)

			// Compute wrap breaks for this line.
			var breaks []int
			numSubRows := 1
			if wrapOn && st.Measurer != nil {
				breaks = computeBreaks(lineBytes,
					st.Measurer, frame.wrapWidth)
				numSubRows = len(breaks) + 1
			}

			// Draw sub-rows of this line.
			for sr := curSubRow; sr < numSubRows &&
				visRow <= lastVis; sr++ {
				y := float32(visRow)*lh - st.ScrollY
				subStart, subEnd := subRowByteRange(
					breaks, sr, len(lineBytes))

				if cfg.ShowLineNumbers && sr == 0 {
					drawGutter(dc, cfg, frame, folds,
						i, y, gutterStyle, foldStyle)
				}

				drawSearchHighlights(dc, &st, i,
					lineBytes, subStart, subEnd,
					textX, y, lh)
				drawSelections(dc, sels, i,
					lineBytes, subStart, subEnd,
					textX, y, lh, st.Measurer)
				drawBracketHighlights(dc, frame, i,
					subStart, subEnd, lineBytes,
					textX, y, lh, st.Measurer)
				drawLineText(dc, lineBytes, breaks,
					subStart, subEnd, i, textX, y,
					decos, monoStyle, st.Measurer)

				if sr == 0 && cfg.EnableFolding &&
					isFoldHeader(folds, i) {
					eolX := textX + st.Measurer.XForColumn(
						lineBytes, len(lineBytes))
					dc.Text(eolX+st.Measurer.Advance()/2,
						y, " ...", foldStyle)
				}

				if wsMode != WhitespaceNone {
					drawWhitespace(dc, lineBytes, i,
						textX, y, lh, st.Measurer,
						monoStyle, wsMode, sels)
				}

				visRow++
			}
			curSubRow = 0

			i++
			if hasFolds {
				i = nextVisible(folds, i)
			}
		}

		drawCursors(dc, cfg, frame, &st, buf, folds,
			hasFolds, wrapOn, textX, firstVis, lastVis,
			lh, monoStyle)

		// Gutter separator.
		if cfg.ShowLineNumbers {
			dc.Line(frame.gutterW, 0, frame.gutterW, dc.Height,
				theme.ColorBorder, 1)
		}

		// Sticky scroll overlay.
		if len(frame.stickyLines) > 0 {
			drawStickyScroll(dc, cfg, frame, &st,
				st.Measurer, lh, monoStyle, decos)
		}

		// Find bar overlay.
		if st.Search.Active {
			drawFindBar(dc, cfg, &st, st.Measurer, lh, monoStyle)
		}
	}
}

// visRangeToLogical maps first/last visual rows to logical line
// indices for decoration collection.
func visRangeToLogical(
	buf *buffer.Buffer, m *text.Measurer,
	frame *editorFrameData, folds []FoldRange,
	hasFolds, wrapOn bool,
	firstVis, lastVis int,
) (int, int) {
	if buf == nil {
		return firstVis, lastVis
	}
	if wrapOn && m != nil {
		f, _ := globalVisualRowToLogical(
			buf, m, frame.wrapWidth, folds, firstVis)
		l, _ := globalVisualRowToLogical(
			buf, m, frame.wrapWidth, folds, lastVis)
		return f, l
	}
	if hasFolds {
		return visibleToLogical(firstVis, folds),
			visibleToLogical(lastVis, folds)
	}
	return firstVis, lastVis
}

// collectDecos gathers decorations for the visible viewport.
func collectDecos(
	cfg EditorCfg, firstLine, lastLine int,
) []buffer.Decoration {
	if firstLine < 0 {
		firstLine = 0
	}
	if lastLine < firstLine {
		lastLine = firstLine
	}
	var decos []buffer.Decoration
	vp := buffer.Viewport{FirstLine: firstLine, LastLine: lastLine}
	for _, dp := range cfg.Decorations {
		decos = append(decos, dp.Decorate(vp)...)
	}
	slices.SortFunc(decos, decoCompare)
	return decos
}

// buildSelInfos precomputes selection ranges for all cursors.
func buildSelInfos(cursors []CursorState) []selInfo {
	var selBuf [4]selInfo
	var sels []selInfo
	if len(cursors) <= len(selBuf) {
		sels = selBuf[:len(cursors)]
	} else {
		sels = make([]selInfo, len(cursors))
	}
	for ci := range cursors {
		cs := &cursors[ci]
		if cs.HasSelection() {
			sels[ci] = selInfo{
				sel:    cs.SelectionRange(),
				hasSel: true,
			}
		}
	}
	return sels
}

// visRowToStartLine converts the first visible visual row to a
// logical line + sub-row.
func visRowToStartLine(
	buf *buffer.Buffer, m *text.Measurer,
	frame *editorFrameData, folds []FoldRange,
	hasFolds, wrapOn bool,
	firstVis int,
) (int, int) {
	if buf == nil {
		return firstVis, 0
	}
	if wrapOn && m != nil {
		return globalVisualRowToLogical(
			buf, m, frame.wrapWidth, folds, firstVis)
	}
	if hasFolds {
		return visibleToLogical(firstVis, folds), 0
	}
	return firstVis, 0
}

// subRowByteRange returns the [start, end) byte range for sub-row
// sr of a wrapped line. Returns [0, lineLen) if no breaks.
func subRowByteRange(breaks []int, sr, lineLen int) (int, int) {
	if len(breaks) == 0 {
		return 0, max(lineLen, 0)
	}
	if sr < 0 {
		sr = 0
	}
	start := 0
	if sr > 0 && sr <= len(breaks) {
		start = breaks[sr-1]
	}
	end := lineLen
	if sr < len(breaks) {
		end = breaks[sr]
	}
	return start, end
}

// drawGutter draws the line number or fold indicator for one line.
func drawGutter(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	folds []FoldRange,
	line int,
	y float32,
	gutterStyle, foldStyle gui.TextStyle,
) {
	if frame.state.Measurer == nil || line < 0 {
		return
	}
	if cfg.EnableFolding && isFoldHeader(folds, line) {
		dc.Text(frame.gutterW-frame.padLeft-
			frame.state.Measurer.Advance(),
			y, ">", foldStyle)
		return
	}
	num := strconv.Itoa(line + 1)
	nw := float32(len(num)) * frame.state.Measurer.Advance()
	dc.Text(frame.gutterW-nw-frame.padLeft, y, num, gutterStyle)
}

// drawSearchHighlights draws search match backgrounds for a
// sub-row of a line. subStart/subEnd are the byte range of the
// visible sub-row; highlights outside this range are skipped.
func drawSearchHighlights(
	dc *gui.DrawContext,
	st *editorState,
	line int,
	lineBytes []byte,
	subStart, subEnd int,
	textX, y, lh float32,
) {
	if !st.Search.Active || len(st.Search.Matches) == 0 {
		return
	}
	for _, mr := range matchesForLine(st.Search.Matches, line) {
		if !rangeOverlapsSubRow(mr, line, subStart, subEnd) {
			continue
		}
		drawSelectionBg(dc, mr, line, lineBytes, textX, y, lh,
			st.Measurer, matchBgColor)
	}
	idx := st.Search.CurrentMatch
	if idx >= 0 && idx < len(st.Search.Matches) {
		cm := st.Search.Matches[idx]
		if cm.Start.Line <= line && cm.End.Line >= line &&
			rangeOverlapsSubRow(cm, line, subStart, subEnd) {
			drawSelectionBg(dc, cm, line, lineBytes, textX, y,
				lh, st.Measurer, currentMatchBgColor)
		}
	}
}

// drawSelections draws selection backgrounds for all cursors,
// clipped to the visible sub-row [subStart, subEnd).
func drawSelections(
	dc *gui.DrawContext,
	sels []selInfo,
	line int,
	lineBytes []byte,
	subStart, subEnd int,
	textX, y, lh float32,
	m *text.Measurer,
) {
	for ci := range sels {
		if sels[ci].hasSel &&
			rangeOverlapsSubRow(sels[ci].sel, line,
				subStart, subEnd) {
			drawSelectionBg(dc, sels[ci].sel, line,
				lineBytes, textX, y, lh, m, selectionBgColor)
		}
	}
}

// rangeOverlapsSubRow reports whether a buffer range overlaps
// the byte range [subStart, subEnd) on the given line.
func rangeOverlapsSubRow(
	r buffer.Range, line, subStart, subEnd int,
) bool {
	if r.Start.Line > line || r.End.Line < line {
		return false
	}
	startCol := 0
	if r.Start.Line == line {
		startCol = r.Start.ByteCol
	}
	endCol := subEnd
	if r.End.Line == line {
		endCol = r.End.ByteCol
	}
	return startCol < subEnd && endCol > subStart
}

// drawBracketHighlights draws bracket match highlights for a line.
func drawBracketHighlights(
	dc *gui.DrawContext,
	frame *editorFrameData,
	line, subStart, subEnd int,
	lineBytes []byte,
	textX, y, lh float32,
	m *text.Measurer,
) {
	if !frame.bracketFound {
		return
	}
	for _, bp := range frame.bracketMatch {
		if bp.Line == line &&
			bp.ByteCol >= subStart && bp.ByteCol < subEnd {
			bx := textX + m.XForColumn(lineBytes, bp.ByteCol)
			dc.FilledRect(bx, y, m.Advance(), lh,
				bracketMatchColor)
		}
	}
}

// drawLineText renders line text, either as a full line (no wrap)
// or a wrapped sub-row.
func drawLineText(
	dc *gui.DrawContext,
	lineBytes []byte,
	breaks []int,
	subStart, subEnd, line int,
	textX, y float32,
	decos []buffer.Decoration,
	base gui.TextStyle,
	m *text.Measurer,
) {
	if m == nil {
		return
	}
	if len(breaks) == 0 {
		lineDecos := decosForLine(decos, line)
		if len(lineDecos) == 0 {
			if len(lineBytes) > 0 {
				dc.Text(textX, y,
					text.ExpandTabs(lineBytes, m.TabWidth),
					base)
			}
		} else {
			renderStyledLine(dc, textX, y, lineBytes,
				lineDecos, base, m)
		}
		return
	}
	subBytes := lineBytes[subStart:subEnd]
	if len(subBytes) > 0 {
		vcol := text.VisualCols(lineBytes, subStart, m.TabWidth)
		dc.Text(textX, y,
			text.ExpandTabsSpan(subBytes, vcol, m.TabWidth), base)
	}
}

// drawCursors draws all cursor carets.
func drawCursors(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	st *editorState,
	buf *buffer.Buffer,
	folds []FoldRange,
	hasFolds, wrapOn bool,
	textX float32,
	firstVis, lastVis int,
	lh float32,
	style gui.TextStyle,
) {
	for ci := range st.Cursors {
		cs := &st.Cursors[ci]
		if hasFolds && isFolded(folds, cs.Cursor.Line) {
			continue
		}
		lb := buf.Line(cs.Cursor.Line)
		var cVisRow int
		var curBreaks []int
		if wrapOn && st.Measurer != nil {
			cVisRow = globalLogicalToVisualRow(
				buf, st.Measurer, frame.wrapWidth,
				folds, cs.Cursor.Line)
			curBreaks = computeBreaks(lb,
				st.Measurer, frame.wrapWidth)
			we := wrapEntry{BreakCols: curBreaks}
			cVisRow += wrapCursorVisualRow(&we,
				cs.Cursor.ByteCol)
		} else if hasFolds {
			cVisRow = logicalToVisible(cs.Cursor.Line, folds)
		} else {
			cVisRow = cs.Cursor.Line
		}
		if cVisRow < firstVis || cVisRow > lastVis {
			continue
		}
		cy := float32(cVisRow)*lh - st.ScrollY
		cx := textX + st.Measurer.XForColumn(lb,
			cs.Cursor.ByteCol)
		if len(curBreaks) > 0 {
			we := wrapEntry{BreakCols: curBreaks}
			sr := wrapCursorVisualRow(&we, cs.Cursor.ByteCol)
			subStart, _ := wrapSubRowRange(&we, len(lb), sr)
			cx = textX +
				st.Measurer.XForColumn(lb,
					cs.Cursor.ByteCol) -
				st.Measurer.XForColumn(lb, subStart)
		}
		dc.FilledRect(cx, cy, 1, lh, style.Color)
	}
}

// stickyBgColor is the background for the sticky scroll area.
var stickyBgColor = gui.RGBA(30, 30, 30, 240)

// stickyBorderColor is the bottom border of the sticky area.
var stickyBorderColor = gui.RGBA(60, 60, 60, 255)

// drawStickyScroll draws pinned scope headers at the top.
func drawStickyScroll(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	st *editorState,
	m *text.Measurer,
	lh float32,
	baseStyle gui.TextStyle,
	decos []buffer.Decoration,
) {
	if m == nil || lh <= 0 || len(frame.stickyLines) == 0 {
		return
	}
	textX := frame.gutterW + frame.padLeft
	stickyH := float32(len(frame.stickyLines)) * lh

	// Background.
	dc.FilledRect(0, 0, dc.Width, stickyH, stickyBgColor)
	dc.Line(0, stickyH, dc.Width, stickyH, stickyBorderColor, 1)

	gutterStyle := baseStyle
	gutterStyle.Color = gui.CurrentTheme().ColorBorder

	for si, line := range frame.stickyLines {
		y := float32(si) * lh
		lineBytes := cfg.Buffer.Line(line)

		// Gutter number.
		if cfg.ShowLineNumbers {
			num := strconv.Itoa(line + 1)
			nw := float32(len(num)) * m.Advance()
			dc.Text(frame.gutterW-nw-frame.padLeft,
				y, num, gutterStyle)
		}

		// Line text with syntax highlighting.
		lineDecos := decosForLine(decos, line)
		if len(lineDecos) == 0 {
			if len(lineBytes) > 0 {
				dc.Text(textX, y,
					text.ExpandTabs(lineBytes, m.TabWidth),
					baseStyle)
			}
		} else {
			renderStyledLine(dc, textX, y, lineBytes,
				lineDecos, baseStyle, m)
		}
	}
}

// decoCompare sorts decorations by line, then start col, then
// descending priority.
func decoCompare(a, b buffer.Decoration) int {
	if a.Range.Start.Line != b.Range.Start.Line {
		return a.Range.Start.Line - b.Range.Start.Line
	}
	if a.Range.Start.ByteCol != b.Range.Start.ByteCol {
		return a.Range.Start.ByteCol - b.Range.Start.ByteCol
	}
	return b.Priority - a.Priority // higher priority first
}

// decosForLine returns the subset of sorted decos that touch
// line i. Since decos is sorted by start line, this is a scan
// that stops early.
func decosForLine(decos []buffer.Decoration, line int) []buffer.Decoration {
	if line < 0 {
		return nil
	}
	var out []buffer.Decoration
	for j := range decos {
		d := &decos[j]
		if d.Kind != buffer.DecoToken {
			continue
		}
		if d.Range.Start.Line > line {
			break
		}
		if d.Range.End.Line < line {
			continue
		}
		out = append(out, *d)
	}
	return out
}

// renderStyledLine draws a line split into styled spans per the
// token decorations. Decorations must be DecoToken and sorted by
// start col.
func renderStyledLine(
	dc *gui.DrawContext,
	x, y float32,
	lineBytes []byte,
	decos []buffer.Decoration,
	base gui.TextStyle,
	m *text.Measurer,
) {
	if m == nil || len(lineBytes) == 0 {
		return
	}
	originX := x
	col := 0 // current byte offset
	tw := m.TabWidth

	for _, d := range decos {
		startCol := d.Range.Start.ByteCol
		endCol := min(d.Range.End.ByteCol, len(lineBytes))
		startCol = max(startCol, col)
		if startCol >= endCol {
			continue
		}

		// Emit unstyled gap before this token.
		if col < startCol {
			vcol := text.VisualCols(lineBytes, col, tw)
			gap := text.ExpandTabsSpan(
				lineBytes[col:startCol], vcol, tw)
			dc.Text(x, y, gap, base)
			x = originX + m.XForColumn(lineBytes, startCol)
		}

		// Emit styled span.
		vcol := text.VisualCols(lineBytes, startCol, tw)
		span := text.ExpandTabsSpan(
			lineBytes[startCol:endCol], vcol, tw)
		style := base
		if d.FgColor != 0 {
			style.Color = decoColorToGUI(d.FgColor)
		}
		dc.Text(x, y, span, style)
		col = endCol
		x = originX + m.XForColumn(lineBytes, col)
	}

	// Emit trailing unstyled text.
	if col < len(lineBytes) {
		vcol := text.VisualCols(lineBytes, col, tw)
		dc.Text(x, y, text.ExpandTabsSpan(
			lineBytes[col:], vcol, tw), base)
	}
}

// decoColorToGUI converts 0xRRGGBBAA to gui.Color.
func decoColorToGUI(c uint32) gui.Color {
	return gui.RGBA(
		uint8((c>>24)&0xFF),
		uint8((c>>16)&0xFF),
		uint8((c>>8)&0xFF),
		uint8(c&0xFF),
	)
}

// drawSelectionBg draws the selection background for a single line.
func drawSelectionBg(
	dc *gui.DrawContext,
	sel buffer.Range,
	lineIdx int,
	lineBytes []byte,
	textX, y, lh float32,
	m *text.Measurer,
	color gui.Color,
) {
	if m == nil {
		return
	}
	// Check if this line is inside the selection.
	if lineIdx < sel.Start.Line || lineIdx > sel.End.Line {
		return
	}
	lineLen := len(lineBytes)

	startCol := 0
	if lineIdx == sel.Start.Line {
		startCol = sel.Start.ByteCol
	}
	endCol := lineLen
	if lineIdx == sel.End.Line {
		endCol = sel.End.ByteCol
	}

	if startCol > lineLen {
		startCol = lineLen
	}
	if endCol > lineLen {
		endCol = lineLen
	}
	if startCol >= endCol && lineIdx == sel.End.Line {
		return
	}

	sx := textX + m.XForColumn(lineBytes, startCol)
	var ex float32
	if lineIdx < sel.End.Line {
		// Line continues into next; extend one advance past EOL.
		ex = textX + m.XForColumn(lineBytes, lineLen) + m.Advance()
	} else {
		ex = textX + m.XForColumn(lineBytes, endCol)
	}
	if ex > sx {
		dc.FilledRect(sx, y, ex-sx, lh, color)
	}
}

// drawFindBar renders the find/replace bar at the top-right of the
// viewport.
func drawFindBar(
	dc *gui.DrawContext,
	cfg EditorCfg,
	st *editorState,
	m *text.Measurer,
	lh float32,
	baseStyle gui.TextStyle,
) {
	if m == nil {
		return
	}
	adv := m.Advance()
	if adv <= 0 || lh <= 0 {
		return
	}
	ss := &st.Search
	rowH := lh * 1.5
	pad := adv / 2

	// Bar dimensions.
	rows := 1
	if ss.ShowReplace {
		rows = 2
	}
	barH := rowH*float32(rows) + pad*2
	barW := min(dc.Width*0.5, float32(400))
	if barW < 200 {
		barW = min(float32(200), dc.Width)
	}
	edgePad := adv * 4
	barX := dc.Width - barW - edgePad
	if barW+edgePad > dc.Width {
		barW = dc.Width - edgePad
		if barW < edgePad {
			barW = dc.Width
			edgePad = 0
		}
		barX = dc.Width - barW - edgePad
	}
	if barX < 0 {
		barX = 0
	}
	barY := pad

	// Background + border.
	dc.FilledRect(barX, barY, barW, barH, findBarBgColor)
	dc.Rect(barX, barY, barW, barH, findBarBorderColor, 1)

	// Styles.
	dimStyle := baseStyle
	dimStyle.Color = gui.RGBA(120, 120, 120, 255)

	inputX := barX + pad
	inputY := barY + pad
	inputW := barW - pad*2

	// Toggle indicators.
	toggles := ""
	if ss.IsRegex {
		toggles += "[.*] "
	} else {
		toggles += " .*  "
	}
	if ss.CaseSensitive {
		toggles += "[Aa] "
	} else {
		toggles += " Aa  "
	}
	if ss.InSelection {
		toggles += "[Sel]"
	} else {
		toggles += " Sel "
	}
	toggleW := float32(len(toggles)) * adv
	fieldW := inputW - toggleW - adv
	if fieldW < adv {
		fieldW = adv
	}

	// Draw query field.
	drawSearchField(dc, ss.Query, ss.FieldCursor,
		!ss.FocusReplace, inputX, inputY, fieldW, rowH,
		baseStyle, m)

	// Toggle text.
	toggleX := inputX + fieldW + adv
	toggleY := inputY + (rowH-lh)/2
	dc.Text(toggleX, toggleY, toggles, dimStyle)

	// Match count.
	countStr := matchCountStr(ss)
	countW := float32(len(countStr)) * adv
	countX := inputX + fieldW - countW - pad
	dc.Text(countX, toggleY, countStr, dimStyle)

	// Replace row.
	if ss.ShowReplace {
		replY := inputY + rowH
		drawSearchField(dc, ss.ReplaceText, ss.FieldCursor,
			ss.FocusReplace, inputX, replY, fieldW, rowH,
			baseStyle, m)
	}
}

// drawSearchField renders one input field of the find bar.
func drawSearchField(
	dc *gui.DrawContext,
	fieldText string,
	cursorPos int,
	focused bool,
	x, y, w, h float32,
	style gui.TextStyle,
	m *text.Measurer,
) {
	if m == nil || w <= 0 || h <= 0 {
		return
	}
	adv := m.Advance()
	if adv <= 0 {
		return
	}
	lh := m.LineHeight()
	pad := adv / 2

	// Field background.
	fieldBg := gui.RGBA(30, 30, 30, 255)
	if focused {
		fieldBg = gui.RGBA(50, 50, 50, 255)
	}
	dc.FilledRect(x, y+2, w, h-4, fieldBg)

	// Text.
	textY := y + (h-lh)/2
	textX := x + pad
	if len(fieldText) > 0 {
		maxChars := int((w - pad*2) / adv)
		display := fieldText
		if maxChars > 0 && len(display) > maxChars {
			display = display[len(display)-maxChars:]
		}
		dc.Text(textX, textY, display, style)
	}

	// Cursor.
	if focused {
		displayText := fieldText
		cursorByte := cursorPos
		maxChars := int((w - pad*2) / adv)
		if maxChars > 0 && len(displayText) > maxChars {
			offset := len(displayText) - maxChars
			displayText = displayText[offset:]
			cursorByte -= offset
			if cursorByte < 0 {
				cursorByte = 0
			}
		}
		cx := textX + m.XForColumn([]byte(displayText), cursorByte)
		dc.FilledRect(cx, y+4, 1, h-8, style.Color)
	}
}

// matchCountStr returns a display string like "3 of 42".
func matchCountStr(ss *searchState) string {
	if len(ss.Query) == 0 {
		return ""
	}
	if len(ss.Matches) == 0 {
		return "No results"
	}
	cur := max(ss.CurrentMatch+1, 1)
	total := len(ss.Matches)
	suffix := ""
	if total >= maxMatches {
		suffix = "+"
	}
	return strconv.Itoa(cur) + " of " + strconv.Itoa(total) + suffix
}

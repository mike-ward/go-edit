package edit

import (
	"slices"
	"strconv"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

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

		// Visible line range.
		first := max(int(st.ScrollY/lh), 0)
		last := int((st.ScrollY + dc.Height) / lh)
		if last >= total {
			last = total - 1
		}

		textX := frame.gutterW + frame.padLeft

		// Collect decorations for visible range.
		var decos []buffer.Decoration
		vp := buffer.Viewport{FirstLine: first, LastLine: last}
		for _, dp := range cfg.Decorations {
			decos = append(decos, dp.Decorate(vp)...)
		}
		slices.SortFunc(decos, decoCompare)

		// Precompute selection ranges for all cursors.
		// Stack alloc for the common single-cursor case.
		type selInfo struct {
			sel    buffer.Range
			hasSel bool
		}
		var selBuf [4]selInfo
		var sels []selInfo
		if len(st.Cursors) <= len(selBuf) {
			sels = selBuf[:len(st.Cursors)]
		} else {
			sels = make([]selInfo, len(st.Cursors))
		}
		for ci := range st.Cursors {
			cs := &st.Cursors[ci]
			if cs.HasSelection() {
				sels[ci] = selInfo{
					sel:    cs.SelectionRange(),
					hasSel: true,
				}
			}
		}

		for i := first; i <= last; i++ {
			y := float32(i)*lh - st.ScrollY

			if cfg.ShowLineNumbers {
				num := strconv.Itoa(i + 1)
				nw := float32(len(num)) * st.Measurer.Advance()
				dc.Text(frame.gutterW-nw-frame.padLeft, y,
					num, gutterStyle)
			}

			lineBytes := buf.Line(i)

			// Draw search match highlights (below selection).
			if st.Search.Active && len(st.Search.Matches) > 0 {
				for _, mr := range matchesForLine(st.Search.Matches, i) {
					drawSelectionBg(dc, mr, i,
						lineBytes, textX, y, lh,
						st.Measurer, matchBgColor)
				}
				// Current match in brighter color.
				idx := st.Search.CurrentMatch
				if idx >= 0 && idx < len(st.Search.Matches) {
					cm := st.Search.Matches[idx]
					if cm.Start.Line <= i && cm.End.Line >= i {
						drawSelectionBg(dc, cm, i,
							lineBytes, textX, y, lh,
							st.Measurer, currentMatchBgColor)
					}
				}
			}

			// Draw selection backgrounds for all cursors.
			for ci := range sels {
				if sels[ci].hasSel {
					drawSelectionBg(dc, sels[ci].sel, i,
						lineBytes, textX, y, lh,
						st.Measurer, selectionBgColor)
				}
			}

			lineDecos := decosForLine(decos, i)
			if len(lineDecos) == 0 {
				if len(lineBytes) > 0 {
					dc.Text(textX, y,
						text.ExpandTabs(lineBytes, st.Measurer.TabWidth),
						monoStyle)
				}
			} else {
				renderStyledLine(dc, textX, y, lineBytes,
					lineDecos, monoStyle, st.Measurer)
			}
		}

		// Draw all cursors.
		for ci := range st.Cursors {
			cs := &st.Cursors[ci]
			if cs.Cursor.Line >= first && cs.Cursor.Line <= last {
				cy := float32(cs.Cursor.Line)*lh - st.ScrollY
				cx := textX + st.Measurer.XForColumn(
					buf.Line(cs.Cursor.Line), cs.Cursor.ByteCol)
				dc.FilledRect(cx, cy, 1, lh, monoStyle.Color)
			}
		}

		// Gutter separator.
		if cfg.ShowLineNumbers {
			dc.Line(frame.gutterW, 0, frame.gutterW, dc.Height,
				theme.ColorBorder, 1)
		}

		// Find bar overlay.
		if st.Search.Active {
			drawFindBar(dc, cfg, &st, st.Measurer, lh, monoStyle)
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

package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// clampScroll keeps ScrollY within [0, maxScroll]. Also sanitizes
// NaN — if ScrollY went NaN from bad input upstream, snap to 0.
func clampScroll(st *editorState, cfg EditorCfg, frame *editorFrameData, lh float32) {
	if st.ScrollY != st.ScrollY { // NaN
		st.ScrollY = 0
	}
	if lh <= 0 {
		st.ScrollY = 0
		return
	}
	total := frame.totalVisRows
	if total <= 0 {
		total = cfg.Buffer.LineCount()
	}
	maxScroll := float32(total)*lh - cfg.Height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if st.ScrollY > maxScroll {
		st.ScrollY = maxScroll
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}
}

// clampScrollX keeps ScrollX in [0, maxScrollX]. Sanitizes NaN.
func clampScrollX(st *editorState, maxScrollX float32) {
	if st.ScrollX != st.ScrollX { // NaN
		st.ScrollX = 0
	}
	if st.ScrollX < 0 {
		st.ScrollX = 0
	}
	if st.ScrollX > maxScrollX {
		st.ScrollX = maxScrollX
	}
}

func ensureCursorVisible(st *editorState, frame *editorFrameData, cfg EditorCfg) {
	viewportH := cfg.Height
	if !frame.valid || frame.lineHeight <= 0 {
		return
	}
	if viewportH != viewportH || viewportH <= 0 { // NaN or non-positive
		return
	}
	if len(st.Cursors) == 0 {
		return
	}
	p := st.primary()
	lh := frame.lineHeight
	visRow := p.Cursor.Line
	if frame.wrapActive && st.Measurer != nil {
		visRow = globalLogicalToVisualRow(
			cfg.Buffer, st.Measurer,
			frame.wrapWidth, st.FoldedRanges, p.Cursor.Line)
		// Add sub-row offset for cursor within wrapped line.
		we := &wrapEntry{Line: p.Cursor.Line}
		lb := cfg.Buffer.Line(p.Cursor.Line)
		we.BreakCols = computeBreaks(lb, st.Measurer,
			frame.wrapWidth)
		visRow += wrapCursorVisualRow(we, p.Cursor.ByteCol)
	} else if len(st.FoldedRanges) > 0 {
		visRow = logicalToVisible(p.Cursor.Line, st.FoldedRanges)
	}
	cy := float32(visRow) * lh
	if cy < st.ScrollY {
		st.ScrollY = cy
	}
	if cy+lh > st.ScrollY+viewportH {
		st.ScrollY = cy + lh - viewportH
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}

	// Horizontal visibility (no-wrap only).
	if !frame.wrapActive && st.Measurer != nil {
		lb := cfg.Buffer.Line(p.Cursor.Line)
		cursorX := st.Measurer.XForColumn(lb, p.Cursor.ByteCol)
		textAreaW := cfg.Width - frame.gutterW - frame.padLeft
		if textAreaW > 0 {
			if cursorX < st.ScrollX {
				st.ScrollX = cursorX
			}
			if cursorX > st.ScrollX+textAreaW {
				st.ScrollX = cursorX - textAreaW
			}
		}
		maxScrollX := max(frame.maxContentW-textAreaW, 0)
		clampScrollX(st, maxScrollX)
	}
}

// clampCursors clamps all cursors to valid coordinates within buf.
// Called from AmendLayout to recover gracefully from external
// buffer mutations.
func clampCursors(st *editorState, buf *buffer.Buffer) {
	for i := range st.Cursors {
		clampPos(&st.Cursors[i].Cursor, buf)
		clampPos(&st.Cursors[i].Anchor, buf)
	}
}

func clampPos(p *buffer.Position, buf *buffer.Buffer) {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= buf.LineCount() {
		p.Line = buf.LineCount() - 1
	}
	ll := len(buf.Line(p.Line))
	if p.ByteCol < 0 {
		p.ByteCol = 0
	}
	if p.ByteCol > ll {
		p.ByteCol = ll
	}
}

// pageLines computes the number of logical lines per page.
func pageLines(frame *editorFrameData, viewportH float32) int {
	if frame.lineHeight <= 0 {
		return 1
	}
	n := int(viewportH / frame.lineHeight)
	n = max(n, 1)
	return n
}

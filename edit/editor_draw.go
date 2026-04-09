package edit

import (
	"slices"
	"strconv"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

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

		for i := first; i <= last; i++ {
			y := float32(i)*lh - st.ScrollY

			if cfg.ShowLineNumbers {
				num := strconv.Itoa(i + 1)
				nw := float32(len(num)) * st.Measurer.Advance()
				dc.Text(frame.gutterW-nw-frame.padLeft, y,
					num, gutterStyle)
			}

			lineBytes := buf.Line(i)
			lineDecos := decosForLine(decos, i)
			if len(lineDecos) == 0 {
				if len(lineBytes) > 0 {
					dc.Text(textX, y, string(lineBytes),
						monoStyle)
				}
			} else {
				renderStyledLine(dc, textX, y, lineBytes,
					lineDecos, monoStyle, st.Measurer)
			}
		}

		// Cursor.
		if st.Cursor.Line >= first && st.Cursor.Line <= last {
			cy := float32(st.Cursor.Line)*lh - st.ScrollY
			cx := textX + st.Measurer.XForColumn(
				buf.Line(st.Cursor.Line), st.Cursor.ByteCol)
			dc.FilledRect(cx, cy, 1, lh, monoStyle.Color)
		}

		// Gutter separator.
		if cfg.ShowLineNumbers {
			dc.Line(frame.gutterW, 0, frame.gutterW, dc.Height,
				theme.ColorBorder, 1)
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
	advance := m.Advance()
	col := 0 // current byte offset

	for _, d := range decos {
		startCol := d.Range.Start.ByteCol
		endCol := min(d.Range.End.ByteCol, len(lineBytes))
		startCol = max(startCol, col)
		if startCol >= endCol {
			continue
		}

		// Emit unstyled gap before this token.
		if col < startCol {
			gap := string(lineBytes[col:startCol])
			dc.Text(x, y, gap, base)
			x += float32(startCol-col) * advance
		}

		// Emit styled span.
		span := string(lineBytes[startCol:endCol])
		style := base
		if d.FgColor != 0 {
			style.Color = decoColorToGUI(d.FgColor)
		}
		dc.Text(x, y, span, style)
		x += float32(endCol-startCol) * advance
		col = endCol
	}

	// Emit trailing unstyled text.
	if col < len(lineBytes) {
		dc.Text(x, y, string(lineBytes[col:]), base)
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

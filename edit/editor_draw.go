package edit

import (
	"strconv"

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

		for i := first; i <= last; i++ {
			y := float32(i)*lh - st.ScrollY

			if cfg.ShowLineNumbers {
				num := strconv.Itoa(i + 1)
				nw := float32(len(num)) * st.Measurer.Advance()
				dc.Text(frame.gutterW-nw-frame.padLeft, y,
					num, gutterStyle)
			}

			lineBytes := buf.Line(i)
			if len(lineBytes) > 0 {
				// TODO(perf): string(lineBytes) allocates per
				// visible line per frame. Consider a line-keyed
				// cache or a dc.TextBytes variant upstream.
				dc.Text(textX, y, string(lineBytes), monoStyle)
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

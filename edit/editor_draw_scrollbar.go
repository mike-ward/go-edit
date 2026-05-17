package edit

import "github.com/mike-ward/go-gui/gui"

// drawScrollbar draws vertical and horizontal scrollbar overlays.
// Both are semi-transparent overlays; no layout metrics are affected.
// When both are visible, each track is shortened by scrollbarWidth
// at the corner to avoid overlap.
func drawScrollbar(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	rt *resolvedTheme,
) {
	lh := frame.lineHeight
	textAreaW := cfg.Width - frame.gutterW - frame.padLeft

	vertVisible := scrollbarVisible(cfg.Scrollbar,
		frame.totalVisRows, lh, cfg.Height)
	horizVisible := scrollbarHorizVisible(cfg.Scrollbar,
		frame.wrapActive, frame.maxContentW, textAreaW)

	if vertVisible {
		drawVertScrollbar(dc, cfg, frame, rt, horizVisible)
	}
	if horizVisible {
		drawHorizScrollbar(dc, cfg, frame, rt, vertVisible)
	}
}

// drawVertScrollbar draws the vertical bar on the right edge.
func drawVertScrollbar(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	rt *resolvedTheme,
	horizVisible bool,
) {
	trackX := dc.Width - scrollbarWidth
	trackH := dc.Height
	if horizVisible {
		trackH -= scrollbarWidth
	}
	if trackH <= 0 {
		return
	}

	dc.FilledRect(trackX, 0, scrollbarWidth, trackH, rt.scrollbarTrack)

	thumbY, thumbH, hasThumb := scrollbarGeometry(
		frame.totalVisRows, frame.lineHeight,
		cfg.Height, frame.state.ScrollY, trackH)
	if hasThumb {
		dc.FilledRoundedRect(trackX+1, thumbY+1,
			scrollbarWidth-2, thumbH-2, 3, rt.scrollbarThumb)
	}
}

// drawHorizScrollbar draws the horizontal bar on the bottom edge.
// The track starts at the gutter right edge so the thumb position
// corresponds only to the scrollable text area.
func drawHorizScrollbar(
	dc *gui.DrawContext,
	cfg EditorCfg,
	frame *editorFrameData,
	rt *resolvedTheme,
	vertVisible bool,
) {
	trackY := dc.Height - scrollbarWidth
	trackX := frame.gutterW
	trackW := dc.Width - trackX
	if vertVisible {
		trackW -= scrollbarWidth
	}
	if trackW <= 0 {
		return
	}

	dc.FilledRect(trackX, trackY, trackW, scrollbarWidth, rt.scrollbarTrack)

	textAreaW := cfg.Width - frame.gutterW - frame.padLeft
	contentW := frame.maxContentW + cursorScrollPad
	// Reuse scrollbarGeometry: map content→track as if each pixel
	// is one "row" and the viewport is textAreaW "rows".
	thumbX, thumbW, hasThumb := scrollbarGeometry(
		int(contentW), 1, textAreaW,
		frame.state.ScrollX, trackW)
	if hasThumb {
		dc.FilledRoundedRect(
			trackX+thumbX+1, trackY+1,
			thumbW-2, scrollbarWidth-2, 3, rt.scrollbarThumb)
	}
}

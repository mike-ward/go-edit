// Package text wraps gui.TextMeasurer for the editor: cached monospace
// metrics, ASCII fast-path column↔x conversion, and fallback to
// go-glyph's Layout for non-ASCII lines.
//
// This is the single choke point between go-edit and the text-shaping
// stack. The editor never imports go-glyph directly except through this
// package. Swap underlying shapers by changing this file.
package text

import (
	"unicode/utf8"

	"github.com/mike-ward/go-glyph"
	"github.com/mike-ward/go-gui/gui"
)

// Measurer caches monospace metrics for a single TextStyle and exposes
// byte-column ↔ pixel-x conversions plus line height.
type Measurer struct {
	tm         gui.TextMeasurer
	style      gui.TextStyle
	advance    float32
	lineHeight float32
}

// New builds a Measurer for the given window and style. It measures
// "M" once to cache the monospace advance. Returns nil if the window
// has no TextMeasurer (e.g. headless tests without a backend); callers
// must guard.
func New(w *gui.Window, style gui.TextStyle) *Measurer {
	if w == nil {
		return nil
	}
	tm := w.TextMeasurer()
	if tm == nil {
		return nil
	}
	return &Measurer{
		tm:         tm,
		style:      style,
		advance:    tm.TextWidth("M", style),
		lineHeight: tm.FontHeight(style),
	}
}

// Advance returns the cached width of a single monospace glyph.
func (m *Measurer) Advance() float32 { return m.advance }

// LineHeight returns the cached line height.
func (m *Measurer) LineHeight() float32 { return m.lineHeight }

// Style returns the text style this measurer was built with.
func (m *Measurer) Style() gui.TextStyle { return m.style }

// XForColumn returns the x-offset of the cursor at byteCol within
// lineBytes. ASCII-only lines use the monospace fast path; any
// non-ASCII byte falls back to go-glyph layout.
func (m *Measurer) XForColumn(lineBytes []byte, byteCol int) float32 {
	if m == nil || byteCol <= 0 {
		return 0
	}
	if byteCol > len(lineBytes) {
		byteCol = len(lineBytes)
	}
	if isASCII(lineBytes[:byteCol]) {
		return float32(byteCol) * m.advance
	}
	layout, err := m.tm.LayoutText(string(lineBytes), m.style, 0)
	if err != nil {
		return float32(byteCol) * m.advance
	}
	cp, ok := layout.GetCursorPos(byteCol)
	if !ok {
		return float32(byteCol) * m.advance
	}
	return cp.X
}

// ColumnForX returns the byte column closest to x within lineBytes.
// Returns the clamped column; never -1.
func (m *Measurer) ColumnForX(lineBytes []byte, x float32) int {
	if m == nil || x != x || x <= 0 { // x != x traps NaN
		return 0
	}
	if isASCII(lineBytes) {
		col := min(max(int((x+m.advance/2)/m.advance), 0), len(lineBytes))
		return col
	}
	layout, err := m.tm.LayoutText(string(lineBytes), m.style, 0)
	if err != nil {
		return clampASCIICol(lineBytes, x, m.advance)
	}
	idx := layout.HitTest(x, m.lineHeight/2)
	if idx < 0 {
		return len(lineBytes)
	}
	return idx
}

// LayoutLine returns the go-glyph layout for lineBytes. Exposed for
// callers that need direct access (selection rects, bidi, etc.).
// Allocates — avoid in hot paths where ASCII fast path suffices.
func (m *Measurer) LayoutLine(lineBytes []byte) (glyph.Layout, error) {
	return m.tm.LayoutText(string(lineBytes), m.style, 0)
}

func isASCII(p []byte) bool {
	for _, b := range p {
		if b >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func clampASCIICol(p []byte, x, advance float32) int {
	if advance <= 0 {
		return 0
	}
	col := int((x + advance/2) / advance)
	if col < 0 {
		return 0
	}
	if col > len(p) {
		return len(p)
	}
	return col
}

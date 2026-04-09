// Package text wraps gui.TextMeasurer for the editor: cached monospace
// metrics, ASCII fast-path column↔x conversion, and fallback to
// go-glyph's Layout for non-ASCII lines.
//
// This is the single choke point between go-edit and the text-shaping
// stack. The editor never imports go-glyph directly except through this
// package. Swap underlying shapers by changing this file.
package text

import (
	"slices"
	"unicode/utf8"

	"github.com/mike-ward/go-glyph"
	"github.com/mike-ward/go-gui/gui"
)

// DefaultTabWidth is the tab stop interval in columns when no
// explicit width is configured.
const DefaultTabWidth = 4

// Measurer caches monospace metrics for a single TextStyle and exposes
// byte-column ↔ pixel-x conversions plus line height.
type Measurer struct {
	tm         gui.TextMeasurer
	style      gui.TextStyle
	advance    float32
	lineHeight float32
	TabWidth   int // tab stop interval in columns; 0 → DefaultTabWidth
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

// NewFake builds a Measurer with fixed advance and line height,
// without a real backend. ASCII fast-path only. For tests.
func NewFake(advance, lineHeight float32) *Measurer {
	return &Measurer{
		advance:    advance,
		lineHeight: lineHeight,
	}
}

// Advance returns the cached width of a single monospace glyph.
func (m *Measurer) Advance() float32 { return m.advance }

// LineHeight returns the cached line height.
func (m *Measurer) LineHeight() float32 { return m.lineHeight }

// Style returns the text style this measurer was built with.
func (m *Measurer) Style() gui.TextStyle { return m.style }

// XForColumn returns the x-offset of the cursor at byteCol within
// lineBytes. ASCII-only lines use the monospace fast path (with
// tab-stop expansion); any non-ASCII byte falls back to go-glyph
// layout.
func (m *Measurer) XForColumn(lineBytes []byte, byteCol int) float32 {
	if m == nil || byteCol <= 0 {
		return 0
	}
	if byteCol > len(lineBytes) {
		byteCol = len(lineBytes)
	}
	if isASCII(lineBytes[:byteCol]) {
		vcols := VisualCols(lineBytes, byteCol, m.tabWidth())
		return float32(vcols) * m.advance
	}
	layout, err := m.tm.LayoutText(string(lineBytes), m.style, 0)
	if err != nil {
		vcols := VisualCols(lineBytes, byteCol, m.tabWidth())
		return float32(vcols) * m.advance
	}
	cp, ok := layout.GetCursorPos(byteCol)
	if !ok {
		vcols := VisualCols(lineBytes, byteCol, m.tabWidth())
		return float32(vcols) * m.advance
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
		// Convert x to visual column, then map back to byte col.
		tw := m.tabWidth()
		targetVCol := int((x + m.advance/2) / m.advance)
		return byteColForVisualCol(lineBytes, targetVCol, tw)
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

func (m *Measurer) tabWidth() int {
	if m.TabWidth > 0 {
		return m.TabWidth
	}
	return DefaultTabWidth
}

// VisualCols returns the number of visual columns occupied by
// p[:byteCol], expanding tabs to tab stops.
func VisualCols(p []byte, byteCol, tabWidth int) int {
	vcol := 0
	for i := range byteCol {
		if p[i] == '\t' {
			vcol = vcol/tabWidth*tabWidth + tabWidth
		} else {
			vcol++
		}
	}
	return vcol
}

// byteColForVisualCol returns the byte column at or just past the
// given visual column, expanding tabs to tab stops.
func byteColForVisualCol(p []byte, targetVCol, tabWidth int) int {
	vcol := 0
	for i, b := range p {
		if vcol >= targetVCol {
			return i
		}
		if b == '\t' {
			vcol = vcol/tabWidth*tabWidth + tabWidth
		} else {
			vcol++
		}
	}
	return len(p)
}

// ExpandTabs replaces each '\t' in line with spaces aligned to
// tab stops of width tabWidth. The returned string has the same
// visual layout as XForColumn computes. If there are no tabs the
// original bytes are returned as a string with no allocation beyond
// the string conversion.
func ExpandTabs(line []byte, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = DefaultTabWidth
	}
	// Fast path: no tabs.
	if !slices.Contains(line, '\t') {
		return string(line)
	}
	var out []byte
	vcol := 0
	for _, b := range line {
		if b == '\t' {
			next := vcol/tabWidth*tabWidth + tabWidth
			for vcol < next {
				out = append(out, ' ')
				vcol++
			}
		} else {
			out = append(out, b)
			vcol++
		}
	}
	return string(out)
}

// ExpandTabsSpan replaces tabs in a slice of line starting at the
// given visual column. Used for rendering individual spans where
// the starting visual column affects tab-stop alignment.
func ExpandTabsSpan(span []byte, startVCol, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = DefaultTabWidth
	}
	if !slices.Contains(span, '\t') {
		return string(span)
	}
	var out []byte
	vcol := startVCol
	for _, b := range span {
		if b == '\t' {
			next := vcol/tabWidth*tabWidth + tabWidth
			for vcol < next {
				out = append(out, ' ')
				vcol++
			}
		} else {
			out = append(out, b)
			vcol++
		}
	}
	return string(out)
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

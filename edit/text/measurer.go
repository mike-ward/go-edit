// Package text wraps gui.TextMeasurer for the editor: cached monospace
// metrics, ASCII fast-path column↔x conversion, and fallback to
// go-glyph's Layout for non-ASCII lines.
//
// This is the single choke point between go-edit and the text-shaping
// stack. The editor never imports go-glyph directly except through this
// package. Swap underlying shapers by changing this file.
package text

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/mike-ward/go-glyph"
	"github.com/mike-ward/go-gui/gui"
)

// errNoLayout is returned by LayoutLine when no TextMeasurer is
// available (headless tests without a backend).
var errNoLayout = errors.New("text: no layout available")

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

	// Single-entry layout cache. Non-ASCII lines hit go-glyph's
	// LayoutText multiple times per frame (cursor, selection,
	// brackets, squiggles). Caching the last layout avoids
	// redundant Pango calls within the same line.
	cacheKey string
	cacheLay glyph.Layout
	cacheOK  bool
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

// InvalidateCache clears the single-entry layout cache. Call at
// the start of each frame (AmendLayout) so stale layouts from
// edited lines are not reused.
func (m *Measurer) InvalidateCache() {
	m.cacheOK = false
	m.cacheKey = ""
}

// layoutCached returns a go-glyph Layout for lineBytes, reusing
// the cached layout when the line content matches. Returns false
// when no TextMeasurer is available (headless).
func (m *Measurer) layoutCached(
	lineBytes []byte,
) (glyph.Layout, bool) {
	// The compiler optimizes string(b)==s to avoid allocation.
	if m.cacheOK && string(lineBytes) == m.cacheKey {
		return m.cacheLay, true
	}
	if m.tm == nil {
		return glyph.Layout{}, false
	}
	s := string(lineBytes)
	layout, err := m.tm.LayoutText(s, m.style, 0)
	if err != nil {
		return glyph.Layout{}, false
	}
	m.cacheKey = s
	m.cacheLay = layout
	m.cacheOK = true
	return layout, true
}

// Advance returns the cached width of "M".
func (m *Measurer) Advance() float32 { return m.advance }

// LineHeight returns the cached line height in pixels.
func (m *Measurer) LineHeight() float32 { return m.lineHeight }

// Style returns the text style used by this measurer.
func (m *Measurer) Style() gui.TextStyle { return m.style }

// TextWidth returns the pixel width of s in the Measurer's style.
func (m *Measurer) TextWidth(s string) float32 {
	if m == nil || m.tm == nil {
		return 0
	}
	return m.tm.TextWidth(s, m.style)
}

// SpaceWidth returns the pixel width of a space character. Falls
// back to Advance when no TextMeasurer is available.
func (m *Measurer) SpaceWidth() float32 {
	if m == nil {
		return 0
	}
	if m.tm == nil {
		return m.advance
	}
	return m.tm.TextWidth(" ", m.style)
}

// XForColumn returns the x-offset of the cursor at byteCol within
// lineBytes. Uses go-glyph layout for pixel-accurate positioning
// with any font (proportional or monospace). Falls back to
// column-count * advance only when no layout is available
// (headless tests).
func (m *Measurer) XForColumn(lineBytes []byte, byteCol int) float32 {
	if m == nil || byteCol <= 0 {
		return 0
	}
	if byteCol > len(lineBytes) {
		byteCol = len(lineBytes)
	}
	layout, ok := m.layoutCached(lineBytes)
	if ok {
		cp, cpOK := layout.GetCursorPos(byteCol)
		if cpOK {
			return cp.X
		}
	}
	// Fallback (headless): column-count * advance.
	vcols := VisualCols(lineBytes, byteCol, m.tabWidth())
	return float32(vcols) * m.advance
}

// ColumnForX returns the byte column closest to x within
// lineBytes. Uses go-glyph hit-test for pixel-accurate mapping
// with any font. Falls back to advance-based rounding when no
// layout is available.
func (m *Measurer) ColumnForX(lineBytes []byte, x float32) int {
	if m == nil || x != x || x <= 0 { // x != x traps NaN
		return 0
	}
	layout, ok := m.layoutCached(lineBytes)
	if ok {
		idx := layout.HitTest(x, m.lineHeight/2)
		if idx < 0 {
			return len(lineBytes)
		}
		return idx
	}
	// Fallback (headless): advance-based with tab awareness.
	if m.advance <= 0 {
		return 0
	}
	tw := m.tabWidth()
	targetVCol := int((x + m.advance/2) / m.advance)
	return byteColForVisualCol(lineBytes, targetVCol, tw)
}

// LayoutLine returns the go-glyph layout for lineBytes. Exposed for
// callers that need direct access (selection rects, bidi, etc.).
// Allocates — avoid in hot paths where ASCII fast path suffices.
func (m *Measurer) LayoutLine(lineBytes []byte) (glyph.Layout, error) {
	layout, ok := m.layoutCached(lineBytes)
	if !ok {
		return glyph.Layout{}, errNoLayout
	}
	return layout, nil
}

// CharWidth returns the pixel width of the character at byteCol.
// Uses go-glyph layout; falls back to advance when unavailable.
func (m *Measurer) CharWidth(lineBytes []byte, byteCol int) float32 {
	if m == nil {
		return 0
	}
	if byteCol < 0 || byteCol >= len(lineBytes) {
		return m.advance
	}
	layout, ok := m.layoutCached(lineBytes)
	if ok {
		if cr, crOK := layout.GetCharRect(byteCol); crOK {
			return cr.Width
		}
	}
	return m.advance
}

// NextCursorPos returns the byte offset of the next valid cursor
// position after byteCol. Uses go-glyph layout for grapheme-
// cluster boundaries; falls back to rune advance when layout is
// unavailable (headless tests).
func (m *Measurer) NextCursorPos(
	lineBytes []byte, byteCol int,
) int {
	if m == nil || byteCol >= len(lineBytes) {
		return len(lineBytes)
	}
	layout, ok := m.layoutCached(lineBytes)
	if ok && len(layout.LogAttrs) > 0 {
		return layout.MoveCursorRight(byteCol)
	}
	// Fallback: advance one rune.
	_, sz := utf8.DecodeRune(lineBytes[byteCol:])
	if sz == 0 {
		sz = 1
	}
	return byteCol + sz
}

// PrevCursorPos returns the byte offset of the previous valid
// cursor position before byteCol. Mirrors NextCursorPos.
func (m *Measurer) PrevCursorPos(
	lineBytes []byte, byteCol int,
) int {
	if m == nil || byteCol <= 0 {
		return 0
	}
	layout, ok := m.layoutCached(lineBytes)
	if ok && len(layout.LogAttrs) > 0 {
		return layout.MoveCursorLeft(byteCol)
	}
	_, sz := utf8.DecodeLastRune(lineBytes[:byteCol])
	if sz == 0 {
		sz = 1
	}
	return byteCol - sz
}

func (m *Measurer) tabWidth() int {
	return normalizeTabWidth(m.TabWidth)
}

// normalizeTabWidth returns tw when positive, else DefaultTabWidth.
// Public VisualCols/ExpandTabs* accept tabWidth from untrusted
// callers (config, fuzz, tests); non-positive values would cause
// div-by-zero in the tab-stop arithmetic.
func normalizeTabWidth(tw int) int {
	if tw > 0 {
		return tw
	}
	return DefaultTabWidth
}

// VisualCols returns the number of visual columns occupied by
// p[:byteCol], expanding tabs to tab stops. Iterates by rune
// so multi-byte UTF-8 sequences count as one visual column.
// byteCol is clamped to [0, len(p)]; tabWidth <= 0 falls back
// to DefaultTabWidth.
func VisualCols(p []byte, byteCol, tabWidth int) int {
	if byteCol <= 0 {
		return 0
	}
	if byteCol > len(p) {
		byteCol = len(p)
	}
	tabWidth = normalizeTabWidth(tabWidth)
	vcol := 0
	for i := 0; i < byteCol; {
		r, sz := utf8.DecodeRune(p[i:])
		if sz == 0 {
			sz = 1
		}
		if i+sz > byteCol {
			break
		}
		if r == '\t' {
			vcol = vcol/tabWidth*tabWidth + tabWidth
		} else {
			vcol++
		}
		i += sz
	}
	return vcol
}

// byteColForVisualCol returns the byte column at or just past the
// given visual column, expanding tabs to tab stops. Iterates by
// rune so multi-byte UTF-8 sequences are not split. tabWidth <= 0
// falls back to DefaultTabWidth.
func byteColForVisualCol(p []byte, targetVCol, tabWidth int) int {
	tabWidth = normalizeTabWidth(tabWidth)
	vcol := 0
	for i := 0; i < len(p); {
		if vcol >= targetVCol {
			return i
		}
		r, sz := utf8.DecodeRune(p[i:])
		if sz == 0 {
			sz = 1
		}
		if r == '\t' {
			vcol = vcol/tabWidth*tabWidth + tabWidth
		} else {
			vcol++
		}
		i += sz
	}
	return len(p)
}

// ExpandTabs replaces each '\t' in line with spaces aligned to
// tab stops of width tabWidth. If there are no tabs the original
// bytes are returned as a string with no allocation beyond the
// string conversion.
func ExpandTabs(line []byte, tabWidth int) string {
	return ExpandTabsSpan(line, 0, tabWidth)
}

// ExpandTabsSpan replaces tabs in a slice of line starting at the
// given visual column. Used for rendering individual spans where
// the starting visual column affects tab-stop alignment.
func ExpandTabsSpan(span []byte, startVCol, tabWidth int) string {
	tabWidth = normalizeTabWidth(tabWidth)
	if !slices.Contains(span, '\t') {
		return string(span)
	}
	// Pre-allocate: each tab expands to at most tabWidth spaces.
	out := make([]byte, 0, len(span)+tabWidth*4)
	vcol := startVCol
	runStart := 0
	for i := 0; i < len(span); {
		if span[i] == '\t' {
			out = append(out, span[runStart:i]...)
			next := vcol/tabWidth*tabWidth + tabWidth
			for vcol < next {
				out = append(out, ' ')
				vcol++
			}
			i++
			runStart = i
		} else {
			if span[i] < utf8.RuneSelf {
				vcol++
				i++
			} else {
				_, sz := utf8.DecodeRune(span[i:])
				if sz == 0 {
					sz = 1
				}
				vcol++
				i += sz
			}
		}
	}
	out = append(out, span[runStart:]...)
	return string(out)
}

// IsASCII reports whether p contains only ASCII bytes.
func IsASCII(p []byte) bool {
	for _, b := range p {
		if b >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

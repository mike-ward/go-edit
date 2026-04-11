package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// WhitespaceMode controls whitespace indicator rendering.
type WhitespaceMode int

const (
	// WhitespaceNone disables whitespace visualization.
	WhitespaceNone WhitespaceMode = iota
	// WhitespaceAll shows indicators for all whitespace.
	WhitespaceAll
	// WhitespaceSelection shows indicators only inside selections.
	WhitespaceSelection
)

// resolveWhitespace returns the effective whitespace mode,
// applying the runtime override if set. Override == 0 means
// "use config default"; 1..3 map to the WhitespaceMode values
// (offset by 1 to distinguish "not overridden" from
// WhitespaceNone).
func resolveWhitespace(cfg WhitespaceMode, override int) WhitespaceMode {
	if override > 0 {
		mode := WhitespaceMode(override - 1)
		if mode > WhitespaceSelection {
			return WhitespaceNone
		}
		return mode
	}
	return cfg
}

// cycleWhitespace advances the override to the next mode.
// 0 (not set) → 2 (All+1), 2 (All) → 3 (Selection), 3 → 1 (None), 1 → 2.
func cycleWhitespace(override int) int {
	switch override {
	case 0:
		return int(WhitespaceAll) + 1
	default:
		mode := WhitespaceMode(override - 1)
		next := (mode + 1) % 3
		return int(next) + 1
	}
}

// whitespaceColor is the dim color for whitespace indicators.
var whitespaceColor = gui.RGBA(100, 100, 100, 100)

// drawWhitespace renders whitespace indicators (·, →, ↵) for a
// single line. Only positions within [startCol, endCol) are drawn
// when restricted (WhitespaceSelection mode).
func drawWhitespace(
	dc *gui.DrawContext,
	lineBytes []byte,
	lineIdx int,
	textX, y, lh float32,
	m *text.Measurer,
	style gui.TextStyle,
	mode WhitespaceMode,
	sels []selInfo,
	clipLeft float32,
) {
	if mode == WhitespaceNone || m == nil {
		return
	}

	wsStyle := style
	wsStyle.Color = whitespaceColor

	for col := range len(lineBytes) {
		ch := lineBytes[col]
		if ch != ' ' && ch != '\t' {
			continue
		}
		if mode == WhitespaceSelection &&
			!byteInSelection(lineIdx, col, sels) {
			continue
		}
		x := textX + m.XForColumn(lineBytes, col)
		if x < clipLeft {
			continue
		}
		if ch == ' ' {
			dc.Text(x, y, "·", wsStyle)
		} else {
			dc.Text(x, y, "→", wsStyle)
		}
	}

	// EOL indicator.
	if mode == WhitespaceAll ||
		(mode == WhitespaceSelection &&
			byteInSelection(lineIdx, len(lineBytes), sels)) {
		x := textX + m.XForColumn(lineBytes, len(lineBytes))
		if x < clipLeft {
			return
		}
		dc.Text(x, y, "↵", wsStyle)
	}
}

// byteInSelection reports whether byte offset col on line lineIdx
// falls within any of the given selections.
func byteInSelection(lineIdx, col int, sels []selInfo) bool {
	pos := buffer.Position{Line: lineIdx, ByteCol: col}
	for i := range sels {
		if !sels[i].hasSel {
			continue
		}
		s := sels[i].sel
		if !pos.Before(s.Start) && pos.Before(s.End) {
			return true
		}
	}
	return false
}

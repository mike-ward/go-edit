package edit

import (
	"math"
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

// dc returns a minimal DrawContext sufficient for textLeftClip tests.
func newTestDC() *gui.DrawContext {
	return gui.NewDrawContext(800, 600, nil)
}

// textsIn returns the slice of text entries recorded by dc.
func textsIn(dc *gui.DrawContext) []gui.DrawCanvasTextEntry {
	return dc.Texts()
}

// --- textLeftClip passthrough cases ---

func TestTextLeftClip_NothingToClip(t *testing.T) {
	dc := newTestDC()
	textLeftClip(dc, 100, 0, "hello", gui.TextStyle{}, 50, 8)
	ts := textsIn(dc)
	if len(ts) != 1 {
		t.Fatalf("want 1 text entry, got %d", len(ts))
	}
	if ts[0].Text != "hello" {
		t.Errorf("text=%q want %q", ts[0].Text, "hello")
	}
	if ts[0].X != 100 {
		t.Errorf("x=%v want 100", ts[0].X)
	}
}

func TestTextLeftClip_EmptyString(t *testing.T) {
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "", gui.TextStyle{}, 50, 8)
	if len(textsIn(dc)) != 0 {
		t.Fatal("empty string should produce no text entry")
	}
}

func TestTextLeftClip_ExactBoundary(t *testing.T) {
	// x == clipLeft → passthrough unchanged.
	dc := newTestDC()
	textLeftClip(dc, 50, 0, "abc", gui.TextStyle{}, 50, 8)
	ts := textsIn(dc)
	if len(ts) != 1 {
		t.Fatalf("want 1 entry, got %d", len(ts))
	}
	if ts[0].X != 50 {
		t.Errorf("x=%v want 50", ts[0].X)
	}
}

// --- textLeftClip clipping cases ---

func TestTextLeftClip_PartialClip(t *testing.T) {
	// x=0, clipLeft=24, advance=8 → ceil(24/8)=3, skip "hel", draw "lo".
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "hello", gui.TextStyle{}, 24, 8)
	ts := textsIn(dc)
	if len(ts) != 1 {
		t.Fatalf("want 1 entry, got %d", len(ts))
	}
	// Drawn text must start at or past clipLeft.
	if ts[0].X < 24 {
		t.Errorf("draw x=%v is left of clipLeft=24", ts[0].X)
	}
	// Exactly 3 runes skipped ("hel"), remaining = "lo".
	if ts[0].Text != "lo" {
		t.Errorf("text=%q want %q", ts[0].Text, "lo")
	}
	if ts[0].X != 24 {
		t.Errorf("draw x=%v want 24", ts[0].X)
	}
}

func TestTextLeftClip_EntireStringClipped(t *testing.T) {
	// clipLeft far beyond end of string → nothing drawn.
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "hi", gui.TextStyle{}, 9999, 8)
	if len(textsIn(dc)) != 0 {
		t.Fatal("fully clipped string should produce no text entry")
	}
}

func TestTextLeftClip_SingleRuneClipped(t *testing.T) {
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "X", gui.TextStyle{}, 8, 8)
	// "X" at x=0 with advance=8 occupies [0,8); clipLeft=8 → skip 1 → empty.
	if len(textsIn(dc)) != 0 {
		t.Fatal("single rune fully clipped should produce no text entry")
	}
}

func TestTextLeftClip_SingleRuneVisible(t *testing.T) {
	dc := newTestDC()
	textLeftClip(dc, 8, 0, "X", gui.TextStyle{}, 8, 8)
	ts := textsIn(dc)
	if len(ts) != 1 || ts[0].Text != "X" {
		t.Fatalf("x==clipLeft: want 'X', got %v", ts)
	}
}

func TestTextLeftClip_DrawXAtOrPastClipLeft(t *testing.T) {
	// Invariant: after any partial clip, draw x must be >= clipLeft.
	adv := float32(8)
	clipLeft := float32(30)
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "abcdefgh", gui.TextStyle{}, clipLeft, adv)
	ts := textsIn(dc)
	if len(ts) == 0 {
		return // fully clipped is fine
	}
	if ts[0].X < clipLeft {
		t.Errorf("draw x=%v < clipLeft=%v", ts[0].X, clipLeft)
	}
}

func TestTextLeftClip_NegativeX(t *testing.T) {
	// x=-40 (large horizontal scroll), clipLeft=50, advance=8.
	// Need to skip ceil((50-(-40))/8) = ceil(11.25) = 12 runes.
	dc := newTestDC()
	textLeftClip(dc, -40, 0, "abcdefghijklmnop", gui.TextStyle{}, 50, 8)
	ts := textsIn(dc)
	if len(ts) == 0 {
		return // fully clipped is acceptable if string is shorter
	}
	if ts[0].X < 50 {
		t.Errorf("draw x=%v < clipLeft=50", ts[0].X)
	}
}

func TestTextLeftClip_UnicodeMultibyte(t *testing.T) {
	// UTF-8 multi-byte rune: "→" is 3 bytes. Clip should not slice mid-rune.
	dc := newTestDC()
	s := "→→→abc"
	textLeftClip(dc, 0, 0, s, gui.TextStyle{}, 24, 8)
	ts := textsIn(dc)
	if len(ts) == 0 {
		return
	}
	// Verify draw x is at or past clipLeft.
	if ts[0].X < 24 {
		t.Errorf("draw x=%v < clipLeft=24", ts[0].X)
	}
	// Verify the remaining text is valid UTF-8 (no mid-rune split).
	for _, r := range ts[0].Text {
		if r == '\uFFFD' {
			t.Errorf("replacement rune in %q (mid-rune split)", ts[0].Text)
		}
	}
}

// --- hardening: NaN/Inf advance ---

func TestTextLeftClip_NaNAdvance(t *testing.T) {
	// NaN advance: !(NaN > 0) → passthrough, no panic.
	dc := newTestDC()
	nan := float32(math.NaN())
	textLeftClip(dc, 0, 0, "hello", gui.TextStyle{}, 50, nan)
	ts := textsIn(dc)
	if len(ts) != 1 {
		t.Fatalf("NaN advance: want passthrough (1 entry), got %d", len(ts))
	}
}

func TestTextLeftClip_ZeroAdvance(t *testing.T) {
	// advance=0 → passthrough, no divide-by-zero.
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "hello", gui.TextStyle{}, 50, 0)
	ts := textsIn(dc)
	if len(ts) != 1 {
		t.Fatalf("zero advance: want passthrough, got %d entries", len(ts))
	}
}

func TestTextLeftClip_HugeClipLeft(t *testing.T) {
	// clipLeft=1e30 → skip overflows int; cap prevents infinite loop.
	dc := newTestDC()
	textLeftClip(dc, 0, 0, "hello", gui.TextStyle{}, 1e30, 8)
	// Fully clipped → no entry.
	if len(textsIn(dc)) != 0 {
		t.Fatal("huge clipLeft: expected no text entry")
	}
}

func TestTextLeftClip_InfClipLeft(t *testing.T) {
	dc := newTestDC()
	inf := float32(math.Inf(1))
	textLeftClip(dc, 0, 0, "hello", gui.TextStyle{}, inf, 8)
	if len(textsIn(dc)) != 0 {
		t.Fatal("Inf clipLeft: expected no text entry")
	}
}

// --- drawWhitespace clipLeft behaviour ---

func TestDrawWhitespace_ClipLeft_SkipsCharsInGutter(t *testing.T) {
	// textX=0, advance=8, clipLeft=16 → chars at col 0 (x=0) and col 1
	// (x=8) are skipped; col 2 (x=16) and beyond are drawn.
	m := fakeMeasurer()
	dc := newTestDC()
	line := []byte("   ") // three spaces
	drawWhitespace(dc, line, 0, 0, 0, 16, m,
		gui.TextStyle{}, WhitespaceAll, nil, 16)
	ts := textsIn(dc)
	for _, e := range ts {
		if e.X < 16 {
			t.Errorf("whitespace marker drawn at x=%v, left of clipLeft=16", e.X)
		}
	}
}

func TestDrawWhitespace_ClipLeft_EOLSkipped(t *testing.T) {
	// EOL marker sits before clipLeft → must not be drawn.
	m := fakeMeasurer()
	dc := newTestDC()
	line := []byte("a") // 1 char; EOL at x = 0+1*8 = 8
	drawWhitespace(dc, line, 0, 0, 0, 16, m,
		gui.TextStyle{}, WhitespaceAll, nil, 16)
	if len(textsIn(dc)) != 0 {
		t.Error("EOL marker before clipLeft should be suppressed")
	}
}

func TestDrawWhitespace_ClipLeft_EOLDrawn(t *testing.T) {
	// EOL marker sits at or past clipLeft → must be drawn.
	m := fakeMeasurer()
	dc := newTestDC()
	line := []byte("ab") // EOL at x = 0+2*8 = 16
	drawWhitespace(dc, line, 0, 0, 0, 16, m,
		gui.TextStyle{}, WhitespaceAll, nil, 16)
	ts := textsIn(dc)
	found := false
	for _, e := range ts {
		if e.Text == "↵" {
			found = true
		}
	}
	if !found {
		t.Error("EOL marker at clipLeft should be drawn")
	}
}

package text

import (
	"math"
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

func TestNew_NilWindow(t *testing.T) {
	if m := New(nil, gui.TextStyle{}); m != nil {
		t.Errorf("New(nil)=%v want nil", m)
	}
}

func TestMeasurer_NilReceiverSafe(t *testing.T) {
	var m *Measurer
	if got := m.XForColumn([]byte("hi"), 2); got != 0 {
		t.Errorf("XForColumn on nil=%v want 0", got)
	}
	if got := m.ColumnForX([]byte("hi"), 10); got != 0 {
		t.Errorf("ColumnForX on nil=%v want 0", got)
	}
}

func TestColumnForX_NaN(t *testing.T) {
	m := &Measurer{advance: 8, lineHeight: 16}
	nan := float32(math.NaN())
	if got := m.ColumnForX([]byte("hello"), nan); got != 0 {
		t.Errorf("ColumnForX(NaN)=%d want 0", got)
	}
}

func TestTextWidth_NilReceiver(t *testing.T) {
	var m *Measurer
	if got := m.TextWidth("hello"); got != 0 {
		t.Errorf("TextWidth on nil=%v want 0", got)
	}
}

func TestTextWidth_NilTm(t *testing.T) {
	m := &Measurer{advance: 8}
	if got := m.TextWidth("hello"); got != 0 {
		t.Errorf("TextWidth with nil tm=%v want 0", got)
	}
}

func TestSpaceWidth_NilReceiver(t *testing.T) {
	var m *Measurer
	if got := m.SpaceWidth(); got != 0 {
		t.Errorf("SpaceWidth on nil=%v want 0", got)
	}
}

func TestSpaceWidth_NilTmFallsBackToAdvance(t *testing.T) {
	m := &Measurer{advance: 10}
	if got := m.SpaceWidth(); got != 10 {
		t.Errorf("SpaceWidth with nil tm=%v want 10", got)
	}
}

func TestCharWidth_NegativeByteCol(t *testing.T) {
	m := &Measurer{advance: 8}
	if got := m.CharWidth([]byte("abc"), -1); got != 8 {
		t.Errorf("CharWidth(-1)=%v want 8 (advance)", got)
	}
}

func TestCharWidth_OOBByteCol(t *testing.T) {
	m := &Measurer{advance: 8}
	if got := m.CharWidth([]byte("abc"), 5); got != 8 {
		t.Errorf("CharWidth(5)=%v want 8 (advance)", got)
	}
}

func TestCharWidth_EmptyLine(t *testing.T) {
	m := &Measurer{advance: 8}
	if got := m.CharWidth(nil, 0); got != 8 {
		t.Errorf("CharWidth(nil,0)=%v want 8", got)
	}
}

func TestColumnForX_ZeroAdvanceFallback(t *testing.T) {
	m := &Measurer{advance: 0, lineHeight: 16}
	if got := m.ColumnForX([]byte("abc"), 10); got != 0 {
		t.Errorf("ColumnForX with zero advance=%d want 0", got)
	}
}

func TestLayoutCached_CacheHit(t *testing.T) {
	m := &Measurer{advance: 8}
	// No tm → always returns false; verify no panic.
	_, ok := m.layoutCached([]byte("hello"))
	if ok {
		t.Error("expected false with nil tm")
	}
	// Nil lineBytes.
	_, ok = m.layoutCached(nil)
	if ok {
		t.Error("expected false for nil lineBytes")
	}
}

func TestXForColumn_LayoutFallsBackOnBadIndex(t *testing.T) {
	// No tm → fallback path. byteCol beyond line end → clamped.
	m := &Measurer{advance: 8, lineHeight: 16}
	line := []byte("ab")
	got := m.XForColumn(line, 10)
	// Clamped to len(line)=2, fallback: 2*8=16.
	if got != 16 {
		t.Errorf("XForColumn(10)=%v want 16", got)
	}
}

func TestIsASCII(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"hello", true},
		{"foo bar 123 !@#", true},
		{"café", false},
		{"日本語", false},
		{"\x00\x7f", true},
		{"\x80", false},
	}
	for _, c := range cases {
		if got := IsASCII([]byte(c.in)); got != c.want {
			t.Errorf("IsASCII(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestVisualCols(t *testing.T) {
	cases := []struct {
		line    string
		byteCol int
		tabW    int
		want    int
	}{
		{"hello", 5, 4, 5},     // no tabs
		{"\thello", 1, 4, 4},   // one tab = 4 cols
		{"\thello", 6, 4, 9},   // tab(4) + hello(5)
		{"\t\thello", 2, 4, 8}, // two tabs
		{"ab\tcd", 3, 4, 4},    // tab at col 2 → next stop at 4
		{"ab\tcd", 5, 4, 6},    // after tab + cd
		{"\thello", 1, 8, 8},   // tab width 8
		{"", 0, 4, 0},          // empty
		{"abc\tdef", 4, 4, 4},  // tab at col 3 → stop at 4
	}
	for _, c := range cases {
		got := VisualCols([]byte(c.line), c.byteCol, c.tabW)
		if got != c.want {
			t.Errorf("VisualCols(%q, %d, %d)=%d want %d",
				c.line, c.byteCol, c.tabW, got, c.want)
		}
	}
}

func TestByteColForVisualCol(t *testing.T) {
	cases := []struct {
		line       string
		targetVCol int
		tabW       int
		want       int
	}{
		{"hello", 3, 4, 3},
		{"\thello", 0, 4, 0},
		{"\thello", 4, 4, 1}, // visual col 4 = after tab = byte 1
		{"\thello", 5, 4, 2}, // visual col 5 = 'e' = byte 2
		{"\thello", 9, 4, 6}, // past end
		{"ab\tcd", 4, 4, 3},  // visual col 4 = after tab = byte 3
		{"", 5, 4, 0},        // empty
	}
	for _, c := range cases {
		got := byteColForVisualCol([]byte(c.line), c.targetVCol, c.tabW)
		if got != c.want {
			t.Errorf("byteColForVisualCol(%q, %d, %d)=%d want %d",
				c.line, c.targetVCol, c.tabW, got, c.want)
		}
	}
}

func TestXForColumn_WithTabs(t *testing.T) {
	m := &Measurer{advance: 8, lineHeight: 16, TabWidth: 4}
	line := []byte("\thello")
	// After tab: 4 visual cols * 8px = 32px.
	if x := m.XForColumn(line, 1); x != 32 {
		t.Errorf("XForColumn after tab=%v want 32", x)
	}
	// After tab + "h": 5 visual cols * 8px = 40px.
	if x := m.XForColumn(line, 2); x != 40 {
		t.Errorf("XForColumn after tab+h=%v want 40", x)
	}
}

func TestColumnForX_WithTabs(t *testing.T) {
	m := &Measurer{advance: 8, lineHeight: 16, TabWidth: 4}
	line := []byte("\thello")
	// Click at x=32 (4 visual cols) → byte 1 (after tab).
	if col := m.ColumnForX(line, 32); col != 1 {
		t.Errorf("ColumnForX(32)=%d want 1", col)
	}
	// Click at x=36 (4.5 visual cols) → byte 2 (rounds to 'h').
	if col := m.ColumnForX(line, 36); col != 2 {
		t.Errorf("ColumnForX(36)=%d want 2", col)
	}
}

func TestXForColumn_DefaultTabWidth(t *testing.T) {
	m := &Measurer{advance: 8, lineHeight: 16} // TabWidth 0 → default 4
	line := []byte("\tx")
	if x := m.XForColumn(line, 1); x != 32 {
		t.Errorf("default tab width: XForColumn=%v want 32", x)
	}
}

func TestExpandTabs(t *testing.T) {
	cases := []struct {
		line string
		tabW int
		want string
	}{
		{"hello", 4, "hello"},             // no tabs
		{"\thello", 4, "    hello"},       // leading tab
		{"\t\thello", 4, "        hello"}, // two tabs
		{"ab\tcd", 4, "ab  cd"},           // mid-line tab (col 2 → stop 4)
		{"abc\tdef", 4, "abc def"},        // col 3 → stop 4 (1 space)
		{"abcd\tef", 4, "abcd    ef"},     // col 4 → stop 8 (4 spaces)
		{"\thello", 8, "        hello"},   // tab width 8
		{"", 4, ""},                       // empty
		{"\t", 4, "    "},                 // tab only
		{"a\tb\tc", 4, "a   b   c"},       // multiple mid-line tabs
	}
	for _, c := range cases {
		got := ExpandTabs([]byte(c.line), c.tabW)
		if got != c.want {
			t.Errorf("ExpandTabs(%q, %d)=%q want %q",
				c.line, c.tabW, got, c.want)
		}
	}
}

func TestExpandTabs_ZeroTabWidth(t *testing.T) {
	got := ExpandTabs([]byte("\tx"), 0) // 0 → DefaultTabWidth
	want := "    x"
	if got != want {
		t.Errorf("ExpandTabs(0)=%q want %q", got, want)
	}
}

func TestExpandTabsSpan(t *testing.T) {
	cases := []struct {
		span      string
		startVCol int
		tabW      int
		want      string
	}{
		{"hello", 0, 4, "hello"},       // no tabs
		{"\thello", 0, 4, "    hello"}, // tab at vcol 0
		{"\thello", 2, 4, "  hello"},   // tab at vcol 2 → stop 4 (2 spaces)
		{"\thello", 3, 4, " hello"},    // tab at vcol 3 → stop 4 (1 space)
		{"\thello", 4, 4, "    hello"}, // tab at vcol 4 → stop 8 (4 spaces)
		{"x\ty", 0, 4, "x   y"},        // tab after 1 char
		{"", 0, 4, ""},                 // empty span
	}
	for _, c := range cases {
		got := ExpandTabsSpan([]byte(c.span), c.startVCol, c.tabW)
		if got != c.want {
			t.Errorf("ExpandTabsSpan(%q, %d, %d)=%q want %q",
				c.span, c.startVCol, c.tabW, got, c.want)
		}
	}
}

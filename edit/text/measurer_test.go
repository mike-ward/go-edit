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
		if got := isASCII([]byte(c.in)); got != c.want {
			t.Errorf("isASCII(%q)=%v want %v", c.in, got, c.want)
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

func TestClampASCIICol(t *testing.T) {
	p := []byte("hello")
	cases := []struct {
		x    float32
		adv  float32
		want int
	}{
		{-5, 10, 0},
		{0, 10, 0},
		{14, 10, 1},   // 1.4 → rounds to 1
		{15, 10, 2},   // 1.5 → rounds to 2
		{49, 10, 5},   // 4.9 → rounds to 5 (clamped)
		{1000, 10, 5}, // past end → clamp
		{5, 0, 0},     // zero advance → 0
	}
	for _, c := range cases {
		if got := clampASCIICol(p, c.x, c.adv); got != c.want {
			t.Errorf("clampASCIICol(x=%v adv=%v)=%d want %d",
				c.x, c.adv, got, c.want)
		}
	}
}

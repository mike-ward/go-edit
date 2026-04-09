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

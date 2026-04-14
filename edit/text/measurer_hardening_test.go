package text

import (
	"math"
	"testing"
)

// TestVisualCols_Hardening exercises the hardening guards:
// byteCol out of range, tabWidth <= 0, negative byteCol.
func TestVisualCols_Hardening(t *testing.T) {
	cases := []struct {
		name     string
		p        []byte
		byteCol  int
		tabWidth int
		want     int
	}{
		{"negative byteCol", []byte("abc"), -1, 4, 0},
		{"zero byteCol", []byte("abc"), 0, 4, 0},
		{"byteCol > len", []byte("abc"), 99, 4, 3},
		{"zero tabWidth", []byte("\tabc"), 4, 0, 7}, // defaults to 4
		{"negative tabWidth", []byte("\t"), 1, -1, 4},
		{"nil bytes", nil, 5, 4, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VisualCols(c.p, c.byteCol, c.tabWidth); got != c.want {
				t.Errorf("VisualCols=%d want %d", got, c.want)
			}
		})
	}
}

// TestExpandTabsSpan_Hardening exercises the tab-width guard.
func TestExpandTabsSpan_Hardening(t *testing.T) {
	// tabWidth <= 0 must not div-by-zero.
	got := ExpandTabsSpan([]byte("\tx"), 0, 0)
	if len(got) == 0 {
		t.Fatal("empty result on tabWidth=0")
	}
	got = ExpandTabsSpan([]byte("\tx"), 0, -5)
	if len(got) == 0 {
		t.Fatal("empty result on tabWidth=-5")
	}
}

// TestColumnForX_Hardening covers NaN/Inf/huge inputs on the
// fallback (no-TextMeasurer) path.
func TestColumnForX_Hardening(t *testing.T) {
	m := NewFake(8, 16)
	line := []byte("hello")
	cases := []struct {
		name string
		x    float32
	}{
		{"NaN", float32(math.NaN())},
		{"+Inf", float32(math.Inf(1))},
		{"-Inf", float32(math.Inf(-1))},
		{"huge", 1e30},
		{"negative", -100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := m.ColumnForX(line, c.x)
			if got < 0 || got > len(line) {
				t.Errorf("ColumnForX=%d out of [0,%d]", got, len(line))
			}
		})
	}
}

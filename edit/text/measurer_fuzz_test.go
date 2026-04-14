package text

import (
	"math"
	"testing"
)

// FuzzColumnForX asserts ColumnForX never panics and always
// returns a byte offset in [0, len(line)] regardless of input,
// including NaN/±Inf/huge x values.
func FuzzColumnForX(f *testing.F) {
	f.Add([]byte(""), float32(0))
	f.Add([]byte("hello"), float32(10))
	f.Add([]byte("hello"), float32(math.NaN()))
	f.Add([]byte("hello"), float32(math.Inf(1)))
	f.Add([]byte("hello"), float32(math.Inf(-1)))
	f.Add([]byte("\t\t\t"), float32(1e30))
	f.Add([]byte("héllo"), float32(-5))

	f.Fuzz(func(t *testing.T, line []byte, x float32) {
		m := NewFake(8, 16)
		got := m.ColumnForX(line, x)
		if got < 0 || got > len(line) {
			t.Fatalf("ColumnForX=%d, line len=%d, x=%v",
				got, len(line), x)
		}
	})
}

// FuzzVisualCols asserts VisualCols never panics and returns a
// non-negative visual column count for arbitrary byte input.
func FuzzVisualCols(f *testing.F) {
	f.Add([]byte(""), 0, 4)
	f.Add([]byte("\t\t\t"), 3, 4)
	f.Add([]byte("héllo"), 10, 4)
	f.Add([]byte("\x00\xff\xfe"), 3, 4)

	f.Fuzz(func(t *testing.T, p []byte, byteCol, tabWidth int) {
		if tabWidth <= 0 || tabWidth > 64 {
			return
		}
		if byteCol < 0 {
			return
		}
		got := VisualCols(p, byteCol, tabWidth)
		if got < 0 {
			t.Fatalf("VisualCols=%d negative", got)
		}
	})
}

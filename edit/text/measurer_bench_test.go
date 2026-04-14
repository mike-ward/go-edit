package text

import (
	"bytes"
	"testing"
)

// BenchmarkExpandTabsSpan_NoTabs exercises the fast path where
// the span contains no tabs (returns string(span) directly).
func BenchmarkExpandTabsSpan_NoTabs(b *testing.B) {
	span := bytes.Repeat([]byte("abcdefgh"), 16) // 128 bytes, no tabs
	b.ReportAllocs()
	for b.Loop() {
		_ = ExpandTabsSpan(span, 0, 4)
	}
}

// BenchmarkExpandTabsSpan_WithTabs exercises the expansion path.
func BenchmarkExpandTabsSpan_WithTabs(b *testing.B) {
	span := []byte("\tfoo\tbar\tbaz\tqux")
	b.ReportAllocs()
	for b.Loop() {
		_ = ExpandTabsSpan(span, 0, 4)
	}
}

// BenchmarkVisualCols exercises the byte-iter fallback used by
// the headless/no-layout path.
func BenchmarkVisualCols(b *testing.B) {
	line := bytes.Repeat([]byte("hello world "), 16)
	n := len(line)
	b.ReportAllocs()
	for b.Loop() {
		_ = VisualCols(line, n, 4)
	}
}

// BenchmarkColumnForX_Fallback exercises the advance-based
// fallback path (no TextMeasurer attached).
func BenchmarkColumnForX_Fallback(b *testing.B) {
	m := NewFake(8, 16)
	line := bytes.Repeat([]byte("abc def "), 32)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.ColumnForX(line, 100)
	}
}

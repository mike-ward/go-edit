package buffer

import (
	"strings"
	"testing"
)

// FuzzBufferApply feeds arbitrary byte sequences as both initial
// content and edit payload. Asserts: Apply never panics, the resulting
// buffer's String() round-trips through FromBytes, and per-line byte
// sum + newline count match String().
func FuzzBufferApply(f *testing.F) {
	f.Add([]byte("hello\nworld"), []byte("X"), 0, 2, 0, 5)
	f.Add([]byte(""), []byte("\n\n"), 0, 0, 0, 0)
	f.Add([]byte("\x00\x01\xff"), []byte{}, 0, 0, 0, 3)
	f.Add([]byte("a\nb\nc"), []byte("multi\nline\ninsert"), 0, 1, 2, 0)
	// Exercise the MaxLineBytes hard-split path: a 2*MaxLineBytes
	// single-line input must open without panic and every
	// resulting line must fit the cap.
	f.Add([]byte(strings.Repeat("x", 2*MaxLineBytes)),
		[]byte("edit"), 0, 0, 0, 0)

	f.Fuzz(func(t *testing.T,
		initial, payload []byte,
		sl, sc, el, ec int,
	) {
		b := FromBytes(initial)
		// Invariant: every line fits the cap after load.
		for i := range b.LineCount() {
			if len(b.Line(i)) > MaxLineBytes {
				t.Fatalf("load produced line %d len %d > %d",
					i, len(b.Line(i)), MaxLineBytes)
			}
		}
		b.Apply(Edit{
			Range:    Range{Start: Position{sl, sc}, End: Position{el, ec}},
			NewBytes: payload,
		})
		// Invariant: Apply never produces an over-limit line
		// (rejected edits leave buffer unchanged).
		for i := range b.LineCount() {
			if len(b.Line(i)) > MaxLineBytes {
				t.Fatalf("apply left line %d len %d > %d",
					i, len(b.Line(i)), MaxLineBytes)
			}
		}
		s := b.String()
		b2 := FromBytes([]byte(s))
		if b2.String() != s {
			t.Fatalf("round-trip failed: %q -> %q", s, b2.String())
		}
		checkInvariants(t, b, 0)
	})
}

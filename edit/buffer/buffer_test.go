package buffer

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func pos(line, col int) Position { return Position{Line: line, ByteCol: col} }
func rangeOf(sl, sc, el, ec int) Range {
	return Range{Start: pos(sl, sc), End: pos(el, ec)}
}

func TestEmptyBuffer(t *testing.T) {
	b := New()
	if b.LineCount() != 1 {
		t.Fatalf("LineCount=%d want 1", b.LineCount())
	}
	if got := b.String(); got != "" {
		t.Fatalf("String=%q want empty", got)
	}
	if b.Len() != 0 {
		t.Fatalf("Len=%d want 0", b.Len())
	}
}

func TestLoadAndString(t *testing.T) {
	cases := []string{
		"",
		"hello",
		"a\nb",
		"a\nb\n",
		"\n",
		"foo\nbar\nbaz",
	}
	for _, in := range cases {
		b := FromBytes([]byte(in))
		if got := b.String(); got != in {
			t.Errorf("round-trip %q -> %q", in, got)
		}
	}
}

func TestApplyInsertSingleLine(t *testing.T) {
	b := FromBytes([]byte("foo"))
	b.Apply(Edit{Range: rangeOf(0, 1, 0, 1), NewBytes: []byte("XY")})
	if got := b.String(); got != "fXYoo" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyInsertNewline(t *testing.T) {
	b := FromBytes([]byte("foo"))
	b.Apply(Edit{Range: rangeOf(0, 1, 0, 1), NewBytes: []byte("\n")})
	if got := b.String(); got != "f\noo" {
		t.Fatalf("got %q", got)
	}
	if b.LineCount() != 2 {
		t.Fatalf("LineCount=%d", b.LineCount())
	}
}

func TestApplyInsertMultiLine(t *testing.T) {
	b := FromBytes([]byte("foo"))
	b.Apply(Edit{Range: rangeOf(0, 3, 0, 3), NewBytes: []byte("abc\ndef\nghi")})
	want := "fooabc\ndef\nghi"
	if got := b.String(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyDeleteSingleLine(t *testing.T) {
	b := FromBytes([]byte("foobar"))
	b.Apply(Edit{Range: rangeOf(0, 1, 0, 4)})
	if got := b.String(); got != "far" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDeleteAcrossLines(t *testing.T) {
	b := FromBytes([]byte("foo\nbar\nbaz"))
	b.Apply(Edit{Range: rangeOf(0, 1, 2, 2)})
	if got := b.String(); got != "fz" {
		t.Fatalf("got %q", got)
	}
	if b.LineCount() != 1 {
		t.Fatalf("LineCount=%d", b.LineCount())
	}
}

func TestApplyReplaceAcrossLines(t *testing.T) {
	b := FromBytes([]byte("foo\nbar\nbaz"))
	b.Apply(Edit{Range: rangeOf(0, 1, 2, 2), NewBytes: []byte("X\nY")})
	want := "fX\nYz"
	if got := b.String(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestChangeRecord(t *testing.T) {
	b := FromBytes([]byte("foo\nbar"))
	c := b.Apply(Edit{Range: rangeOf(0, 1, 1, 1), NewBytes: []byte("ZZ")})
	if string(c.OldBytes) != "oo\nb" {
		t.Errorf("OldBytes=%q", c.OldBytes)
	}
	if c.AppliedRange.End != pos(0, 3) {
		t.Errorf("AppliedRange.End=%+v", c.AppliedRange.End)
	}
}

func TestClampOutOfRange(t *testing.T) {
	b := FromBytes([]byte("abc"))
	// Way past end — should clamp, not panic.
	b.Apply(Edit{Range: rangeOf(99, 99, 99, 99), NewBytes: []byte("!")})
	if got := b.String(); got != "abc!" {
		t.Fatalf("got %q", got)
	}
}

// TestPropertyInvariants runs random edit sequences and asserts that
// line count and byte length reported by the buffer always match an
// independent scan of String().
func TestPropertyInvariants(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	b := FromBytes([]byte("seed line\nanother\nthird"))
	for i := range 2000 {
		mutateRandom(rng, b)
		checkInvariants(t, b, i)
	}
}

func mutateRandom(r *rand.Rand, b *Buffer) {
	s := b.String()
	if len(s) == 0 {
		b.Apply(Edit{NewBytes: []byte{byte('a' + r.IntN(26))}})
		return
	}
	start := r.IntN(len(s) + 1)
	end := start + r.IntN(min(8, len(s)-start+1))
	sp := stringPos(s, start)
	ep := stringPos(s, end)
	var nb []byte
	switch r.IntN(4) {
	case 0:
		nb = []byte{byte('a' + r.IntN(26))}
	case 1:
		nb = []byte("\n")
	case 2:
		nb = []byte("ab\ncd")
	case 3:
		nb = nil // delete only
	}
	b.Apply(Edit{Range: Range{Start: sp, End: ep}, NewBytes: nb})
}

func stringPos(s string, off int) Position {
	line := 0
	col := 0
	for i := 0; i < off && i < len(s); i++ {
		if s[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return Position{Line: line, ByteCol: col}
}

func checkInvariants(t *testing.T, b *Buffer, step int) {
	t.Helper()
	s := b.String()
	wantLines := strings.Count(s, "\n") + 1
	if b.LineCount() != wantLines {
		t.Fatalf("step %d: LineCount=%d want %d\ncontent=%q",
			step, b.LineCount(), wantLines, s)
	}
	sum := 0
	for i := 0; i < b.LineCount(); i++ {
		sum += len(b.Line(i))
	}
	wantSum := len(s) - (wantLines - 1)
	if sum != wantSum {
		t.Fatalf("step %d: per-line sum=%d want %d", step, sum, wantSum)
	}
}

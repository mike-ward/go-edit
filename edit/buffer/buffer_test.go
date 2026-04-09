package buffer

import (
	"math/rand/v2"
	"os"
	"path/filepath"
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

func TestLoadDetectsEncodingAndEOL(t *testing.T) {
	// CRLF file should normalize to LF in buffer, with Props.EOL = EOLCRLF.
	raw := "line1\r\nline2\r\n"
	b := FromBytes([]byte(raw))
	// FromBytes does NOT normalize — it's raw.
	// Load (via io.Reader) does.
	b2, err := Load(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if b2.Props.EOL != EOLCRLF {
		t.Errorf("EOL = %d, want EOLCRLF(%d)", b2.Props.EOL, EOLCRLF)
	}
	if b2.Props.Encoding != EncodingUTF8 {
		t.Errorf("Encoding = %d, want UTF8", b2.Props.Encoding)
	}
	// Buffer content should be LF-normalized.
	if got := b2.String(); got != "line1\nline2\n" {
		t.Errorf("content = %q, want LF-normalized", got)
	}
	_ = b // unused but shows FromBytes doesn't normalize
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello\r\nworld\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Props.FilePath != path {
		t.Errorf("FilePath = %q, want %q", b.Props.FilePath, path)
	}
	if b.Props.EOL != EOLCRLF {
		t.Errorf("EOL = %d, want EOLCRLF", b.Props.EOL)
	}
	if b.Props.FileMode == 0 {
		t.Error("FileMode not set")
	}
	if b.Props.ModTime.IsZero() {
		t.Error("ModTime not set")
	}
	// Content normalized to LF.
	if got := b.String(); got != "hello\nworld\n" {
		t.Errorf("content = %q", got)
	}
}

func TestDirtyFlag(t *testing.T) {
	b := New()
	if b.Dirty() {
		t.Error("new buffer should not be dirty")
	}
	b.Apply(Edit{NewBytes: []byte("x")})
	if !b.Dirty() {
		t.Error("buffer should be dirty after Apply")
	}
	b.MarkClean()
	if b.Dirty() {
		t.Error("buffer should be clean after MarkClean")
	}
}

func TestFromRawBytes_UTF16CRLF(t *testing.T) {
	// "A\r\nB\r\n" in UTF-16 LE with BOM.
	data := []byte{
		0xFF, 0xFE, // BOM
		'A', 0, '\r', 0, '\n', 0,
		'B', 0, '\r', 0, '\n', 0,
	}
	b, err := fromRawBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	// EOL must be CRLF (not mixed) — validates post-transcode detection.
	if b.Props.EOL != EOLCRLF {
		t.Errorf("EOL = %d, want EOLCRLF(%d)", b.Props.EOL, EOLCRLF)
	}
	if b.Props.Encoding != EncodingUTF16LE {
		t.Errorf("Encoding = %d, want UTF16LE", b.Props.Encoding)
	}
	// Buffer content should be LF-normalized UTF-8.
	if got := b.String(); got != "A\nB\n" {
		t.Errorf("content = %q", got)
	}
}

func TestBytes_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"empty", "", ""},
		{"single line", "hello", "hello"},
		{"newline only", "\n", "\n"},
		{"empty lines", "\n\n\n", "\n\n\n"},
		{"trailing newline", "a\nb\n", "a\nb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := FromBytes([]byte(tt.src))
			if got := string(b.Bytes()); got != tt.want {
				t.Errorf("Bytes() = %q, want %q", got, tt.want)
			}
		})
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

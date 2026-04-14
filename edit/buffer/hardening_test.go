package buffer

import (
	"io"
	"strings"
	"testing"
	"time"
)

// repeatReader produces n bytes of b without allocating a buffer.
type repeatReader struct {
	b byte
	n int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	n = min(n, r.n)
	for i := range n {
		p[i] = r.b
	}
	r.n -= n
	return n, nil
}

// newlineAfterReader emits n bytes: alternating runs of printable
// bytes separated by '\n' every `period` bytes. Used by
// max-bytes-at-limit tests to avoid hitting MaxLineBytes hard-split.
type newlineAfterReader struct {
	n, period, pos int
}

func (r *newlineAfterReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.n)
	for i := range n {
		if (r.pos+i+1)%r.period == 0 {
			p[i] = '\n'
		} else {
			p[i] = 'x'
		}
	}
	r.pos += n
	r.n -= n
	return n, nil
}

func TestLoad_NilReader(t *testing.T) {
	b, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || b.LineCount() != 1 || b.Len() != 0 {
		t.Errorf("got %+v", b)
	}
}

func TestLoad_ExceedsMaxBytes(t *testing.T) {
	// +1 byte past the cap — must reject.
	r := &repeatReader{b: 'x', n: MaxLoadBytes + 1}
	_, err := Load(r)
	if err == nil {
		t.Fatal("want error for over-limit input")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("err=%v", err)
	}
}

func TestLoad_AtExactLimit(t *testing.T) {
	// Exactly MaxLoadBytes must succeed. Use 4KiB-wide lines so
	// the MaxLineBytes cap never triggers a hard-split; Len then
	// equals the input byte count exactly.
	r := &newlineAfterReader{n: MaxLoadBytes, period: 4096}
	b, err := Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != MaxLoadBytes {
		t.Errorf("Len=%d want %d", b.Len(), MaxLoadBytes)
	}
}

// --- MaxLineBytes (W8b) ---

func TestFromBytes_HardSplitsOverlongLine(t *testing.T) {
	// One logical line, 2.5 * MaxLineBytes long. FromBytes must
	// hard-split into 3 continuation lines (sizes: Max, Max,
	// Max/2) to keep every line under the cap.
	const total = 2*MaxLineBytes + MaxLineBytes/2
	raw := make([]byte, total)
	for i := range raw {
		raw[i] = 'x'
	}
	b := FromBytes(raw)
	if b.LineCount() != 3 {
		t.Fatalf("LineCount = %d, want 3", b.LineCount())
	}
	if got := b.Line(0); len(got) != MaxLineBytes {
		t.Errorf("line 0 len = %d, want %d", len(got), MaxLineBytes)
	}
	if got := b.Line(1); len(got) != MaxLineBytes {
		t.Errorf("line 1 len = %d, want %d", len(got), MaxLineBytes)
	}
	if got := b.Line(2); len(got) != MaxLineBytes/2 {
		t.Errorf("line 2 len = %d, want %d", len(got), MaxLineBytes/2)
	}
}

func TestApply_RejectsOverlongSingleLineInsert(t *testing.T) {
	b := FromBytes([]byte("abc"))
	v := b.Version()
	// Insert bytes that would push line 0 past MaxLineBytes.
	big := make([]byte, MaxLineBytes)
	for i := range big {
		big[i] = 'y'
	}
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 3},
			End:   Position{Line: 0, ByteCol: 3},
		},
		NewBytes: big,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Errorf("expected zero Change on rejection, got %+v", c)
	}
	if b.Version() != v {
		t.Errorf("version advanced on rejected edit: %d -> %d",
			v, b.Version())
	}
	if string(b.Line(0)) != "abc" {
		t.Errorf("buffer mutated on rejected edit: %q", b.Line(0))
	}
}

func TestApply_RejectsOverlongJoinDelete(t *testing.T) {
	// Two lines that alone fit, but joining them would exceed
	// MaxLineBytes. Delete the newline between them → reject.
	first := make([]byte, MaxLineBytes-10)
	for i := range first {
		first[i] = 'a'
	}
	second := make([]byte, 20)
	for i := range second {
		second[i] = 'b'
	}
	raw := append(append(first, '\n'), second...)
	b := FromBytes(raw)
	if b.LineCount() != 2 {
		t.Fatalf("setup: LineCount = %d, want 2", b.LineCount())
	}
	v := b.Version()
	// Delete [line 0 end .. line 1 start) — collapses the newline.
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: len(first)},
			End:   Position{Line: 1, ByteCol: 0},
		},
		NewBytes: nil,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Errorf("expected zero Change on rejection, got %+v", c)
	}
	if b.Version() != v {
		t.Errorf("version advanced on rejected join: %d -> %d",
			v, b.Version())
	}
	if b.LineCount() != 2 {
		t.Errorf("join unexpectedly applied: LineCount = %d",
			b.LineCount())
	}
}

func TestApply_RejectsOverlongMultiSegmentFirstLine(t *testing.T) {
	// Seed buffer line 0 with a prefix so that prefix + segs[0]
	// exceeds MaxLineBytes even though each segment individually
	// fits.
	prefix := make([]byte, MaxLineBytes-5)
	for i := range prefix {
		prefix[i] = 'p'
	}
	b := FromBytes(prefix)
	if len(b.Line(0)) != MaxLineBytes-5 {
		t.Fatalf("setup: line 0 len = %d", len(b.Line(0)))
	}
	// First segment alone is 10 bytes; prefix(Max-5) + 10 > Max.
	payload := append(make([]byte, 10), '\n')
	for i := range 10 {
		payload[i] = 'a'
	}
	payload = append(payload, 'z') // second segment
	v := b.Version()
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: len(prefix)},
			End:   Position{Line: 0, ByteCol: len(prefix)},
		},
		NewBytes: payload,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Errorf("expected rejection on first-line overflow, got %+v", c)
	}
	if b.Version() != v {
		t.Errorf("version advanced on rejected edit: %d -> %d",
			v, b.Version())
	}
}

func TestApply_RejectsOverlongMultiSegmentLastLine(t *testing.T) {
	// Line 0 has a long suffix after the insertion column so
	// segs[last] + suffix > MaxLineBytes, but each segment
	// alone fits.
	suffix := make([]byte, MaxLineBytes-5)
	for i := range suffix {
		suffix[i] = 's'
	}
	b := FromBytes(suffix)
	// Insert at col 0: payload = "a\nBIG" where BIG (10 bytes)
	// ends on the same post-edit line as the original suffix.
	payload := []byte("a\n")
	for range 10 {
		payload = append(payload, 'z')
	}
	v := b.Version()
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 0},
			End:   Position{Line: 0, ByteCol: 0},
		},
		NewBytes: payload,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Errorf("expected rejection on last-line overflow, got %+v", c)
	}
	if b.Version() != v {
		t.Errorf("version advanced on rejected edit: %d -> %d",
			v, b.Version())
	}
}

func TestApply_RejectsOverlongMultiSegmentMiddleLine(t *testing.T) {
	// Middle segment alone exceeds MaxLineBytes.
	b := FromBytes([]byte(""))
	big := make([]byte, MaxLineBytes+1)
	for i := range big {
		big[i] = 'm'
	}
	payload := append([]byte("head\n"), big...)
	payload = append(payload, '\n')
	payload = append(payload, 't', 'a', 'i', 'l')
	v := b.Version()
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 0},
			End:   Position{Line: 0, ByteCol: 0},
		},
		NewBytes: payload,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Errorf("expected rejection on middle-segment overflow, got %+v", c)
	}
	if b.Version() != v {
		t.Errorf("version advanced on rejected edit: %d -> %d",
			v, b.Version())
	}
}

func TestApply_AcceptsMultiSegmentAtExactLimit(t *testing.T) {
	// Every resulting line is exactly MaxLineBytes — must succeed.
	seg := make([]byte, MaxLineBytes)
	for i := range seg {
		seg[i] = 'x'
	}
	payload := append(append(append([]byte{}, seg...), '\n'), seg...)
	b := New()
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 0},
			End:   Position{Line: 0, ByteCol: 0},
		},
		NewBytes: payload,
	})
	if len(c.Applied.NewBytes) == 0 {
		t.Fatal("edit at exact limit was rejected")
	}
	if b.LineCount() != 2 {
		t.Errorf("LineCount = %d, want 2", b.LineCount())
	}
	for i := range b.LineCount() {
		if len(b.Line(i)) != MaxLineBytes {
			t.Errorf("line %d len = %d, want %d",
				i, len(b.Line(i)), MaxLineBytes)
		}
	}
}

func TestApply_RejectsPayloadLargerThanMaxLoadBytes(t *testing.T) {
	// A NewBytes payload exceeding MaxLoadBytes must be rejected
	// without running bytes.Split on the payload — the guard is
	// intended to block adversarial paste DoS.
	b := New()
	huge := make([]byte, MaxLoadBytes+1)
	// Fill with a single byte; a real DoS payload would have
	// many newlines to force the multi-segment path.
	for i := range huge {
		huge[i] = 'a'
	}
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 0},
			End:   Position{Line: 0, ByteCol: 0},
		},
		NewBytes: huge,
	})
	if c.Applied.NewBytes != nil || c.Applied.Range != (Range{}) {
		t.Fatalf("expected rejection, got change %+v", c)
	}
	if b.Version() != 0 {
		t.Errorf("version advanced on rejected DoS edit: %d",
			b.Version())
	}
}

func TestApply_AcceptsEditAtExactLimit(t *testing.T) {
	// An insert that produces a line of exactly MaxLineBytes
	// must succeed — the check is "exceeds", not "equals".
	b := New()
	data := make([]byte, MaxLineBytes)
	for i := range data {
		data[i] = 'z'
	}
	c := b.Apply(Edit{
		Range: Range{
			Start: Position{Line: 0, ByteCol: 0},
			End:   Position{Line: 0, ByteCol: 0},
		},
		NewBytes: data,
	})
	if len(c.Applied.NewBytes) == 0 {
		t.Fatal("edit at exact limit was rejected")
	}
	if len(b.Line(0)) != MaxLineBytes {
		t.Errorf("line 0 len = %d, want %d",
			len(b.Line(0)), MaxLineBytes)
	}
}

// --- Phase 1.2 hardening ---

func TestLoadFile_EmptyPath(t *testing.T) {
	_, err := LoadFile("")
	if err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestWriteTo_NilWriter(t *testing.T) {
	b := New()
	_, err := b.WriteTo(nil)
	if err == nil {
		t.Fatal("want error for nil writer")
	}
}

func TestSaveFile_EmptyPath_NoProps(t *testing.T) {
	b := New()
	b.Props.FilePath = ""
	err := b.SaveFile("")
	if err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestNewWatcher_NilClock(t *testing.T) {
	// Must not panic; defaults to time.Now.
	w := NewWatcher(nil)
	w.Watch("/nonexistent", time.Now())
	// Check must not panic.
	_ = w.Check()
}

func TestWatcher_WatchEmptyPath(t *testing.T) {
	w := NewWatcher(time.Now)
	w.Watch("", time.Now())
	// Empty path silently ignored — no entries.
	if len(w.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(w.entries))
	}
}

func TestSniffEncoding_Nil(t *testing.T) {
	enc, bom := sniffEncoding(nil)
	if enc != EncodingUTF8 {
		t.Errorf("enc=%d, want UTF8", enc)
	}
	if bom {
		t.Error("unexpected BOM")
	}
}

func TestDetectEOL_Nil(t *testing.T) {
	if got := detectEOL(nil); got != EOLUnknown {
		t.Errorf("got %d, want EOLUnknown", got)
	}
}

func TestNormalizeEOL_Nil(t *testing.T) {
	if got := normalizeEOL(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestApplyEOL_Nil(t *testing.T) {
	if got := applyEOL(nil, EOLCRLF); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

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
	for i := 0; i < n; i++ {
		p[i] = r.b
	}
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
	// Exactly MaxLoadBytes must succeed.
	r := &repeatReader{b: 'x', n: MaxLoadBytes}
	b, err := Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != MaxLoadBytes {
		t.Errorf("Len=%d want %d", b.Len(), MaxLoadBytes)
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

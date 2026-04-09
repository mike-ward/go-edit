package buffer

import (
	"io"
	"strings"
	"testing"
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

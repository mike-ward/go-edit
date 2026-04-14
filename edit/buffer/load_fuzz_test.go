package buffer

import (
	"bytes"
	"testing"
)

// FuzzLoad feeds arbitrary byte payloads to Load. Invariants:
// never panics; rejects payloads over MaxLoadBytes with error;
// accepted loads produce only lines of len <= MaxLineBytes.
func FuzzLoad(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte("\x00\x01\xff"))
	f.Add([]byte("\xef\xbb\xbfutf8-bom"))
	f.Add([]byte("\xfe\xff\x00h\x00i")) // UTF-16BE w/ BOM
	f.Add([]byte("\r\nmixed\rline\nendings"))
	f.Add(bytes.Repeat([]byte("x"), 2*MaxLineBytes))

	f.Fuzz(func(t *testing.T, data []byte) {
		b, err := Load(bytes.NewReader(data))
		if err != nil {
			// Legitimate rejection (e.g. oversized). Must not
			// return a buffer.
			if b != nil {
				t.Fatalf("Load err=%v but buffer non-nil", err)
			}
			return
		}
		if b == nil {
			t.Fatal("Load returned nil buffer with nil err")
		}
		if b.LineCount() < 1 {
			t.Fatalf("LineCount=%d want >=1", b.LineCount())
		}
		for i := range b.LineCount() {
			if len(b.Line(i)) > MaxLineBytes {
				t.Fatalf("line %d len %d > %d",
					i, len(b.Line(i)), MaxLineBytes)
			}
		}
	})
}

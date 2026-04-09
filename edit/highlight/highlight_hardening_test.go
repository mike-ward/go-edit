package highlight

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestDecorate_NegativeViewport(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: -5, LastLine: -1})
	if len(decos) != 0 {
		t.Fatalf("expected no decos, got %d", len(decos))
	}
}

func TestDecorate_InvertedViewport(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 5, LastLine: 0})
	if len(decos) != 0 {
		t.Fatalf("expected no decos, got %d", len(decos))
	}
}

func TestClose_StopsObserver(t *testing.T) {
	buf := buffer.FromBytes([]byte("var x = 1"))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}

	// Tokenize once.
	h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})

	h.Close()

	// Edit after close — should not panic or affect highlighter.
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 8},
			End:   buffer.Position{Line: 0, ByteCol: 9},
		},
		NewBytes: []byte("2"),
	})

	// Decorate after close+edit — should still return cached
	// tokens (observer removed, so valid flag unchanged).
	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	_ = decos // no panic = pass
}

func TestClose_Double(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	h.Close()
	h.Close() // double close — should not panic
}

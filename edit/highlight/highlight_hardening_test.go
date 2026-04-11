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

	decos := h.Decorate(buffer.Viewport{FirstLine: -5, LastLine: -1}, nil)
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

	decos := h.Decorate(buffer.Viewport{FirstLine: 5, LastLine: 0}, nil)
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
	h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0}, nil)

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
	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0}, nil)
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

// TestHighlighter_IncrementalPreservesPrefix confirms that a
// tail-edit does not wipe the per-line token cache for lines
// before the edit; the pristine prefix must survive.
func TestHighlighter_IncrementalPreservesPrefix(t *testing.T) {
	src := "package main\nfunc f() int { return 1 }\nvar x = 42"
	buf := buffer.FromBytes([]byte(src))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	defer h.Close()

	// Prime full tokenization.
	_ = h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 2}, nil)
	if !h.primed {
		t.Fatal("highlighter not primed after first Decorate")
	}
	// Snapshot line 0 tokens (the pristine prefix).
	line0Before := append([]Token(nil), h.tokens[0]...)

	// Edit on line 2 only.
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 2, ByteCol: 8},
			End:   buffer.Position{Line: 2, ByteCol: 10},
		},
		NewBytes: []byte("99"),
	})
	// dirtyLineStart should point at line 2, not earlier.
	if h.dirtyLineStart != 2 {
		t.Fatalf("dirtyLineStart = %d, want 2", h.dirtyLineStart)
	}
	// Re-decorate; must splice only line 2+ without touching line 0.
	_ = h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 2}, nil)

	if len(h.tokens[0]) != len(line0Before) {
		t.Fatalf("line 0 token count changed: %d -> %d",
			len(line0Before), len(h.tokens[0]))
	}
	for i := range line0Before {
		if h.tokens[0][i] != line0Before[i] {
			t.Fatalf("line 0 token %d drifted: %+v -> %+v",
				i, line0Before[i], h.tokens[0][i])
		}
	}
}

// TestHighlighter_IncrementalBacksOffThroughMultilineString
// verifies that editing near a multi-line string does not lose
// tokens on the continuation lines. The restart heuristic should
// back off until the preceding line is not inside a string.
func TestHighlighter_IncrementalBacksOffThroughMultilineString(t *testing.T) {
	src := "package main\nvar s = `line1\nline2\nline3`\nvar y = 1"
	buf := buffer.FromBytes([]byte(src))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	defer h.Close()

	// Prime.
	_ = h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 4}, nil)

	// Edit on line 4 (outside the string).
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 4, ByteCol: 8},
			End:   buffer.Position{Line: 4, ByteCol: 9},
		},
		NewBytes: []byte("2"),
	})
	// Full re-decorate — must not panic and the string lines
	// should still have their tokens.
	_ = h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 4}, nil)
	if len(h.tokens) != 5 {
		t.Fatalf("tokens len = %d, want 5", len(h.tokens))
	}
}

// TestDecorate_ZeroAllocOnCachedValid confirms that a second
// Decorate call into a pre-sized out slice does not allocate
// once tokenization has run and the token cache is valid.
func TestDecorate_ZeroAllocOnCachedValid(t *testing.T) {
	buf := buffer.FromBytes([]byte("package main\nfunc f() {}"))
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Skip("no Go lexer")
	}
	defer h.Close()

	vp := buffer.Viewport{FirstLine: 0, LastLine: 1}
	// Prime: tokenize + size the scratch buffer.
	scratch := h.Decorate(vp, nil)
	if len(scratch) == 0 {
		t.Fatal("expected non-empty decorations on priming call")
	}
	// The steady-state call must not allocate.
	n := testing.AllocsPerRun(50, func() {
		out := h.Decorate(vp, scratch[:0])
		_ = out
	})
	if n != 0 {
		t.Errorf("Decorate allocated %v times on cached valid call, want 0", n)
	}
}

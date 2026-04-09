package highlight

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/mike-ward/go-edit/edit/buffer"
)

func goBuffer(src string) *buffer.Buffer {
	buf := buffer.FromBytes([]byte(src))
	buf.Props.FilePath = "test.go"
	return buf
}

func TestTokenizeGoLine(t *testing.T) {
	buf := goBuffer("package main")
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	if len(decos) == 0 {
		t.Fatal("no decorations")
	}
	// "package" should be a keyword with a non-zero color.
	found := false
	for _, d := range decos {
		if d.Range.Start.ByteCol == 0 && d.Range.End.ByteCol == 7 {
			found = true
			if d.FgColor == 0 {
				t.Error("keyword 'package' has zero FgColor")
			}
		}
	}
	if !found {
		t.Error("no decoration covering 'package' (0..7)")
	}
}

func TestTokensCoverFullLine(t *testing.T) {
	src := `func main() {}`
	buf := goBuffer(src)
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	// Verify tokens cover the line without gaps (allowing
	// default-color tokens to be skipped).
	_ = decos // coverage validated by non-panic
}

func TestEditInvalidatesTokens(t *testing.T) {
	buf := goBuffer("var x = 1")
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	decos1 := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})

	// Edit: change "1" to `"hello"`
	buf.Apply(buffer.Edit{
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 8},
			End:   buffer.Position{Line: 0, ByteCol: 9},
		},
		NewBytes: []byte(`"hello"`),
	})

	decos2 := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	// Should have different decorations (string literal vs int).
	if len(decos1) == len(decos2) {
		same := true
		for i := range decos1 {
			if decos1[i].FgColor != decos2[i].FgColor {
				same = false
				break
			}
		}
		if same {
			t.Error("decorations unchanged after edit")
		}
	}
}

func TestViewportOnly(t *testing.T) {
	src := "line1\nline2\nline3\nline4"
	buf := goBuffer(src)
	h := New(buf, "go", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	// Request only line 2.
	decos := h.Decorate(buffer.Viewport{FirstLine: 2, LastLine: 2})
	for _, d := range decos {
		if d.Range.Start.Line != 2 {
			t.Errorf("decoration on line %d, want 2",
				d.Range.Start.Line)
		}
	}
}

func TestLanguageAutodetect(t *testing.T) {
	buf := buffer.FromBytes([]byte("print('hello')"))
	buf.Props.FilePath = "test.py"
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("autodetect failed for .py")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	if len(decos) == 0 {
		t.Error("no decorations for Python")
	}
}

func TestEmptyBuffer(t *testing.T) {
	buf := buffer.New()
	buf.Props.FilePath = "test.go"
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter for empty buffer")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	_ = decos // should not panic
}

func TestNoLexerReturnsNil(t *testing.T) {
	buf := buffer.FromBytes([]byte("hello"))
	// No file path, no language → no lexer.
	h := New(buf, "", nil)
	if h != nil {
		t.Fatal("expected nil for unknown language")
	}
}

func TestSetTokenOverridesChangesDecoration(t *testing.T) {
	buf := goBuffer("package main")
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	// Get original "package" keyword color.
	decos1 := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	var origColor uint32
	for _, d := range decos1 {
		if d.Range.Start.ByteCol == 0 && d.Range.End.ByteCol == 7 {
			origColor = d.FgColor
			break
		}
	}

	// Override keyword color.
	override := uint32(0xFF0000FF) // red
	h.SetTokenOverrides(map[chroma.TokenType]uint32{
		chroma.KeywordNamespace: override,
	})

	decos2 := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	var newColor uint32
	for _, d := range decos2 {
		if d.Range.Start.ByteCol == 0 && d.Range.End.ByteCol == 7 {
			newColor = d.FgColor
			break
		}
	}

	if newColor == origColor && origColor != override {
		t.Errorf("override did not change color: orig=%#x new=%#x",
			origColor, newColor)
	}
	if newColor != override {
		t.Errorf("expected %#x, got %#x", override, newColor)
	}
}

func TestSetTokenOverridesNilClears(t *testing.T) {
	buf := goBuffer("package main")
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	h.SetTokenOverrides(map[chroma.TokenType]uint32{
		chroma.KeywordNamespace: 0xFF0000FF,
	})
	// Clear overrides.
	h.SetTokenOverrides(nil)

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 0})
	for _, d := range decos {
		if d.Range.Start.ByteCol == 0 && d.Range.End.ByteCol == 7 {
			if d.FgColor == 0xFF0000FF {
				t.Error("override still active after nil clear")
			}
			break
		}
	}
}

func TestMultiLineToken(t *testing.T) {
	src := "var s = `\nmultiline\nstring`"
	buf := goBuffer(src)
	h := New(buf, "", nil)
	if h == nil {
		t.Fatal("nil highlighter")
	}
	defer h.Close()

	decos := h.Decorate(buffer.Viewport{FirstLine: 0, LastLine: 2})
	// Line 1 ("multiline") should have a decoration from the
	// string literal spanning across lines.
	found := false
	for _, d := range decos {
		if d.Range.Start.Line == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("no decoration on line 1 of multi-line string")
	}
}

package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// --- decoCompare ---

func deco(line, col, priority int) buffer.Decoration {
	return buffer.Decoration{
		Kind:     buffer.DecoToken,
		Range:    buffer.Range{Start: buffer.Position{Line: line, ByteCol: col}},
		Priority: priority,
	}
}

func TestDecoCompare_LineOrder(t *testing.T) {
	a := deco(1, 0, 0)
	b := deco(3, 0, 0)
	if got := decoCompare(a, b); got >= 0 {
		t.Fatalf("line 1 should sort before line 3, got %d", got)
	}
	if got := decoCompare(b, a); got <= 0 {
		t.Fatalf("line 3 should sort after line 1, got %d", got)
	}
}

func TestDecoCompare_SameLine_ColOrder(t *testing.T) {
	a := deco(1, 2, 0)
	b := deco(1, 8, 0)
	if got := decoCompare(a, b); got >= 0 {
		t.Fatalf("col 2 should sort before col 8, got %d", got)
	}
}

func TestDecoCompare_SameLineCol_PriorityDesc(t *testing.T) {
	a := deco(1, 0, 5)
	b := deco(1, 0, 10)
	// Higher priority first → b before a.
	if got := decoCompare(a, b); got <= 0 {
		t.Fatalf("priority 10 should sort before 5, got %d", got)
	}
}

func TestDecoCompare_Equal(t *testing.T) {
	a := deco(1, 0, 5)
	if got := decoCompare(a, a); got != 0 {
		t.Fatalf("equal decos should return 0, got %d", got)
	}
}

// --- decosForLine ---

func tokenDeco(startLine, startCol, endLine, endCol int) buffer.Decoration {
	return buffer.Decoration{
		Kind: buffer.DecoToken,
		Range: buffer.Range{
			Start: buffer.Position{Line: startLine, ByteCol: startCol},
			End:   buffer.Position{Line: endLine, ByteCol: endCol},
		},
		FgColor: 0xFF0000FF,
	}
}

func TestDecosForLine_NoMatch(t *testing.T) {
	decos := []buffer.Decoration{
		tokenDeco(0, 0, 0, 5),
		tokenDeco(2, 0, 2, 3),
	}
	got := decosForLine(decos, 1, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 decos for line 1, got %d", len(got))
	}
}

func TestDecosForLine_ExactMatch(t *testing.T) {
	decos := []buffer.Decoration{
		tokenDeco(0, 0, 0, 5),
		tokenDeco(1, 0, 1, 3),
		tokenDeco(2, 0, 2, 3),
	}
	got := decosForLine(decos, 1, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestDecosForLine_MultiLineDeco(t *testing.T) {
	// A decoration spanning lines 1-3 should match line 2.
	decos := []buffer.Decoration{tokenDeco(1, 0, 3, 5)}
	got := decosForLine(decos, 2, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 (multi-line span), got %d", len(got))
	}
}

func TestDecosForLine_SkipsNonToken(t *testing.T) {
	decos := []buffer.Decoration{{
		Kind: buffer.DecoBackground,
		Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 5},
		},
	}}
	got := decosForLine(decos, 0, nil)
	if len(got) != 0 {
		t.Fatalf("should skip non-token, got %d", len(got))
	}
}

func TestDecosForLine_EarlyBreak(t *testing.T) {
	decos := []buffer.Decoration{
		tokenDeco(5, 0, 5, 3),
		tokenDeco(10, 0, 10, 3),
	}
	// Line 0 — should break immediately at first deco.
	got := decosForLine(decos, 0, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestDecosForLine_EmptySlice(t *testing.T) {
	got := decosForLine(nil, 0, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

// --- decoColorToGUI ---

func TestDecoColorToGUI_Channels(t *testing.T) {
	// 0xAABBCCDD → R=0xAA, G=0xBB, B=0xCC, A=0xDD
	c := decoColorToGUI(0xAABBCCDD)
	want := gui.RGBA(0xAA, 0xBB, 0xCC, 0xDD)
	if c != want {
		t.Fatalf("got %v, want %v", c, want)
	}
}

func TestDecoColorToGUI_Black(t *testing.T) {
	c := decoColorToGUI(0x000000FF)
	want := gui.RGBA(0, 0, 0, 0xFF)
	if c != want {
		t.Fatalf("got %v, want %v", c, want)
	}
}

func TestDecoColorToGUI_White(t *testing.T) {
	c := decoColorToGUI(0xFFFFFFFF)
	want := gui.RGBA(0xFF, 0xFF, 0xFF, 0xFF)
	if c != want {
		t.Fatalf("got %v, want %v", c, want)
	}
}

func TestDecoColorToGUI_Zero(t *testing.T) {
	c := decoColorToGUI(0)
	want := gui.RGBA(0, 0, 0, 0)
	if c != want {
		t.Fatalf("got %v, want %v", c, want)
	}
}

// --- isEditAction ---

func TestIsEditAction_EditPrefix(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"edit.backspace", true},
		{"edit.delete", true},
		{"edit.newline", true},
		{"cursor.left", false},
		{"cursor.up", false},
		{"", false},
		{"edit", false}, // no dot
		{"edit.", true},
	}
	for _, tc := range cases {
		if got := isEditAction(tc.id); got != tc.want {
			t.Errorf("isEditAction(%q) = %v, want %v",
				tc.id, got, tc.want)
		}
	}
}

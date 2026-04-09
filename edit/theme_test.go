package edit

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/mike-ward/go-gui/gui"
)

func TestResolveColorConfigured(t *testing.T) {
	c := gui.RGBA(255, 0, 0, 255)
	fb := gui.RGBA(0, 255, 0, 255)
	got := resolveColor(c, fb)
	if got != c {
		t.Fatalf("expected configured color, got %v", got)
	}
}

func TestResolveColorFallback(t *testing.T) {
	fb := gui.RGBA(0, 255, 0, 255)
	got := resolveColor(gui.Color{}, fb)
	if got != fb {
		t.Fatalf("expected fallback color, got %v", got)
	}
}

func TestResolveEditorThemeDefaults(t *testing.T) {
	rt := resolveEditorTheme(EditorTheme{})
	if rt.selectionBg != defaultSelectionBg {
		t.Error("selectionBg not defaulted")
	}
	if rt.bracketMatchBg != defaultBracketMatchBg {
		t.Error("bracketMatchBg not defaulted")
	}
	if rt.findBarBg != defaultFindBarBg {
		t.Error("findBarBg not defaulted")
	}
}

func TestResolveEditorThemeOverride(t *testing.T) {
	custom := gui.RGBA(1, 2, 3, 4)
	et := EditorTheme{SelectionBg: custom}
	rt := resolveEditorTheme(et)
	if rt.selectionBg != custom {
		t.Errorf("expected custom selectionBg, got %v", rt.selectionBg)
	}
}

func TestGuiColorToUint32(t *testing.T) {
	c := gui.RGBA(0xAA, 0xBB, 0xCC, 0xFF)
	got := guiColorToUint32(c)
	want := uint32(0xAABBCCFF)
	if got != want {
		t.Fatalf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestGuiColorToUint32Unset(t *testing.T) {
	if guiColorToUint32(gui.Color{}) != 0 {
		t.Fatal("unset color should return 0")
	}
}

func TestTokenOverridesFromThemeEmpty(t *testing.T) {
	m := TokenOverridesFromTheme(EditorTheme{})
	if m != nil {
		t.Fatal("expected nil for zero EditorTheme")
	}
}

func TestTokenOverridesFromThemePartial(t *testing.T) {
	et := EditorTheme{Keyword: 0xFF0000FF}
	m := TokenOverridesFromTheme(et)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if len(m) == 0 {
		t.Fatal("expected entries for keyword types")
	}
}

func TestTokenOverridesFromThemeAllFields(t *testing.T) {
	et := EditorTheme{
		Keyword:  0x010101FF,
		String:   0x020202FF,
		Number:   0x030303FF,
		Comment:  0x040404FF,
		Operator: 0x050505FF,
		Type:     0x060606FF,
		Function: 0x070707FF,
		Builtin:  0x080808FF,
	}
	m := TokenOverridesFromTheme(et)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	// Should have entries for all mapped token types.
	if len(m) < 20 {
		t.Fatalf("expected 20+ entries, got %d", len(m))
	}
}

func TestThemeOverridePipeline(t *testing.T) {
	// EditorTheme → TokenOverridesFromTheme → verify correct
	// chroma token types are mapped.
	et := EditorTheme{
		Comment: 0xAABBCCFF,
	}
	m := TokenOverridesFromTheme(et)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	for _, tt := range []chroma.TokenType{
		chroma.Comment,
		chroma.CommentSingle,
		chroma.CommentMultiline,
	} {
		c, ok := m[tt]
		if !ok {
			t.Errorf("missing override for %v", tt)
			continue
		}
		if c != 0xAABBCCFF {
			t.Errorf("token %v: got %#x, want 0xAABBCCFF", tt, c)
		}
	}
}

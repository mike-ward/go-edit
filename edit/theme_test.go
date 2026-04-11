package edit

import (
	"testing"

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

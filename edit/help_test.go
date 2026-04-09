package edit

import (
	"math"
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

func TestKeyChordName(t *testing.T) {
	tests := []struct {
		key  gui.KeyCode
		mods gui.Modifier
		want string
	}{
		{gui.KeyZ, gui.ModCtrl, "Ctrl+Z"},
		{gui.KeyZ, gui.ModCtrl | gui.ModShift, "Ctrl+Shift+Z"},
		{gui.KeyF1, 0, "F1"},
		{gui.KeyA, gui.ModSuper, "Cmd+A"},
		{gui.KeySlash, gui.ModCtrl, "Ctrl+/"},
		{gui.KeyLeft, gui.ModShift, "Shift+Left"},
		{gui.KeyEscape, 0, "Esc"},
	}
	for _, tt := range tests {
		got := keyChordName(tt.key, tt.mods)
		if got != tt.want {
			t.Errorf("keyChordName(%d, %d) = %q, want %q",
				tt.key, tt.mods, got, tt.want)
		}
	}
}

func TestActionLabel(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"cursor.left", "Cursor Left"},
		{"edit.undo", "Edit Undo"},
		{"find.openReplace", "Find OpenReplace"},
		{"help.show", "Help Show"},
	}
	for _, tt := range tests {
		got := actionLabel(tt.id)
		if got != tt.want {
			t.Errorf("actionLabel(%q) = %q, want %q",
				tt.id, got, tt.want)
		}
	}
}

func TestGatherHelpDedup(t *testing.T) {
	km1 := &Keymap{
		Name: "base",
		Bindings: []Binding{
			{Key: gui.KeyZ, Modifiers: gui.ModCtrl,
				ActionID: "edit.undo"},
		},
	}
	km2 := &Keymap{
		Name: "override",
		Bindings: []Binding{
			{Key: gui.KeyZ, Modifiers: gui.ModSuper,
				ActionID: "edit.undo"},
		},
	}
	stack := &KeymapStack{}
	stack.Push(km1)
	stack.Push(km2)

	entries := gatherHelp(stack)

	// Should have exactly one entry for edit.undo (top layer wins).
	count := 0
	for _, e := range entries {
		if e.Desc == "Edit Undo" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 edit.undo entry, got %d", count)
	}
}

func TestGatherHelpFromDefault(t *testing.T) {
	stack := &KeymapStack{}
	stack.Push(DefaultKeymap)
	entries := gatherHelp(stack)
	if len(entries) == 0 {
		t.Fatal("expected non-empty help entries")
	}
}

func testHelpEntries() []helpEntry {
	stack := &KeymapStack{}
	stack.Push(DefaultKeymap)
	return gatherHelp(stack)
}

func TestHandleHelpKeyEscape(t *testing.T) {
	st := &editorState{HelpActive: true, HelpScrollY: 100}
	e := &gui.Event{KeyCode: gui.KeyEscape}
	entries := testHelpEntries()
	if !handleHelpKey(st, e, 16, 400, entries) {
		t.Fatal("expected key consumed")
	}
	if st.HelpActive {
		t.Fatal("help should be dismissed")
	}
	if st.HelpScrollY != 0 {
		t.Fatal("scroll should be reset")
	}
}

func TestHandleHelpKeyScroll(t *testing.T) {
	entries := testHelpEntries()
	st := &editorState{HelpActive: true, HelpScrollY: 0}
	e := &gui.Event{KeyCode: gui.KeyDown}
	handleHelpKey(st, e, 16, 400, entries)
	if st.HelpScrollY != 16 {
		t.Fatalf("expected scroll 16, got %f", st.HelpScrollY)
	}

	e.KeyCode = gui.KeyUp
	handleHelpKey(st, e, 16, 400, entries)
	if st.HelpScrollY != 0 {
		t.Fatalf("expected scroll 0, got %f", st.HelpScrollY)
	}

	// Up past 0 clamps.
	e.KeyCode = gui.KeyUp
	handleHelpKey(st, e, 16, 400, entries)
	if st.HelpScrollY != 0 {
		t.Fatal("scroll should not go negative")
	}
}

func TestGatherHelpNilStack(t *testing.T) {
	entries := gatherHelp(nil)
	if entries != nil {
		t.Fatal("expected nil for nil stack")
	}
}

func TestActionLabelEmpty(t *testing.T) {
	got := actionLabel("")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestHelpContentHeightNonEmpty(t *testing.T) {
	entries := testHelpEntries()
	h := helpContentHeight(entries, 16)
	if h <= 0 {
		t.Fatalf("expected positive height, got %f", h)
	}
}

func TestHelpContentHeightZeroLh(t *testing.T) {
	entries := testHelpEntries()
	if helpContentHeight(entries, 0) != 0 {
		t.Fatal("expected 0 for zero lh")
	}
	if helpContentHeight(entries, -1) != 0 {
		t.Fatal("expected 0 for negative lh")
	}
}

func TestHelpContentHeightNaN(t *testing.T) {
	entries := testHelpEntries()
	nan := float32(math.NaN())
	if helpContentHeight(entries, nan) != 0 {
		t.Fatal("expected 0 for NaN lh")
	}
}

func TestHandleHelpKeyNaNLineHeight(t *testing.T) {
	entries := testHelpEntries()
	nan := float32(math.NaN())
	st := &editorState{HelpActive: true, HelpScrollY: 0}
	e := &gui.Event{KeyCode: gui.KeyDown}
	handleHelpKey(st, e, nan, 400, entries)
	// Should use fallback lh=16, not produce NaN.
	if st.HelpScrollY != st.HelpScrollY { // NaN check
		t.Fatal("HelpScrollY is NaN")
	}
	if st.HelpScrollY != 16 {
		t.Fatalf("expected fallback scroll 16, got %f",
			st.HelpScrollY)
	}
}

func TestClampHelpScrollNaN(t *testing.T) {
	entries := testHelpEntries()
	nan := float32(math.NaN())
	st := &editorState{HelpScrollY: nan}
	clampHelpScroll(st, entries, 16, 400)
	if st.HelpScrollY != 0 {
		t.Fatalf("expected 0 after NaN clamp, got %f",
			st.HelpScrollY)
	}
}

func TestHandleHelpKeyScrollClampsMax(t *testing.T) {
	entries := testHelpEntries()
	st := &editorState{HelpActive: true, HelpScrollY: 0}
	e := &gui.Event{KeyCode: gui.KeyPageDown}
	// Scroll way past content.
	handleHelpKey(st, e, 16, 400, entries)
	handleHelpKey(st, e, 16, 400, entries)
	handleHelpKey(st, e, 16, 400, entries)
	maxScroll := helpContentHeight(entries, 16) - 400
	if maxScroll < 0 {
		maxScroll = 0
	}
	if st.HelpScrollY > maxScroll {
		t.Fatalf("scroll %f exceeds max %f", st.HelpScrollY, maxScroll)
	}
}

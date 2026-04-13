package edit

import (
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

func TestKeymapStackResolveTopLayer(t *testing.T) {
	km := &Keymap{
		Name: "test",
		Bindings: []Binding{
			{Key: gui.KeyA, ActionID: "test.a"},
		},
	}
	var s KeymapStack
	s.Push(km)
	id, ok := s.Resolve(gui.KeyA, 0)
	if !ok || id != "test.a" {
		t.Fatalf("got %q, %v", id, ok)
	}
}

func TestKeymapStackFallThrough(t *testing.T) {
	base := &Keymap{
		Name:     "base",
		Bindings: []Binding{{Key: gui.KeyB, ActionID: "base.b"}},
	}
	top := &Keymap{
		Name:     "top",
		Bindings: []Binding{{Key: gui.KeyA, ActionID: "top.a"}},
	}
	var s KeymapStack
	s.Push(base)
	s.Push(top)
	// KeyB not in top → falls through to base.
	id, ok := s.Resolve(gui.KeyB, 0)
	if !ok || id != "base.b" {
		t.Fatalf("got %q, %v", id, ok)
	}
}

func TestKeymapStackShadow(t *testing.T) {
	base := &Keymap{
		Name:     "base",
		Bindings: []Binding{{Key: gui.KeyA, ActionID: "base.a"}},
	}
	top := &Keymap{
		Name:     "top",
		Bindings: []Binding{{Key: gui.KeyA, ActionID: "top.a"}},
	}
	var s KeymapStack
	s.Push(base)
	s.Push(top)
	id, ok := s.Resolve(gui.KeyA, 0)
	if !ok || id != "top.a" {
		t.Fatalf("got %q, %v — top should shadow base", id, ok)
	}
}

func TestKeymapStackEmptyNoMatch(t *testing.T) {
	var s KeymapStack
	_, ok := s.Resolve(gui.KeyA, 0)
	if ok {
		t.Fatal("empty stack should not match")
	}
}

func TestKeymapStackPop(t *testing.T) {
	km := &Keymap{
		Name:     "only",
		Bindings: []Binding{{Key: gui.KeyA, ActionID: "only.a"}},
	}
	var s KeymapStack
	s.Push(km)
	got := s.Pop()
	if got != km {
		t.Fatal("Pop returned wrong keymap")
	}
	_, ok := s.Resolve(gui.KeyA, 0)
	if ok {
		t.Fatal("should not match after Pop")
	}
}

func TestKeymapStackPopEmpty(t *testing.T) {
	var s KeymapStack
	if s.Pop() != nil {
		t.Fatal("Pop on empty should return nil")
	}
}

func TestKeymapModifiers(t *testing.T) {
	km := &Keymap{
		Name: "mod",
		Bindings: []Binding{
			{Key: gui.KeyA, Modifiers: gui.ModCtrl, ActionID: "ctrl.a"},
			{Key: gui.KeyA, ActionID: "plain.a"},
		},
	}
	var s KeymapStack
	s.Push(km)

	id, ok := s.Resolve(gui.KeyA, gui.ModCtrl)
	if !ok || id != "ctrl.a" {
		t.Fatalf("ctrl+a: got %q, %v", id, ok)
	}

	id, ok = s.Resolve(gui.KeyA, 0)
	if !ok || id != "plain.a" {
		t.Fatalf("plain a: got %q, %v", id, ok)
	}
}

func TestKeymap_LookupMatchesLinearScan(t *testing.T) {
	// Oracle: the pre-refactor linear scan, inlined for the test.
	linear := func(km *Keymap, key gui.KeyCode, mods gui.Modifier) (string, bool) {
		for _, b := range km.Bindings {
			if b.Key == key && b.Modifiers == mods {
				return b.ActionID, true
			}
		}
		return "", false
	}
	// Cover every binding in DefaultKeymap plus a handful of misses.
	km := DefaultKeymap
	var s KeymapStack
	s.Push(km)
	for _, b := range km.Bindings {
		want, wantOk := linear(km, b.Key, b.Modifiers)
		got, gotOk := s.Resolve(b.Key, b.Modifiers)
		if gotOk != wantOk || got != want {
			t.Fatalf("binding %+v: got (%q,%v) want (%q,%v)",
				b, got, gotOk, want, wantOk)
		}
	}
	// Miss: a key/modifier combo that should not exist.
	if _, ok := s.Resolve(gui.KeyA, gui.ModCtrlAltShift|gui.ModSuper); ok {
		t.Fatal("expected unbound combo to miss")
	}
}

func TestDefaultKeymapCoversAllActions(t *testing.T) {
	// Every action ID in DefaultKeymap must exist in
	// defaultActions (page actions are added at runtime, so
	// we check those IDs separately).
	runtime := map[string]bool{
		"cursor.up":       true,
		"cursor.down":     true,
		"select.up":       true,
		"select.down":     true,
		"cursor.pageup":   true,
		"cursor.pagedown": true,
		"select.pageup":   true,
		"select.pagedown": true,
	}
	for _, b := range DefaultKeymap.Bindings {
		if runtime[b.ActionID] {
			continue
		}
		if _, ok := defaultActions[b.ActionID]; !ok {
			t.Errorf("binding %q has no action", b.ActionID)
		}
	}
}

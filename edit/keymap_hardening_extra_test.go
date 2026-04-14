package edit

import (
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

// TestKeymap_MissTopFallsThroughToBase verifies that when the
// top layer does not bind a chord, resolution walks down to the
// base layer.
func TestKeymap_MissTopFallsThroughToBase(t *testing.T) {
	base := &Keymap{
		Name: "base",
		Bindings: []Binding{
			{Key: gui.KeyA, Modifiers: 0, ActionID: "base.a"},
		},
	}
	top := &Keymap{
		Name: "top",
		Bindings: []Binding{
			{Key: gui.KeyB, Modifiers: 0, ActionID: "top.b"},
		},
	}
	var ks KeymapStack
	ks.Push(base)
	ks.Push(top)

	id, ok := ks.Resolve(gui.KeyA, 0)
	if !ok {
		t.Fatal("miss for KeyA")
	}
	if id != "base.a" {
		t.Fatalf("fallthrough failed: id=%q want base.a", id)
	}

	id, ok = ks.Resolve(gui.KeyB, 0)
	if !ok || id != "top.b" {
		t.Fatalf("top binding broken: id=%q ok=%v", id, ok)
	}

	if id, ok := ks.Resolve(gui.KeyC, 0); ok {
		t.Fatalf("unbound chord resolved: id=%q", id)
	}
}

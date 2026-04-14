package edit

import (
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

// FuzzKeymapDispatch asserts KeymapStack.Resolve never panics
// and respects layer priority. Builds a small random stack and
// probes arbitrary (key, mods) packs.
func FuzzKeymapDispatch(f *testing.F) {
	f.Add(uint16(0x41), uint16(0x02), uint16(0x41), uint16(0x02))
	f.Add(uint16(0), uint16(0), uint16(0xFFFF), uint16(0xFFFF))
	f.Add(uint16(0x20), uint16(0x01), uint16(0x20), uint16(0x01))
	// Probe differs from bound chord — exercises miss path.
	f.Add(uint16(0x41), uint16(0x00), uint16(0x42), uint16(0x00))

	f.Fuzz(func(t *testing.T,
		k1, m1, probeK, probeM uint16,
	) {
		base := &Keymap{
			Name: "base",
			Bindings: []Binding{
				{Key: gui.KeyCode(k1), Modifiers: gui.Modifier(m1),
					ActionID: "base.action"},
			},
		}
		top := &Keymap{
			Name: "top",
			Bindings: []Binding{
				{Key: gui.KeyCode(k1), Modifiers: gui.Modifier(m1),
					ActionID: "top.action"},
			},
		}
		var ks KeymapStack
		ks.Push(base)
		ks.Push(top)

		id, ok := ks.Resolve(gui.KeyCode(probeK), gui.Modifier(probeM))

		// Top-layer must win when probe matches the bound chord.
		if probeK == k1 && probeM == m1 {
			if !ok {
				t.Fatal("Resolve miss for bound chord")
			}
			if id != "top.action" {
				t.Fatalf("Resolve=%q want top.action", id)
			}
		}

		// Unbound probes return ("", false).
		if !ok && id != "" {
			t.Fatalf("Resolve miss returned id=%q", id)
		}

		// Pop order must not panic or hang.
		_ = ks.Pop()
		_ = ks.Pop()
		_ = ks.Pop() // extra Pop on empty — must be nil, no panic
	})
}

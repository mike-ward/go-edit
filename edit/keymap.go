package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// ActionFunc is the implementation of a named editor action.
type ActionFunc func(cfg EditorCfg, st *editorState, buf *buffer.Buffer)

// Action bundles an action ID with its implementation and
// metadata controlling post-action behavior.
type Action struct {
	ID      string
	Execute ActionFunc
	// PreservesDesiredCol, when true, keeps the sticky column
	// for vertical movement (e.g. Up/Down). Most actions reset
	// DesiredCol; only vertical movement actions set this.
	PreservesDesiredCol bool
}

// Binding maps a key chord to an action ID.
type Binding struct {
	Key       gui.KeyCode
	Modifiers gui.Modifier
	ActionID  string
}

// Keymap is an ordered list of bindings. First match wins.
type Keymap struct {
	Name     string
	Bindings []Binding
}

// KeymapStack resolves key events by walking layers top to
// bottom (last pushed = highest priority).
type KeymapStack struct {
	layers []*Keymap
}

// Push adds a keymap layer on top. Nil keymaps are ignored.
func (ks *KeymapStack) Push(km *Keymap) {
	if km == nil {
		return
	}
	ks.layers = append(ks.layers, km)
}

// Pop removes and returns the top layer. Returns nil if empty.
func (ks *KeymapStack) Pop() *Keymap {
	n := len(ks.layers)
	if n == 0 {
		return nil
	}
	top := ks.layers[n-1]
	ks.layers = ks.layers[:n-1]
	return top
}

// Resolve finds the action ID for a key+modifier combo,
// searching top to bottom. Returns ("", false) if unbound.
func (ks *KeymapStack) Resolve(key gui.KeyCode, mods gui.Modifier) (string, bool) {
	for i := len(ks.layers) - 1; i >= 0; i-- {
		for _, b := range ks.layers[i].Bindings {
			if b.Key == key && b.Modifiers == mods {
				return b.ActionID, true
			}
		}
	}
	return "", false
}

package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// ActionFunc is the implementation of a named editor action.
type ActionFunc func(cfg EditorCfg, st *editorState, buf *buffer.Buffer, w *gui.Window)

// Action bundles an action ID with its implementation and
// metadata controlling post-action behavior.
type Action struct {
	ID      string
	Execute ActionFunc
	// PreservesDesiredCol, when true, keeps the sticky column
	// for vertical movement (e.g. Up/Down). Most actions reset
	// DesiredCol; only vertical movement actions set this.
	PreservesDesiredCol bool
	// PreservesAnchor, when true, keeps the selection anchor in
	// place after the action executes. Selection-extending actions
	// (select.*) set this. All other actions auto-set
	// Anchor = Cursor after execution.
	PreservesAnchor bool
	// PerCursor, when true, causes the dispatch loop to execute
	// this action independently on every cursor. Edit actions
	// run in reverse position order to avoid position invalidation.
	PerCursor bool
}

// Binding maps a key chord to an action ID.
type Binding struct {
	Key       gui.KeyCode
	Modifiers gui.Modifier
	ActionID  string
}

// Keymap is an ordered list of bindings. First match wins.
//
// Bindings is authoritative and also used by the help overlay for
// ordered enumeration. A lookup map accelerates Resolve, built
// eagerly by KeymapStack.Push. Keymap is immutable after Push.
type Keymap struct {
	Name     string
	Bindings []Binding

	// lookup is built by KeymapStack.Push from Bindings.
	lookup map[uint32]string
}

// packBinding folds (key, mods) into a single uint32 suitable as a
// map key. KeyCode is uint16; Modifier bits currently stay inside
// the low 16 bits (max observed 0x40F).
func packBinding(key gui.KeyCode, mods gui.Modifier) uint32 {
	return uint32(key)<<16 | uint32(uint16(mods))
}

// buildLookup populates k.lookup from k.Bindings. First-match
// semantics are preserved: earlier entries win, so iterate in order
// and skip keys already present.
func (k *Keymap) buildLookup() {
	lk := make(map[uint32]string, len(k.Bindings))
	for _, b := range k.Bindings {
		key := packBinding(b.Key, b.Modifiers)
		if _, exists := lk[key]; exists {
			continue
		}
		lk[key] = b.ActionID
	}
	k.lookup = lk
}

// KeymapStack resolves key events by walking layers top to
// bottom (last pushed = highest priority).
type KeymapStack struct {
	layers []*Keymap
}

// Push adds a keymap layer on top. Nil keymaps are ignored.
// Builds the layer's lookup map once here so subsequent Resolve
// calls touch only reader state — Resolve is concurrency-safe on
// a constructed stack.
func (ks *KeymapStack) Push(km *Keymap) {
	if km == nil {
		return
	}
	if km.lookup == nil && len(km.Bindings) > 0 {
		km.buildLookup()
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
// Reader-only after Push has built the lookup maps.
func (ks *KeymapStack) Resolve(key gui.KeyCode, mods gui.Modifier) (string, bool) {
	pk := packBinding(key, mods)
	for i := len(ks.layers) - 1; i >= 0; i-- {
		layer := ks.layers[i]
		if id, ok := layer.lookup[pk]; ok {
			return id, true
		}
	}
	return "", false
}

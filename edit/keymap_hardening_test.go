package edit

import (
	"testing"

	"github.com/mike-ward/go-gui/gui"
)

func TestKeymapStack_PushNil(t *testing.T) {
	var s KeymapStack
	s.Push(nil) // should be no-op
	if len(s.layers) != 0 {
		t.Fatalf("nil push added a layer")
	}
}

func TestKeymapStack_ResolveUnknownKey(t *testing.T) {
	var s KeymapStack
	s.Push(DefaultKeymap)
	_, ok := s.Resolve(gui.KeyF25, gui.ModCtrl|gui.ModAlt|gui.ModShift)
	if ok {
		t.Fatal("should not match obscure combo")
	}
}

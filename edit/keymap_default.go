package edit

import "github.com/mike-ward/go-gui/gui"

// DefaultKeymap contains the base key bindings matching the
// original hardcoded switch in editorOnKeyDown.
var DefaultKeymap = &Keymap{
	Name: "default",
	Bindings: []Binding{
		{Key: gui.KeyLeft, ActionID: "cursor.left"},
		{Key: gui.KeyRight, ActionID: "cursor.right"},
		{Key: gui.KeyUp, ActionID: "cursor.up"},
		{Key: gui.KeyDown, ActionID: "cursor.down"},
		{Key: gui.KeyHome, ActionID: "cursor.home"},
		{Key: gui.KeyEnd, ActionID: "cursor.end"},
		{Key: gui.KeyPageUp, ActionID: "cursor.pageup"},
		{Key: gui.KeyPageDown, ActionID: "cursor.pagedown"},
		{Key: gui.KeyBackspace, ActionID: "edit.backspace"},
		{Key: gui.KeyDelete, ActionID: "edit.delete"},
		{Key: gui.KeyEnter, ActionID: "edit.newline"},
	},
}

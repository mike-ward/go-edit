package edit

import "github.com/mike-ward/go-gui/gui"

// DefaultKeymap contains the base key bindings matching the
// original hardcoded switch in editorOnKeyDown.
var DefaultKeymap = &Keymap{
	Name: "default",
	Bindings: []Binding{
		// ---- cursor movement ----
		{Key: gui.KeyLeft, ActionID: "cursor.left"},
		{Key: gui.KeyRight, ActionID: "cursor.right"},
		{Key: gui.KeyUp, ActionID: "cursor.up"},
		{Key: gui.KeyDown, ActionID: "cursor.down"},
		{Key: gui.KeyHome, ActionID: "cursor.home"},
		{Key: gui.KeyEnd, ActionID: "cursor.end"},
		{Key: gui.KeyPageUp, ActionID: "cursor.pageup"},
		{Key: gui.KeyPageDown, ActionID: "cursor.pagedown"},

		// ---- selection (shift+arrow) ----
		{Key: gui.KeyLeft, Modifiers: gui.ModShift, ActionID: "select.left"},
		{Key: gui.KeyRight, Modifiers: gui.ModShift, ActionID: "select.right"},
		{Key: gui.KeyUp, Modifiers: gui.ModShift, ActionID: "select.up"},
		{Key: gui.KeyDown, Modifiers: gui.ModShift, ActionID: "select.down"},
		{Key: gui.KeyHome, Modifiers: gui.ModShift, ActionID: "select.home"},
		{Key: gui.KeyEnd, Modifiers: gui.ModShift, ActionID: "select.end"},
		{Key: gui.KeyPageUp, Modifiers: gui.ModShift, ActionID: "select.pageup"},
		{Key: gui.KeyPageDown, Modifiers: gui.ModShift, ActionID: "select.pagedown"},

		// ---- select all ----
		{Key: gui.KeyA, Modifiers: gui.ModCtrl, ActionID: "select.all"},
		{Key: gui.KeyA, Modifiers: gui.ModSuper, ActionID: "select.all"},

		// ---- editing ----
		{Key: gui.KeyBackspace, ActionID: "edit.backspace"},
		{Key: gui.KeyDelete, ActionID: "edit.delete"},
		{Key: gui.KeyEnter, ActionID: "edit.newline"},

		// ---- clipboard (Ctrl + Super variants) ----
		{Key: gui.KeyC, Modifiers: gui.ModCtrl, ActionID: "edit.copy"},
		{Key: gui.KeyC, Modifiers: gui.ModSuper, ActionID: "edit.copy"},
		{Key: gui.KeyX, Modifiers: gui.ModCtrl, ActionID: "edit.cut"},
		{Key: gui.KeyX, Modifiers: gui.ModSuper, ActionID: "edit.cut"},
		{Key: gui.KeyV, Modifiers: gui.ModCtrl, ActionID: "edit.paste"},
		{Key: gui.KeyV, Modifiers: gui.ModSuper, ActionID: "edit.paste"},

		// ---- indent ----
		{Key: gui.KeyTab, ActionID: "edit.indent"},
		{Key: gui.KeyTab, Modifiers: gui.ModShift, ActionID: "edit.dedent"},

		// ---- multi-cursor ----
		{Key: gui.KeyD, Modifiers: gui.ModCtrl, ActionID: "cursor.addNext"},
		{Key: gui.KeyD, Modifiers: gui.ModSuper, ActionID: "cursor.addNext"},
		{Key: gui.KeyEscape, ActionID: "cursor.escape"},

		// ---- line wrap ----
		{Key: gui.KeyZ, Modifiers: gui.ModAlt, ActionID: "view.toggleWrap"},

		// ---- folding ----
		{Key: gui.KeyLeftBracket, Modifiers: gui.ModCtrl | gui.ModShift, ActionID: "fold.toggle"},
		{Key: gui.KeyLeftBracket, Modifiers: gui.ModSuper | gui.ModShift, ActionID: "fold.toggle"},
		{Key: gui.KeyRightBracket, Modifiers: gui.ModCtrl | gui.ModShift, ActionID: "fold.unfoldAll"},
		{Key: gui.KeyRightBracket, Modifiers: gui.ModSuper | gui.ModShift, ActionID: "fold.unfoldAll"},

		// ---- bracket match ----
		{Key: gui.KeyBackslash, Modifiers: gui.ModCtrl | gui.ModShift, ActionID: "cursor.matchBracket"},
		{Key: gui.KeyBackslash, Modifiers: gui.ModSuper | gui.ModShift, ActionID: "cursor.matchBracket"},

		// ---- find / replace ----
		{Key: gui.KeyF, Modifiers: gui.ModCtrl, ActionID: "find.open"},
		{Key: gui.KeyF, Modifiers: gui.ModSuper, ActionID: "find.open"},
		{Key: gui.KeyH, Modifiers: gui.ModCtrl, ActionID: "find.openReplace"},
		{Key: gui.KeyH, Modifiers: gui.ModSuper, ActionID: "find.openReplace"},

		// ---- help ----
		{Key: gui.KeyF1, ActionID: "help.show"},

		// ---- comment ----
		{Key: gui.KeySlash, Modifiers: gui.ModCtrl, ActionID: "edit.toggleComment"},
		{Key: gui.KeySlash, Modifiers: gui.ModSuper, ActionID: "edit.toggleComment"},

		// ---- undo / redo ----
		{Key: gui.KeyZ, Modifiers: gui.ModCtrl, ActionID: "edit.undo"},
		{Key: gui.KeyZ, Modifiers: gui.ModSuper, ActionID: "edit.undo"},
		{Key: gui.KeyZ, Modifiers: gui.ModCtrl | gui.ModShift, ActionID: "edit.redo"},
		{Key: gui.KeyZ, Modifiers: gui.ModSuper | gui.ModShift, ActionID: "edit.redo"},
		{Key: gui.KeyY, Modifiers: gui.ModCtrl, ActionID: "edit.redo"},
	},
}

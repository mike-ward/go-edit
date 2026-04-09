package edit

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mike-ward/go-gui/gui"
)

// helpEntry is one row in the help screen.
type helpEntry struct {
	Key  string // e.g. "Ctrl+Z"
	Desc string // e.g. "Undo"
}

// gatherHelp collects bindings from all keymap layers,
// deduplicating by ActionID (top layer wins). Returns entries
// sorted by category then action name.
func gatherHelp(stack *KeymapStack) []helpEntry {
	if stack == nil {
		return nil
	}
	seen := map[string]bool{}
	var entries []helpEntry

	// Walk top→bottom so higher-priority layers win.
	for i := len(stack.layers) - 1; i >= 0; i-- {
		for _, b := range stack.layers[i].Bindings {
			if seen[b.ActionID] {
				continue
			}
			seen[b.ActionID] = true
			entries = append(entries, helpEntry{
				Key:  keyChordName(b.Key, b.Modifiers),
				Desc: actionLabel(b.ActionID),
			})
		}
	}

	slices.SortFunc(entries, func(a, b helpEntry) int {
		ca := actionCategory(a.Desc)
		cb := actionCategory(b.Desc)
		if ca != cb {
			return strings.Compare(ca, cb)
		}
		return strings.Compare(a.Desc, b.Desc)
	})
	return entries
}

// actionLabel converts "cursor.left" → "Cursor Left".
func actionLabel(id string) string {
	parts := strings.Split(id, ".")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = string(unicode.ToUpper(rune(p[0]))) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// actionCategory returns the category prefix for sorting.
func actionCategory(label string) string {
	cat, _, ok := strings.Cut(label, " ")
	if !ok {
		return label
	}
	return cat
}

// keyChordName produces a human-readable chord name like
// "Ctrl+Shift+Z".
func keyChordName(key gui.KeyCode, mods gui.Modifier) string {
	var b strings.Builder
	if mods&gui.ModCtrl != 0 {
		b.WriteString("Ctrl+")
	}
	if mods&gui.ModSuper != 0 {
		b.WriteString("Cmd+")
	}
	if mods&gui.ModAlt != 0 {
		b.WriteString("Alt+")
	}
	if mods&gui.ModShift != 0 {
		b.WriteString("Shift+")
	}
	b.WriteString(keyCodeName(key))
	return b.String()
}

// keyCodeName returns a display name for a key code.
//
//nolint:gocyclo // key-mapping switch
func keyCodeName(k gui.KeyCode) string {
	switch {
	case k >= gui.KeyA && k <= gui.KeyZ:
		return string(rune('A' + (k - gui.KeyA)))
	case k >= gui.Key0 && k <= gui.Key9:
		return string(rune('0' + (k - gui.Key0)))
	case k >= gui.KeyF1 && k <= gui.KeyF25:
		return "F" + strconv.Itoa(int(k-gui.KeyF1+1))
	}
	switch k {
	case gui.KeySpace:
		return "Space"
	case gui.KeyEnter:
		return "Enter"
	case gui.KeyTab:
		return "Tab"
	case gui.KeyBackspace:
		return "Backspace"
	case gui.KeyDelete:
		return "Del"
	case gui.KeyEscape:
		return "Esc"
	case gui.KeyUp:
		return "Up"
	case gui.KeyDown:
		return "Down"
	case gui.KeyLeft:
		return "Left"
	case gui.KeyRight:
		return "Right"
	case gui.KeyHome:
		return "Home"
	case gui.KeyEnd:
		return "End"
	case gui.KeyPageUp:
		return "PgUp"
	case gui.KeyPageDown:
		return "PgDn"
	case gui.KeySlash:
		return "/"
	case gui.KeyBackslash:
		return "\\"
	case gui.KeyLeftBracket:
		return "["
	case gui.KeyRightBracket:
		return "]"
	case gui.KeyMinus:
		return "-"
	case gui.KeyEqual:
		return "="
	case gui.KeyComma:
		return ","
	case gui.KeyPeriod:
		return "."
	}
	return "?"
}

// helpBgColor is the background for the help overlay.
var helpBgColor = gui.RGBA(20, 20, 20, 255)

// helpHeaderColor is the color for category headers.
var helpHeaderColor = gui.RGBA(120, 180, 255, 255)

// drawHelp renders the help overlay covering the full viewport.
func drawHelp(
	dc *gui.DrawContext,
	entries []helpEntry,
	scrollY float32,
	lh, advance float32,
	style gui.TextStyle,
) {
	if lh <= 0 || advance <= 0 {
		return
	}
	if scrollY != scrollY { // NaN
		scrollY = 0
	}

	// Full-viewport background.
	dc.FilledRect(0, 0, dc.Width, dc.Height, helpBgColor)

	pad := lh * 0.5
	keyColW := advance * 20 // 20 chars for key column
	descX := keyColW + pad*2

	keyStyle := style
	keyStyle.Color = gui.RGBA(200, 200, 100, 255)
	headerStyle := style
	headerStyle.Color = helpHeaderColor

	y := pad - scrollY

	// Title.
	dc.Text(pad, y, "Keyboard Shortcuts", headerStyle)
	y += lh * 1.5
	dc.Text(pad, y, "Press F1 or Esc to close", style)
	y += lh * 2

	prevCat := ""
	for _, e := range entries {
		cat := actionCategory(e.Desc)
		if cat != prevCat {
			prevCat = cat
			if y+lh > 0 && y < dc.Height {
				dc.Text(pad, y, cat, headerStyle)
			}
			y += lh * 1.3
		}
		if y+lh > 0 && y < dc.Height {
			// Right-align key text within key column.
			kw := float32(len(e.Key)) * advance
			dc.Text(pad+keyColW-kw, y, e.Key, keyStyle)
			dc.Text(descX, y, e.Desc, style)
		}
		y += lh
	}
}

// helpContentHeight returns the total height of the help content.
func helpContentHeight(entries []helpEntry, lh float32) float32 {
	if lh <= 0 || lh != lh { // zero, negative, NaN
		return 0
	}
	pad := lh * 0.5
	// Title + subtitle + gap.
	h := pad + lh*1.5 + lh*2
	prevCat := ""
	for _, e := range entries {
		cat := actionCategory(e.Desc)
		if cat != prevCat {
			prevCat = cat
			h += lh * 1.3
		}
		h += lh
	}
	h += lh + pad // last line height + bottom padding
	return h
}

// clampHelpScroll clamps HelpScrollY to [0, maxScroll].
func clampHelpScroll(
	st *editorState, entries []helpEntry,
	lh, viewportH float32,
) {
	if st.HelpScrollY != st.HelpScrollY { // NaN
		st.HelpScrollY = 0
	}
	if st.HelpScrollY < 0 {
		st.HelpScrollY = 0
	}
	maxScroll := helpContentHeight(entries, lh) - viewportH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if st.HelpScrollY > maxScroll {
		st.HelpScrollY = maxScroll
	}
}

// handleHelpKey handles key events when help is active.
// Returns true if the key was consumed.
func handleHelpKey(
	st *editorState, e *gui.Event,
	lh, viewportH float32, entries []helpEntry,
) bool {
	if lh != lh || lh <= 0 { // NaN or non-positive
		lh = 16 // safe fallback
	}
	switch e.KeyCode {
	case gui.KeyEscape, gui.KeyF1:
		st.HelpActive = false
		st.HelpScrollY = 0
		return true
	case gui.KeyDown:
		st.HelpScrollY += lh
	case gui.KeyUp:
		st.HelpScrollY -= lh
	case gui.KeyPageDown:
		st.HelpScrollY += lh * 20
	case gui.KeyPageUp:
		st.HelpScrollY -= lh * 20
	}
	clampHelpScroll(st, entries, lh, viewportH)
	return true
}

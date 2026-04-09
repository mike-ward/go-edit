package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// nsEdit is the StateMap namespace for persistent editor state
// keyed by IDFocus.
const nsEdit = "edit.state"

// capEdit caps the number of concurrently tracked editor instances
// per window.
const capEdit = 64

// editorState is the persistent per-instance state, stored in the
// window's StateMap across frames.
type editorState struct {
	Cursors  []CursorState // sorted by position; index 0 = primary
	ScrollY  float32       // scroll offset in pixels
	Measurer *text.Measurer
	Search   searchState

	// Fold state (persisted).
	FoldedRanges []FoldRange

	// View toggle overrides (0=use cfg, 1=force on, 2=force off).
	// Actions can cycle these at runtime since EditorCfg is a
	// value type and can't be mutated from inside Execute.
	WhitespaceOverride   int // cycles through WhitespaceMode values
	WrapOverride         int // 0=use cfg, 1=force on, 2=force off
	StickyScrollOverride int // 0=use cfg, 1=force on, 2=force off

	// Mouse click tracking for double/triple-click detection.
	LastClickTime int64           // UnixMilli of last mouse-down
	LastClickPos  buffer.Position // position of last click
	ClickCount    int             // 1=single, 2=double, 3=triple

	// Help screen overlay.
	HelpActive  bool
	HelpScrollY float32
}

// primary returns a pointer to the primary cursor (index 0).
// Caller must ensure Cursors is non-empty (ensureCursors does this).
func (st *editorState) primary() *CursorState {
	return &st.Cursors[0]
}

// ensureCursors guarantees at least one cursor exists.
func (st *editorState) ensureCursors() {
	if len(st.Cursors) == 0 {
		st.Cursors = []CursorState{{}}
	}
}

// editorFrameData is the per-frame snapshot shared between the
// AmendLayout callback (which has *Window) and the OnDraw callback
// (which does not). One instance per Editor(cfg) call, discarded at
// end of frame.
type editorFrameData struct {
	state      editorState
	lineHeight float32
	gutterW    float32
	padLeft    float32 // padding between gutter and text
	valid      bool    // set true by AmendLayout; OnDraw no-ops if false

	// Bracket match (transient per-frame, not persisted).
	bracketMatch [2]buffer.Position // [source, match]
	bracketFound bool

	// Wrap map (transient per-frame).
	wrapActive   bool
	wrapWidth    float32
	totalVisRows int // cached total visual rows (fold+wrap aware)

	// Cache keys for totalVisRows (avoid O(n) recompute each
	// frame when nothing changed).
	visRowsCacheWidth float32
	visRowsCacheLines int
	visRowsCacheFolds int
	visRowsDirty      bool

	// Sticky scroll (transient per-frame).
	stickyLines []int

	// Help entries (computed once, shared across closures).
	helpEntries []helpEntry
}

func loadState(w *gui.Window, id uint32) editorState {
	m := gui.StateMap[uint32, editorState](w, nsEdit, capEdit)
	s, _ := m.Get(id)
	s.ensureCursors()
	return s
}

func storeState(w *gui.Window, id uint32, s editorState) {
	m := gui.StateMap[uint32, editorState](w, nsEdit, capEdit)
	m.Set(id, s)
}

package edit

import (
	"strconv"
	"unsafe"

	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// checkDoubleMount panics if the same Editor(cfg) view was inserted
// into the layout tree twice in the same frame. Detection key is
// (current frame counter, layout pointer): a match on frameSeq
// combined with a distinct *gui.Layout pointer is a definite
// double-mount. Same pointer + same counter is benign (the test
// driver reuses one *gui.Layout across ticks even though the
// fake window does not advance FrameCount).
func checkDoubleMount(
	frame *editorFrameData, layout *gui.Layout, w *gui.Window,
) {
	currentFrame := w.FrameCount() + 1
	var layoutPtr uintptr
	if layout != nil {
		layoutPtr = uintptr(unsafe.Pointer(layout))
	}
	if frame.frameSeq == currentFrame &&
		frame.lastLayout != 0 &&
		layoutPtr != 0 &&
		frame.lastLayout != layoutPtr {
		panic("go-edit: Editor view mounted twice in the same " +
			"frame — give each mount a distinct IDFocus or " +
			"construct a new Editor(cfg) per mount site")
	}
	frame.frameSeq = currentFrame
	frame.lastLayout = layoutPtr
}

// editorAmendLayout runs each frame with *Window access. It loads
// persistent state, lazily builds the text Measurer, recomputes
// per-frame layout metrics, and publishes them via the frame struct
// so OnDraw can read them.
func editorAmendLayout(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Window) {
	invalidateSent := false
	var searchEditRemove func()
	var autoCloseRemove func()
	var foldEditRemove func()
	var visRowsEditRemove func()
	var maxContentEditRemove func()

	return func(layout *gui.Layout, w *gui.Window) {
		checkDoubleMount(frame, layout, w)
		frame.imeCommitted = false
		st := loadState(w, cfg.IDFocus)
		if st.Measurer != nil {
			st.Measurer.InvalidateCache()
		}
		if st.Measurer == nil {
			st.Measurer = text.New(w, editorMonoStyle(gui.CurrentTheme()))
			if st.Measurer == nil {
				// No backend (headless). Bail; draw will no-op.
				frame.valid = false
				return
			}
		}

		// Provide RequestRedraw thunk to async decoration
		// providers once.
		if !invalidateSent && cfg.OnInvalidate != nil {
			cfg.OnInvalidate(w.RequestRedraw)
			invalidateSent = true
		}

		applyTabWidth(cfg, st.Measurer)

		lh := st.Measurer.LineHeight()
		advance := st.Measurer.Advance()

		var gutterW float32
		if cfg.ShowLineNumbers {
			digits := len(strconv.Itoa(cfg.Buffer.LineCount()))
			digits = max(digits, 3)
			var sb [12]byte // stack buffer; covers up to 12 digits
			n := min(digits, len(sb))
			for i := range n {
				sb[i] = '0'
			}
			gutterW = st.Measurer.TextWidth(string(sb[:n])) +
				2*advance
		}

		// Clamp cursors against current buffer size — the buffer
		// may have been mutated externally between frames.
		clampCursors(&st, cfg.Buffer)

		// Snap cursors out of folded regions.
		if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
			for i := range st.Cursors {
				snapCursorOutOfFold(&st.Cursors[i],
					st.FoldedRanges)
			}
		}

		// Resolve wrap state (before clampScroll needs it).
		wrapActive := resolveWrap(cfg.LineWrap, st.WrapOverride)
		frame.wrapActive = wrapActive
		if wrapActive {
			frame.wrapWidth = cfg.Width - gutterW - advance/2
			if frame.wrapWidth < advance {
				frame.wrapWidth = advance
			}
		}

		total := cfg.Buffer.LineCount()
		updateVisRowsCache(cfg, &st, frame, wrapActive, total,
			&visRowsEditRemove)

		clampScroll(&st, cfg, frame, lh)

		updateMaxContentWidth(cfg, &st, frame, wrapActive, total,
			&maxContentEditRemove)

		// Clamp horizontal scroll.
		textAreaW := cfg.Width - gutterW - advance/2
		if wrapActive || textAreaW <= 0 {
			st.ScrollX = 0
		} else {
			maxScrollX := max(frame.maxContentW-textAreaW, 0)
			clampScrollX(&st, maxScrollX)
		}

		searchEditRemove = syncSearchObserver(
			cfg, &st, w, searchEditRemove)
		autoCloseRemove = syncAutoCloseFilter(cfg, autoCloseRemove)
		foldEditRemove = syncFoldObserver(cfg, w, foldEditRemove)

		computeBracketMatch(cfg, &st, frame)
		computeStickyScroll(cfg, &st, frame, lh)

		// Help entries (computed once, reused across frames).
		if frame.helpEntries == nil {
			hs := &KeymapStack{}
			hs.Push(DefaultKeymap)
			for _, km := range cfg.Keymaps {
				hs.Push(km)
			}
			frame.helpEntries = gatherHelp(hs)
		}

		executePendingAction(cfg, &st, frame, w)

		computeBlink(cfg, &st, frame, w)

		frame.state = st
		frame.lineHeight = lh
		frame.gutterW = gutterW
		frame.padLeft = advance / 2
		frame.valid = true

		updateCanvasOrigin(layout, frame)
		updateIMEState(cfg, &st, frame, w, gutterW,
			advance, lh, wrapActive)

		// Compute the draw cache version after all frame state is
		// populated, then write it into the DrawCanvas shape.
		// layout.Children[0] is the canvas created in the Editor
		// factory. If the layout shape has changed for any reason
		// the fold result differs and go-gui re-tessellates.
		frame.drawVersion = computeDrawVersion(cfg, &st, frame)
		if len(layout.Children) > 0 &&
			layout.Children[0].Shape != nil {
			layout.Children[0].Shape.Version = frame.drawVersion
		}

		storeState(w, cfg.IDFocus, st)
	}
}

// executePendingAction runs an action queued via TriggerAction
// (e.g. native menu). Called once per AmendLayout pass.
func executePendingAction(
	cfg EditorCfg, st *editorState, frame *editorFrameData,
	w *gui.Window,
) {
	if st.PendingAction == "" {
		return
	}
	actionID := st.PendingAction
	st.PendingAction = ""
	resetBlink(cfg, st)
	action, ok := cfg.Actions[actionID]
	if !ok {
		action, ok = defaultActions[actionID]
	}
	if !ok || (cfg.ReadOnly && isEditAction(actionID)) {
		return
	}
	isEdit := isEditAction(actionID)
	if isEdit && actionID != "edit.undo" &&
		actionID != "edit.redo" {
		cfg.Buffer.SetUndoCursorState(
			buildUndoCursorState(st))
	}
	if action.PerCursor && len(st.Cursors) > 1 {
		dispatchPerCursor(cfg, st, cfg.Buffer, w,
			action, isEdit)
	} else {
		action.Execute(cfg, st, cfg.Buffer, w)
		applyPostAction(st, action)
	}
	sortAndMerge(st)
	if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
		for i := range st.Cursors {
			snapCursorOutOfFold(&st.Cursors[i],
				st.FoldedRanges)
		}
	}
	ensureCursorVisible(st, frame, cfg)
}

// updateCanvasOrigin captures the canvas position from the layout
// tree every frame so IMESetRect works before the first click.
func updateCanvasOrigin(layout *gui.Layout, frame *editorFrameData) {
	if len(layout.Children) > 0 &&
		layout.Children[0].Shape != nil {
		s := layout.Children[0].Shape
		if s.X == s.X { // not NaN
			frame.canvasOriginX = s.X
		}
		if s.Y == s.Y {
			frame.canvasOriginY = s.Y
		}
	}
}

// updateIMEState reads IME composition state from the window and
// positions the OS candidate window near the primary cursor.
func updateIMEState(
	cfg EditorCfg, st *editorState, frame *editorFrameData,
	w *gui.Window, gutterW, advance, lh float32,
	wrapActive bool,
) {
	frame.imeComposing = w.IMEComposing()
	if frame.imeComposing {
		frame.imePreedit = w.IMECompText()
		frame.imeCursor = w.IMECompCursor()
		frame.imeSelLen = w.IMECompSelLen()
	} else {
		frame.imePreedit = ""
		frame.imeCursor = 0
		frame.imeSelLen = 0
	}
	if !frame.imeComposing || len(st.Cursors) == 0 {
		return
	}
	cs := st.Cursors[0]
	lb := cfg.Buffer.Line(cs.Cursor.Line)
	cx := gutterW + advance/2 +
		st.Measurer.XForColumn(lb, cs.Cursor.ByteCol) -
		st.ScrollX
	var visRow int
	hasFolds := cfg.EnableFolding && len(st.FoldedRanges) > 0
	if wrapActive && st.Measurer != nil {
		visRow = globalLogicalToVisualRow(
			cfg.Buffer, st.Measurer,
			frame.wrapWidth, st.FoldedRanges,
			cs.Cursor.Line)
		brk := computeBreaks(lb,
			st.Measurer, frame.wrapWidth)
		we := wrapEntry{BreakCols: brk}
		visRow += wrapCursorVisualRow(&we,
			cs.Cursor.ByteCol)
	} else if hasFolds {
		visRow = logicalToVisible(
			cs.Cursor.Line, st.FoldedRanges)
	} else {
		visRow = cs.Cursor.Line
	}
	cy := float32(visRow)*lh - st.ScrollY
	w.IMESetRect(
		frame.canvasOriginX+cx,
		frame.canvasOriginY+cy,
		1, lh)
}

// applyTabWidth syncs the measurer's tab stop from LangConfig or
// buffer indent settings. LangConfig takes precedence.
func applyTabWidth(cfg EditorCfg, m *text.Measurer) {
	if tw := resolveLangConfig(cfg).TabWidth; tw > 0 {
		m.TabWidth = tw
	} else if tw := cfg.Buffer.Props.IndentStyle.Width; tw > 0 {
		m.TabWidth = tw
	}
}

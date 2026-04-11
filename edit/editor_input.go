package edit

import (
	"maps"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// isEditAction reports whether an action ID is a mutating action
// that should be blocked in read-only mode.
func isEditAction(id string) bool {
	if id == "edit.copy" {
		return false
	}
	return strings.HasPrefix(id, "edit.")
}

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
		st := loadState(w, cfg.IDFocus)
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
			gutterW = float32(digits)*advance + 2*advance
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

		// Execute action queued via TriggerAction (e.g. native menu).
		if st.PendingAction != "" {
			actionID := st.PendingAction
			st.PendingAction = ""
			// cfg.Actions override defaultActions, matching editorOnKeyDown.
			action, ok := cfg.Actions[actionID]
			if !ok {
				action, ok = defaultActions[actionID]
			}
			if ok && (!cfg.ReadOnly || !isEditAction(actionID)) {
				isEdit := isEditAction(actionID)
				if isEdit && actionID != "edit.undo" &&
					actionID != "edit.redo" {
					cfg.Buffer.SetUndoCursorState(
						buildUndoCursorState(&st))
				}
				if action.PerCursor && len(st.Cursors) > 1 {
					dispatchPerCursor(cfg, &st, cfg.Buffer, w,
						action, isEdit)
				} else {
					action.Execute(cfg, &st, cfg.Buffer, w)
					applyPostAction(&st, action)
				}
				sortAndMerge(&st)
				// Snap cursors out of any fold the action landed in.
				if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
					for i := range st.Cursors {
						snapCursorOutOfFold(&st.Cursors[i],
							st.FoldedRanges)
					}
				}
				ensureCursorVisible(&st, frame, cfg)
			}
		}

		frame.state = st
		frame.lineHeight = lh
		frame.gutterW = gutterW
		frame.padLeft = advance / 2
		frame.valid = true

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

// computeDrawVersion folds every frame-visible input into a single
// uint64. go-gui's DrawCanvas cache is keyed by (ID, Version,
// TessWidth, TessHeight); when the fold matches the prior frame
// OnDraw is skipped and the cached tessellation is re-used.
//
// Inputs are hashed via FNV-1a. Float fields are converted through
// math.Float32bits so equal values fold identically. Cursor blink
// is deliberately NOT mixed in — when blink lands it must route
// through a separate overlay layer, not invalidate this cache.
func computeDrawVersion(
	cfg EditorCfg, st *editorState, frame *editorFrameData,
) uint64 {
	const (
		fnvOffset = 14695981039346656037
		fnvPrime  = 1099511628211
	)
	fold := func(acc, v uint64) uint64 {
		return (acc ^ v) * fnvPrime
	}
	v := uint64(fnvOffset)
	if cfg.Buffer != nil {
		v = fold(v, cfg.Buffer.Version())
	}
	v = fold(v, uint64(math.Float32bits(st.ScrollY)))
	v = fold(v, uint64(math.Float32bits(st.ScrollX)))
	v = fold(v, cursorFoldHash(st.Cursors))
	v = fold(v, uint64(len(st.FoldedRanges))<<32|
		uint64(len(st.Cursors)))
	// Fold search state: active + query length + flags.
	var searchMix uint64
	if st.Search.Active {
		searchMix |= 1
	}
	searchMix |= uint64(len(st.Search.Query)) << 8
	if st.Search.CaseSensitive {
		searchMix |= 1 << 24
	}
	if st.Search.IsRegex {
		searchMix |= 1 << 25
	}
	v = fold(v, searchMix)
	// Toggle flags + help state.
	var toggles uint64
	toggles |= uint64(st.WhitespaceOverride) & 0xff
	toggles |= (uint64(st.WrapOverride) & 0xff) << 8
	toggles |= (uint64(st.StickyScrollOverride) & 0xff) << 16
	if st.HelpActive {
		toggles |= 1 << 24
	}
	if frame.wrapActive {
		toggles |= 1 << 25
	}
	if frame.bracketFound {
		toggles |= 1 << 26
	}
	v = fold(v, toggles)
	v = fold(v, uint64(math.Float32bits(frame.wrapWidth)))
	v = fold(v, uint64(math.Float32bits(st.HelpScrollY)))
	v = fold(v, uint64(frame.totalVisRows))
	// Ensure a zero fold result never collides with the initial
	// "never drawn" state (shape.Version starts at 0).
	if v == 0 {
		v = 1
	}
	return v
}

// cursorFoldHash folds every cursor's (line, col, anchor) into a
// single uint64. Allocation-free.
func cursorFoldHash(cursors []CursorState) uint64 {
	const prime = 1099511628211
	h := uint64(14695981039346656037)
	for i := range cursors {
		c := &cursors[i]
		h = (h ^ uint64(c.Cursor.Line)) * prime
		h = (h ^ uint64(c.Cursor.ByteCol)) * prime
		h = (h ^ uint64(c.Anchor.Line)) * prime
		h = (h ^ uint64(c.Anchor.ByteCol)) * prime
	}
	return h
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

// updateVisRowsCache installs or removes the vis-rows delta
// observer and recomputes totalVisRows when the cache is stale.
// When wrap is active, the observer patches lineRowsCache and
// totalVisRows in place on each edit so the next frame does not
// walk the buffer from scratch. A full walk only runs on wrap-
// width changes, fold count changes, or the first frame.
func updateVisRowsCache(
	cfg EditorCfg,
	st *editorState,
	frame *editorFrameData,
	wrapActive bool,
	total int,
	removePtr *func(),
) {
	if wrapActive && *removePtr == nil {
		*removePtr = cfg.Buffer.OnEdit(func(c buffer.Change) {
			applyVisRowsDelta(cfg.Buffer, frame, c)
		})
	} else if !wrapActive && *removePtr != nil {
		(*removePtr)()
		*removePtr = nil
		frame.lineRowsCache = nil
	}
	stale := frame.visRowsDirty ||
		frame.visRowsCacheLines != total ||
		frame.visRowsCacheWidth != frame.wrapWidth ||
		frame.visRowsCacheFolds != len(st.FoldedRanges) ||
		(wrapActive && len(frame.lineRowsCache) != total)
	if !stale {
		return
	}
	if wrapActive && st.Measurer != nil {
		frame.totalVisRows, frame.lineRowsCache =
			buildLineRowsCache(cfg.Buffer, st.Measurer,
				frame.wrapWidth, st.FoldedRanges,
				frame.lineRowsCache)
	} else {
		frame.lineRowsCache = nil
		if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
			frame.totalVisRows = visibleLineCount(
				total, st.FoldedRanges)
		} else {
			frame.totalVisRows = total
		}
	}
	frame.visRowsCacheLines = total
	frame.visRowsCacheWidth = frame.wrapWidth
	frame.visRowsCacheFolds = len(st.FoldedRanges)
	frame.visRowsDirty = false
}

// buildLineRowsCache walks every logical line once, computing its
// wrapped visual row count. The returned slice is reused if out is
// pre-sized appropriately. Folded-interior lines contribute 0.
func buildLineRowsCache(
	buf *buffer.Buffer,
	m *text.Measurer,
	wrapWidth float32,
	folds []FoldRange,
	out []int,
) (total int, cache []int) {
	lc := buf.LineCount()
	if cap(out) >= lc {
		cache = out[:lc]
	} else {
		cache = make([]int, lc)
	}
	for i := range lc {
		if len(folds) > 0 && isFolded(folds, i) {
			cache[i] = 0
			continue
		}
		rows := wrapLineVisualRowCount(buf.Line(i), m, wrapWidth)
		cache[i] = rows
		total += rows
	}
	return total, cache
}

// applyVisRowsDelta patches lineRowsCache and totalVisRows in
// response to a single Change, avoiding a full-buffer walk on the
// common edit-then-render path. Bails out by marking the cache
// dirty if any precondition (measurer, wrap width, fold state,
// cache length) looks unsafe — the next amend will rebuild.
func applyVisRowsDelta(
	buf *buffer.Buffer, frame *editorFrameData, c buffer.Change,
) {
	if frame == nil || buf == nil {
		return
	}
	m := frame.state.Measurer
	ww := frame.wrapWidth
	if m == nil || ww <= 0 || ww != ww { // NaN
		frame.visRowsDirty = true
		return
	}
	folds := frame.state.FoldedRanges
	// Defer to full rebuild when folds are active — the existing
	// fold observer may run after this one and shift fold state,
	// so the cheapest correct answer is to rebuild next frame.
	if len(folds) > 0 {
		frame.visRowsDirty = true
		return
	}
	startLine := c.Applied.Range.Start.Line
	endLineOld := c.Applied.Range.End.Line
	endLineNew := c.AppliedRange.End.Line
	if startLine < 0 || endLineOld < startLine || endLineNew < startLine {
		frame.visRowsDirty = true
		return
	}
	if len(frame.lineRowsCache) == 0 ||
		endLineOld >= len(frame.lineRowsCache) {
		// Cache not primed yet; let updateVisRowsCache do the
		// full walk next frame.
		frame.visRowsDirty = true
		return
	}
	lc := buf.LineCount()
	if endLineNew >= lc {
		frame.visRowsDirty = true
		return
	}

	oldSum := 0
	for i := startLine; i <= endLineOld; i++ {
		oldSum += frame.lineRowsCache[i]
	}

	// Compute new per-line rows. Re-use existing capacity by
	// splicing into a single scratch slice.
	newCount := endLineNew - startLine + 1
	oldCount := endLineOld - startLine + 1
	newSum := 0
	newEntries := make([]int, newCount)
	for i := range newCount {
		line := startLine + i
		rows := wrapLineVisualRowCount(buf.Line(line), m, ww)
		newEntries[i] = rows
		newSum += rows
	}

	// Splice newEntries in place of the old range.
	if newCount == oldCount {
		copy(frame.lineRowsCache[startLine:], newEntries)
	} else {
		tail := frame.lineRowsCache[endLineOld+1:]
		spliced := make([]int, 0, startLine+newCount+len(tail))
		spliced = append(spliced, frame.lineRowsCache[:startLine]...)
		spliced = append(spliced, newEntries...)
		spliced = append(spliced, tail...)
		frame.lineRowsCache = spliced
	}
	frame.totalVisRows += newSum - oldSum
	frame.visRowsCacheLines = len(frame.lineRowsCache)
}

// updateMaxContentWidth installs or removes the max-content dirty
// observer and recomputes maxContentW when the cache is stale.
func updateMaxContentWidth(
	cfg EditorCfg,
	st *editorState,
	frame *editorFrameData,
	wrapActive bool,
	total int,
	removePtr *func(),
) {
	if !wrapActive && *removePtr == nil && st.Measurer != nil {
		*removePtr = cfg.Buffer.OnEdit(func(_ buffer.Change) {
			frame.maxContentDirty = true
		})
	} else if wrapActive && *removePtr != nil {
		(*removePtr)()
		*removePtr = nil
	}
	if !wrapActive && st.Measurer != nil &&
		(frame.maxContentDirty ||
			frame.maxContentCacheLines != total) {
		frame.maxContentW = computeMaxContentWidth(
			cfg.Buffer, st.Measurer)
		frame.maxContentCacheLines = total
		frame.maxContentDirty = false
	}
}

func editorOnKeyDown(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	// Build keymap stack and action registry once at closure
	// creation time.
	stack := &KeymapStack{}
	stack.Push(DefaultKeymap)
	for _, km := range cfg.Keymaps {
		stack.Push(km)
	}

	actions := make(map[string]Action, len(defaultActions)+6)
	maps.Copy(actions, defaultActions)
	// Page actions need frame for viewport height.
	for _, a := range []Action{
		pageUpAction(cfg, frame),
		pageDownAction(cfg, frame),
		selectPageUpAction(cfg, frame),
		selectPageDownAction(cfg, frame),
	} {
		actions[a.ID] = a
	}
	maps.Copy(actions, cfg.Actions)

	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		// Overlay intercepts: help and find bar get first
		// crack at key events, using a single state load.
		{
			st := loadState(w, cfg.IDFocus)
			if st.HelpActive {
				handleHelpKey(&st, e, frame.lineHeight,
					cfg.Height, frame.helpEntries)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
			if st.Search.Active {
				if handleSearchKey(cfg, &st, cfg.Buffer, e) {
					ensureCursorVisible(&st, frame, cfg)
					storeState(w, cfg.IDFocus, st)
					e.IsHandled = true
					return
				}
			}
		}

		actionID, ok := stack.Resolve(e.KeyCode, e.Modifiers)
		if !ok {
			return
		}
		action, ok := actions[actionID]
		if !ok {
			return
		}

		// Block edit actions in read-only mode.
		if cfg.ReadOnly && isEditAction(actionID) {
			return
		}

		st := loadState(w, cfg.IDFocus)

		// Record cursor before edit for undo (skip for undo/redo
		// themselves — they restore cursor from their own records).
		isEdit := isEditAction(actionID)
		if isEdit && actionID != "edit.undo" &&
			actionID != "edit.redo" {
			cfg.Buffer.SetUndoCursorState(buildUndoCursorState(&st))
		}

		if action.PerCursor && len(st.Cursors) > 1 {
			dispatchPerCursor(cfg, &st, cfg.Buffer, w, action, isEdit)
		} else {
			action.Execute(cfg, &st, cfg.Buffer, w)
			applyPostAction(&st, action)
		}

		// Skip cursors past folded ranges after movement.
		if cfg.EnableFolding && len(st.FoldedRanges) > 0 {
			isDown := actionID == "cursor.down" ||
				actionID == "select.down" ||
				actionID == "cursor.pagedown" ||
				actionID == "select.pagedown"
			for i := range st.Cursors {
				if isDown {
					skipFoldsDown(&st.Cursors[i],
						st.FoldedRanges)
				} else {
					skipFoldsUp(&st.Cursors[i],
						st.FoldedRanges)
				}
			}
			clampCursors(&st, cfg.Buffer)
		}

		sortAndMerge(&st)
		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnChar(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		r := rune(e.CharCode)
		if !acceptChar(r) {
			return
		}

		// Overlay intercepts: help consumes all chars;
		// find bar routes to search input.
		{
			st := loadState(w, cfg.IDFocus)
			if st.HelpActive {
				e.IsHandled = true
				return
			}
			if st.Search.Active {
				handleSearchChar(&st, cfg.Buffer, r)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
		}

		if cfg.ReadOnly {
			return
		}
		var buf2 [4]byte
		n := utf8.EncodeRune(buf2[:], r)

		st := loadState(w, cfg.IDFocus)

		// Auto-close: skip over existing closer instead of
		// inserting a duplicate. Check each cursor individually.
		if n == 1 && len(st.Cursors) > 0 {
			pairs := cfg.AutoClosePairs
			if pairs == nil {
				pairs = DefaultAutoClosePairs
			}
			allSkip := true
			for i := range st.Cursors {
				if !shouldSkipCloser(cfg.Buffer,
					st.Cursors[i].Cursor, buf2[0], pairs) {
					allSkip = false
					break
				}
			}
			if allSkip {
				for i := range st.Cursors {
					cs := &st.Cursors[i]
					ll := len(cfg.Buffer.Line(cs.Cursor.Line))
					if cs.Cursor.ByteCol < ll {
						cs.Cursor.ByteCol++
					}
					cs.ClearSelection()
					cs.DesiredCol = cs.Cursor.ByteCol
				}
				sortAndMerge(&st)
				ensureCursorVisible(&st, frame, cfg)
				storeState(w, cfg.IDFocus, st)
				e.IsHandled = true
				return
			}
		}

		cfg.Buffer.SetUndoCursorState(buildUndoCursorState(&st))

		charInsertPerCursor(&st, cfg.Buffer, buf2[:n])

		sortAndMerge(&st)
		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

func editorOnMouseScroll(cfg EditorCfg, frame *editorFrameData) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		dy := e.ScrollY
		dx := e.ScrollX
		// Shift+vertical scroll → horizontal scroll.
		if e.Modifiers.Has(gui.ModShift) && dy != 0 && dx == 0 {
			dx, dy = dy, 0
		}
		// Guard NaN/Inf.
		if dy != dy || dy > 1e6 || dy < -1e6 {
			dy = 0
		}
		if dx != dx || dx > 1e6 || dx < -1e6 {
			dx = 0
		}
		if dy == 0 && dx == 0 {
			return
		}
		st := loadState(w, cfg.IDFocus)
		if st.HelpActive {
			if dy != 0 {
				st.HelpScrollY -= dy * frame.lineHeight * 3
				clampHelpScroll(&st, frame.helpEntries,
					frame.lineHeight, cfg.Height)
			}
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}
		lh := frame.lineHeight
		if dy != 0 {
			st.ScrollY -= dy * lh * 3
			clampScroll(&st, cfg, frame, lh)
		}
		if dx != 0 && !frame.wrapActive {
			// padLeft = advance/2 so advance = padLeft*2.
			adv := frame.padLeft * 2
			if adv <= 0 {
				adv = 8
			}
			st.ScrollX += dx * adv * 3
			textAreaW := cfg.Width - frame.gutterW - frame.padLeft
			maxScrollX := max(frame.maxContentW-textAreaW, 0)
			clampScrollX(&st, maxScrollX)
		}
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

// ---------- Pure cursor math (testable without *Window) ----------

func moveLeft(cs *CursorState, buf *buffer.Buffer) {
	if cs.Cursor.ByteCol > 0 {
		cs.Cursor.ByteCol--
		return
	}
	if cs.Cursor.Line > 0 {
		cs.Cursor.Line--
		cs.Cursor.ByteCol = len(buf.Line(cs.Cursor.Line))
	}
}

func moveRight(cs *CursorState, buf *buffer.Buffer) {
	line := buf.Line(cs.Cursor.Line)
	if cs.Cursor.ByteCol < len(line) {
		cs.Cursor.ByteCol++
		return
	}
	if cs.Cursor.Line < buf.LineCount()-1 {
		cs.Cursor.Line++
		cs.Cursor.ByteCol = 0
	}
}

func moveUp(cs *CursorState, buf *buffer.Buffer, n int) {
	cs.Cursor.Line -= n
	if cs.Cursor.Line < 0 {
		cs.Cursor.Line = 0
	}
	clampCol(cs, buf)
}

func moveDown(cs *CursorState, buf *buffer.Buffer, n int) {
	cs.Cursor.Line += n
	if cs.Cursor.Line >= buf.LineCount() {
		cs.Cursor.Line = buf.LineCount() - 1
	}
	clampCol(cs, buf)
}

func clampCol(cs *CursorState, buf *buffer.Buffer) {
	ll := len(buf.Line(cs.Cursor.Line))
	want := cs.DesiredCol
	want = min(want, ll)
	cs.Cursor.ByteCol = want
}

func backspace(cs *CursorState, buf *buffer.Buffer) {
	pos := cs.Cursor
	if pos.Line == 0 && pos.ByteCol == 0 {
		return
	}
	var start buffer.Position
	if pos.ByteCol > 0 {
		start = buffer.Position{Line: pos.Line, ByteCol: pos.ByteCol - 1}
	} else {
		prevLen := len(buf.Line(pos.Line - 1))
		start = buffer.Position{Line: pos.Line - 1, ByteCol: prevLen}
	}
	c := buf.Apply(buffer.Edit{Range: buffer.Range{Start: start, End: pos}})
	cs.Cursor = c.AppliedRange.End
}

func deleteForward(cs *CursorState, buf *buffer.Buffer) {
	pos := cs.Cursor
	lineLen := len(buf.Line(pos.Line))
	var end buffer.Position
	if pos.ByteCol < lineLen {
		end = buffer.Position{Line: pos.Line, ByteCol: pos.ByteCol + 1}
	} else if pos.Line < buf.LineCount()-1 {
		end = buffer.Position{Line: pos.Line + 1, ByteCol: 0}
	} else {
		return
	}
	_ = buf.Apply(buffer.Edit{Range: buffer.Range{Start: pos, End: end}})
}

func insertNewline(cfg EditorCfg, cs *CursorState, buf *buffer.Buffer) {
	deleteCursorSelection(cs, buf)
	pos := cs.Cursor
	line := buf.Line(pos.Line)

	// Auto-indent: copy leading whitespace from current line.
	indent := leadingWhitespace(line)
	// Open-brace heuristic: add one indent level.
	if pos.ByteCol > 0 && pos.ByteCol <= len(line) &&
		line[pos.ByteCol-1] == '{' {
		indent = append(indent, indentUnit(cfg.Buffer.Props.IndentStyle)...)
	}

	newBytes := make([]byte, 0, 1+len(indent))
	newBytes = append(newBytes, '\n')
	newBytes = append(newBytes, indent...)
	c := buf.Apply(buffer.Edit{
		Range:    buffer.Range{Start: pos, End: pos},
		NewBytes: newBytes,
	})
	cs.Cursor = c.AppliedRange.End
	cs.ClearSelection()
}

// acceptChar reports whether r should be inserted into the buffer
// when received as a character event. Printable runes and tab pass;
// everything else (control chars, \n/\r, null) is rejected.
func acceptChar(r rune) bool {
	return r == '\t' || unicode.IsPrint(r)
}

func pageLines(frame *editorFrameData, viewportH float32) int {
	if frame.lineHeight <= 0 {
		return 1
	}
	n := int(viewportH / frame.lineHeight)
	n = max(n, 1)
	return n
}

// clampCursors clamps all cursors to valid coordinates within buf.
// Called from AmendLayout to recover gracefully from external
// buffer mutations.
func clampCursors(st *editorState, buf *buffer.Buffer) {
	for i := range st.Cursors {
		clampPos(&st.Cursors[i].Cursor, buf)
		clampPos(&st.Cursors[i].Anchor, buf)
	}
}

func clampPos(p *buffer.Position, buf *buffer.Buffer) {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= buf.LineCount() {
		p.Line = buf.LineCount() - 1
	}
	ll := len(buf.Line(p.Line))
	if p.ByteCol < 0 {
		p.ByteCol = 0
	}
	if p.ByteCol > ll {
		p.ByteCol = ll
	}
}

// clampScroll keeps ScrollY within [0, maxScroll]. Also sanitizes
// NaN — if ScrollY went NaN from bad input upstream, snap to 0.
// syncSearchObserver manages the search match observer lifecycle
// and recomputes matches when needed.
func syncSearchObserver(
	cfg EditorCfg, st *editorState, w *gui.Window,
	remove func(),
) func() {
	if st.Search.Active && len(st.Search.Query) > 0 &&
		needsRecompute(&st.Search) {
		recomputeMatches(st, cfg.Buffer)
	}
	if st.Search.Active && remove == nil {
		remove = cfg.Buffer.OnEdit(func(_ buffer.Change) {
			s := loadState(w, cfg.IDFocus)
			s.Search.matchesDirty = true
			storeState(w, cfg.IDFocus, s)
		})
	} else if !st.Search.Active && remove != nil {
		remove()
		remove = nil
	}
	return remove
}

// syncAutoCloseFilter manages the auto-close EditFilter lifecycle.
func syncAutoCloseFilter(
	cfg EditorCfg, remove func(),
) func() {
	pairs := cfg.AutoClosePairs
	if pairs == nil {
		pairs = DefaultAutoClosePairs
	}
	if len(pairs) > 0 && !cfg.ReadOnly && remove == nil {
		remove = cfg.Buffer.AddFilter(autoCloseFilter(pairs))
	} else if (len(pairs) == 0 || cfg.ReadOnly) && remove != nil {
		remove()
		remove = nil
	}
	return remove
}

// syncFoldObserver manages the fold-invalidation observer.
func syncFoldObserver(
	cfg EditorCfg, w *gui.Window, remove func(),
) func() {
	if cfg.EnableFolding && remove == nil {
		remove = cfg.Buffer.OnEdit(func(c buffer.Change) {
			s := loadState(w, cfg.IDFocus)
			if len(s.FoldedRanges) > 0 {
				s.FoldedRanges = invalidateFolds(
					s.FoldedRanges, c)
				storeState(w, cfg.IDFocus, s)
			}
		})
	} else if !cfg.EnableFolding && remove != nil {
		remove()
		remove = nil
	}
	return remove
}

// computeBracketMatch finds the matching bracket for the primary
// cursor and stores the result in frame.
func computeBracketMatch(
	cfg EditorCfg, st *editorState, frame *editorFrameData,
) {
	frame.bracketFound = false
	if cfg.Buffer == nil {
		return
	}
	if cfg.ShowBracketMatch && len(st.Cursors) > 0 {
		// Suppress the highlight when the scan was capped; treat
		// the match as unknown rather than drawing a misleading
		// pair.
		m, ok, capped := findMatchingBracket(
			cfg.Buffer, st.Cursors[0].Cursor)
		if ok && !capped {
			_, bpos := bracketAtCursor(
				cfg.Buffer, st.Cursors[0].Cursor)
			frame.bracketMatch = [2]buffer.Position{bpos, m}
			frame.bracketFound = true
		}
	}
}

// computeStickyScroll finds scope headers for the sticky scroll
// overlay and stores them in frame.
func computeStickyScroll(
	cfg EditorCfg, st *editorState,
	frame *editorFrameData, lh float32,
) {
	frame.stickyLines = nil
	stickyOn := resolveStickyScroll(
		cfg.StickyScroll, st.StickyScrollOverride)
	if !stickyOn || lh <= 0 || lh != lh { // NaN
		return
	}
	firstVis := max(int(st.ScrollY/lh), 0)
	stickyMax := cfg.StickyScrollMax
	if stickyMax <= 0 {
		stickyMax = defaultStickyMax
	}
	tw := text.DefaultTabWidth
	if st.Measurer != nil {
		tw = st.Measurer.TabWidth
	}
	var firstLogical int
	if frame.wrapActive && st.Measurer != nil {
		firstLogical, _ = globalVisualRowToLogical(
			cfg.Buffer, st.Measurer, frame.wrapWidth,
			st.FoldedRanges, firstVis)
	} else if len(st.FoldedRanges) > 0 {
		firstLogical = visibleToLogical(
			firstVis, st.FoldedRanges)
	} else {
		firstLogical = firstVis
	}
	frame.stickyLines = findScopeHeaders(
		cfg.Buffer, firstLogical, stickyMax, tw)
}

// clampScrollX keeps ScrollX in [0, maxScrollX]. Sanitizes NaN.
func clampScrollX(st *editorState, maxScrollX float32) {
	if st.ScrollX != st.ScrollX { // NaN
		st.ScrollX = 0
	}
	if st.ScrollX < 0 {
		st.ScrollX = 0
	}
	if st.ScrollX > maxScrollX {
		st.ScrollX = maxScrollX
	}
}

// computeMaxContentWidth measures the widest line in buf.
func computeMaxContentWidth(buf *buffer.Buffer, m *text.Measurer) float32 {
	if m == nil {
		return 0
	}
	var maxW float32
	for i := range buf.LineCount() {
		line := buf.Line(i)
		if len(line) == 0 {
			continue
		}
		if w := m.XForColumn(line, len(line)); w > maxW {
			maxW = w
		}
	}
	return maxW
}

func clampScroll(st *editorState, cfg EditorCfg, frame *editorFrameData, lh float32) {
	if st.ScrollY != st.ScrollY { // NaN
		st.ScrollY = 0
	}
	if lh <= 0 {
		st.ScrollY = 0
		return
	}
	total := frame.totalVisRows
	if total <= 0 {
		total = cfg.Buffer.LineCount()
	}
	maxScroll := float32(total)*lh - cfg.Height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if st.ScrollY > maxScroll {
		st.ScrollY = maxScroll
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}
}

func ensureCursorVisible(st *editorState, frame *editorFrameData, cfg EditorCfg) {
	viewportH := cfg.Height
	if !frame.valid || frame.lineHeight <= 0 {
		return
	}
	if viewportH != viewportH || viewportH <= 0 { // NaN or non-positive
		return
	}
	p := st.primary()
	lh := frame.lineHeight
	visRow := p.Cursor.Line
	if frame.wrapActive && st.Measurer != nil {
		visRow = globalLogicalToVisualRow(
			cfg.Buffer, st.Measurer,
			frame.wrapWidth, st.FoldedRanges, p.Cursor.Line)
		// Add sub-row offset for cursor within wrapped line.
		we := &wrapEntry{Line: p.Cursor.Line}
		lb := cfg.Buffer.Line(p.Cursor.Line)
		we.BreakCols = computeBreaks(lb, st.Measurer,
			frame.wrapWidth)
		visRow += wrapCursorVisualRow(we, p.Cursor.ByteCol)
	} else if len(st.FoldedRanges) > 0 {
		visRow = logicalToVisible(p.Cursor.Line, st.FoldedRanges)
	}
	cy := float32(visRow) * lh
	if cy < st.ScrollY {
		st.ScrollY = cy
	}
	if cy+lh > st.ScrollY+viewportH {
		st.ScrollY = cy + lh - viewportH
	}
	if st.ScrollY < 0 {
		st.ScrollY = 0
	}

	// Horizontal visibility (no-wrap only).
	if !frame.wrapActive && st.Measurer != nil {
		lb := cfg.Buffer.Line(p.Cursor.Line)
		cursorX := st.Measurer.XForColumn(lb, p.Cursor.ByteCol)
		textAreaW := cfg.Width - frame.gutterW - frame.padLeft
		if textAreaW > 0 {
			if cursorX < st.ScrollX {
				st.ScrollX = cursorX
			}
			if cursorX > st.ScrollX+textAreaW {
				st.ScrollX = cursorX - textAreaW
			}
		}
		maxScrollX := max(frame.maxContentW-textAreaW, 0)
		clampScrollX(st, maxScrollX)
	}
}

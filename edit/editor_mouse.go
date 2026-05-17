package edit

import (
	"time"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

const (
	doubleClickThresholdMs = 400
	animIDEditorDragScroll = "edit-drag-scroll"
	dragScrollIntervalMs   = 32
	dragScrollSpeedFactor  = 0.3
)

// hitTestPosition converts mouse event coordinates to a buffer
// Position, clamped to valid line/col. scrollY overrides the
// frame snapshot when >= 0; pass -1 to use frame.state.ScrollY.
func hitTestPosition(
	e *gui.Event,
	frame *editorFrameData,
	buf *buffer.Buffer,
	scrollY float32,
) buffer.Position {
	mx := e.MouseX - frame.gutterW - frame.padLeft + frame.state.ScrollX
	my := e.MouseY

	// Guard NaN / absurd values. Negative my is valid during
	// drag-above-viewport (triggers upward autoscroll).
	if mx != mx || mx < 0 {
		mx = 0
	}
	if my != my {
		my = 0
	}

	lh := frame.lineHeight
	if lh <= 0 || frame.state.Measurer == nil {
		return buffer.Position{}
	}

	if scrollY < 0 || scrollY != scrollY { // NaN guard
		scrollY = frame.state.ScrollY
	}
	visRow := max(int((my+scrollY)/lh), 0)
	folds := frame.state.FoldedRanges
	m := frame.state.Measurer

	var line, subRow int
	if frame.wrapActive && m != nil {
		line, subRow = globalVisualRowToLogical(
			buf, m, frame.wrapWidth, folds, visRow)
	} else if len(folds) > 0 {
		line = visibleToLogical(visRow, folds)
	} else {
		line = visRow
	}
	lc := buf.LineCount()
	if lc == 0 {
		return buffer.Position{}
	}
	line = min(line, lc-1)

	lineBytes := buf.Line(line)

	// For wrapped lines, adjust mx to account for sub-row.
	if frame.wrapActive && m != nil && subRow > 0 {
		breaks := computeBreaks(lineBytes, m, frame.wrapWidth)
		we := wrapEntry{BreakCols: breaks}
		subStart, subEnd := wrapSubRowRange(
			&we, len(lineBytes), subRow)
		col := m.ColumnForX(lineBytes[subStart:], mx)
		col += subStart
		if col > subEnd {
			col = subEnd
		}
		return buffer.Position{Line: line, ByteCol: col}
	}

	col := min(m.ColumnForX(lineBytes, mx), len(lineBytes))
	return buffer.Position{Line: line, ByteCol: col}
}

// hitTestLocal converts canvas-local coordinates to a buffer
// Position. Reuses a shared Event to avoid heap allocation.
// Pass scrollY >= 0 to override the frame snapshot, or -1 to
// use frame.state.ScrollY.
func hitTestLocal(
	mx, my, scrollY float32,
	frame *editorFrameData,
	buf *buffer.Buffer,
	scratch *gui.Event,
) buffer.Position {
	if buf == nil || scratch == nil {
		return buffer.Position{}
	}
	scratch.MouseX = mx
	scratch.MouseY = my
	return hitTestPosition(scratch, frame, buf, scrollY)
}

// editorOnClick returns the OnClick handler for the DrawCanvas.
// OnClick fires on mouse-down in go-gui.
func editorOnClick(
	cfg EditorCfg,
	frame *editorFrameData,
) func(*gui.Layout, *gui.Event, *gui.Window) {
	return func(layout *gui.Layout, e *gui.Event, w *gui.Window) {
		if !frame.valid {
			return
		}
		st := loadState(w, cfg.IDFocus)
		resetBlink(cfg, &st)

		// Capture canvas origin for MouseLock drag coord
		// translation. Guard NaN from layout.
		if layout.Shape != nil {
			ox, oy := layout.Shape.X, layout.Shape.Y
			if ox == ox { // not NaN
				frame.canvasOriginX = ox
			}
			if oy == oy {
				frame.canvasOriginY = oy
			}
		}

		// Gutter click: toggle fold.
		if cfg.EnableFolding && cfg.ShowLineNumbers &&
			e.MouseX < frame.gutterW {
			lh := frame.lineHeight
			if lh > 0 {
				visRow := int(
					(e.MouseY + st.ScrollY) / lh)
				var line int
				if frame.wrapActive && st.Measurer != nil {
					line, _ = globalVisualRowToLogical(
						cfg.Buffer, st.Measurer,
						frame.wrapWidth,
						st.FoldedRanges, visRow)
				} else if len(st.FoldedRanges) > 0 {
					line = visibleToLogical(
						visRow, st.FoldedRanges)
				} else {
					line = visRow
				}
				tw := 4
				if st.Measurer != nil {
					tw = st.Measurer.TabWidth
				}
				if line >= 0 &&
					line < cfg.Buffer.LineCount() &&
					(isFoldHeader(st.FoldedRanges, line) ||
						isFoldable(cfg.Buffer, line, tw)) {
					st.FoldedRanges = toggleFold(
						st.FoldedRanges,
						cfg.Buffer, line, tw)
					storeState(w, cfg.IDFocus, st)
					e.IsHandled = true
					return
				}
			}
		}

		// Vertical scrollbar click: jump to proportion or drag thumb.
		if scrollbarVisible(cfg.Scrollbar, frame.totalVisRows,
			frame.lineHeight, cfg.Height) &&
			e.MouseX >= cfg.Width-scrollbarWidth {
			handleScrollbarClick(cfg, frame, &st, e, w)
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}

		// Horizontal scrollbar click.
		textAreaW := cfg.Width - frame.gutterW - frame.padLeft
		if scrollbarHorizVisible(cfg.Scrollbar, frame.wrapActive,
			frame.maxContentW, textAreaW) &&
			e.MouseY >= cfg.Height-scrollbarWidth {
			handleHorizScrollbarClick(cfg, frame, &st, e, w)
			storeState(w, cfg.IDFocus, st)
			e.IsHandled = true
			return
		}

		pos := hitTestPosition(e, frame, cfg.Buffer, -1)
		now := time.Now().UnixMilli()

		// Click count detection. Use line-only match so minor
		// horizontal jitter doesn't break double/triple-click.
		if now-st.LastClickTime <= doubleClickThresholdMs &&
			st.LastClickPos.Line == pos.Line {
			st.ClickCount++
			if st.ClickCount > 3 {
				st.ClickCount = 3
			}
		} else {
			st.ClickCount = 1
		}
		st.LastClickTime = now
		st.LastClickPos = pos

		switch st.ClickCount {
		case 2: // double-click: word select
			collapseToPrimary(&st)
			p := st.primary()
			line := cfg.Buffer.Line(pos.Line)
			start, end := wordBoundsAtByte(line, pos.ByteCol)
			p.Anchor = buffer.Position{Line: pos.Line, ByteCol: start}
			p.Cursor = buffer.Position{Line: pos.Line, ByteCol: end}
			p.DesiredCol = p.Cursor.ByteCol
		case 3: // triple-click: line select
			collapseToPrimary(&st)
			p := st.primary()
			lineLen := len(cfg.Buffer.Line(pos.Line))
			p.Anchor = buffer.Position{Line: pos.Line, ByteCol: 0}
			p.Cursor = buffer.Position{Line: pos.Line, ByteCol: lineLen}
			p.DesiredCol = p.Cursor.ByteCol
		default: // single click
			if e.Modifiers.Has(gui.ModAlt) {
				// Alt-click adds a cursor.
				addCursor(&st, CursorState{
					Cursor:     pos,
					Anchor:     pos,
					DesiredCol: pos.ByteCol,
				})
			} else if e.Modifiers.Has(gui.ModShift) {
				// Shift-click extends primary selection,
				// drops secondary cursors.
				collapseToPrimary(&st)
				p := st.primary()
				p.Cursor = pos
				p.DesiredCol = pos.ByteCol
			} else {
				// Regular click: collapse to single cursor.
				collapseToPrimary(&st)
				p := st.primary()
				p.Cursor = pos
				p.Anchor = pos
				p.DesiredCol = pos.ByteCol
			}
		}

		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)

		// Start drag via MouseLock for single clicks
		// (not alt-click).
		if st.ClickCount == 1 && !e.Modifiers.Has(gui.ModAlt) {
			startDrag(cfg, frame, w)
		}

		e.IsHandled = true
	}
}

// startDrag initiates mouse-drag selection with autoscroll.
// Follows the go-gui text widget pattern: a repeating animation
// scrolls and extends the selection while the mouse is outside
// the viewport.
func startDrag(cfg EditorCfg, frame *editorFrameData, w *gui.Window) {
	var lastLocalX, lastLocalY float32
	var scratch gui.Event // reused to avoid per-tick alloc

	// dragUpdate does a single load → hit-test → clamp →
	// store cycle. scrollY < 0 means use the stored value.
	dragUpdate := func(lx, ly, scrollY float32, w *gui.Window) {
		st := loadState(w, cfg.IDFocus)
		resetBlink(cfg, &st)
		if scrollY >= 0 {
			st.ScrollY = scrollY
		}
		p := st.primary()
		p.Cursor = hitTestLocal(lx, ly, st.ScrollY,
			frame, cfg.Buffer, &scratch)
		p.DesiredCol = p.Cursor.ByteCol
		clampScroll(&st, cfg, frame, frame.lineHeight)
		ensureCursorVisible(&st, frame, cfg)
		storeState(w, cfg.IDFocus, st)
	}

	dragScrollCB := func(_ *gui.Animate, w *gui.Window) {
		lh := frame.lineHeight
		if lh <= 0 {
			return
		}
		var delta float32
		if lastLocalY < 0 {
			delta = lastLocalY * dragScrollSpeedFactor
		} else if lastLocalY > cfg.Height {
			delta = (lastLocalY - cfg.Height) * dragScrollSpeedFactor
		} else {
			w.AnimationRemove(animIDEditorDragScroll)
			return
		}
		st := loadState(w, cfg.IDFocus)
		newScroll := st.ScrollY + delta
		dragUpdate(lastLocalX, lastLocalY, newScroll, w)
	}

	w.MouseLock(gui.MouseLockCfg{
		MouseMove: func(_ *gui.Layout, e *gui.Event, w *gui.Window) {
			if !frame.valid {
				return
			}
			lx := e.MouseX - frame.canvasOriginX
			ly := e.MouseY - frame.canvasOriginY
			if lx != lx || ly != ly { // NaN guard
				return
			}
			lastLocalX = lx
			lastLocalY = ly
			dragUpdate(lastLocalX, lastLocalY, -1, w)

			outside := lastLocalY < 0 ||
				lastLocalY > cfg.Height
			if outside && !w.HasAnimation(
				animIDEditorDragScroll) {
				w.AnimationAdd(&gui.Animate{
					AnimID: animIDEditorDragScroll,
					Delay: dragScrollIntervalMs *
						time.Millisecond,
					Repeat:   true,
					Refresh:  gui.AnimationRefreshLayout,
					Callback: dragScrollCB,
				})
			} else if !outside {
				w.AnimationRemove(animIDEditorDragScroll)
			}
		},
		MouseUp: func(_ *gui.Layout, _ *gui.Event, w *gui.Window) {
			w.AnimationRemove(animIDEditorDragScroll)
			w.MouseUnlock()
		},
	})
}

// handleScrollbarClick handles a click in the scrollbar track area.
// Clicks on the thumb start a drag; clicks on the track jump-scroll
// to the clicked proportion.
func handleScrollbarClick(
	cfg EditorCfg,
	frame *editorFrameData,
	st *editorState,
	e *gui.Event,
	w *gui.Window,
) {
	lh := frame.lineHeight
	if lh <= 0 {
		return
	}
	trackH := cfg.Height
	thumbY, thumbH, hasThumb := scrollbarGeometry(
		frame.totalVisRows, lh, cfg.Height, st.ScrollY, trackH)

	clickY := e.MouseY

	if hasThumb && clickY >= thumbY && clickY < thumbY+thumbH {
		// Click on thumb: start drag.
		startScrollbarDrag(cfg, frame, w, clickY-thumbY)
	} else {
		// Click on track: jump to that proportion of maxScroll.
		contentH := float32(frame.totalVisRows) * lh
		maxScroll := contentH - cfg.Height
		if maxScroll > 0 {
			st.ScrollY = clickY / trackH * maxScroll
			clampScroll(st, cfg, frame, lh)
		}
	}
}

// handleHorizScrollbarClick handles a click in the horizontal
// scrollbar track. Clicks on the thumb start a drag; other clicks
// jump-scroll to the clicked proportion.
func handleHorizScrollbarClick(
	cfg EditorCfg,
	frame *editorFrameData,
	st *editorState,
	e *gui.Event,
	w *gui.Window,
) {
	textAreaW := cfg.Width - frame.gutterW - frame.padLeft
	if textAreaW <= 0 {
		return
	}
	contentW := frame.maxContentW + cursorScrollPad
	maxScrollX := max(contentW-textAreaW, 0)

	vertVisible := scrollbarVisible(cfg.Scrollbar,
		frame.totalVisRows, frame.lineHeight, cfg.Height)
	trackX := frame.gutterW
	trackW := cfg.Width - trackX
	if vertVisible {
		trackW -= scrollbarWidth
	}
	if trackW <= 0 {
		return
	}

	thumbY, thumbH, hasThumb := scrollbarGeometry(
		int(contentW), 1, textAreaW, st.ScrollX, trackW)
	clickX := e.MouseX - trackX

	if hasThumb && clickX >= thumbY && clickX < thumbY+thumbH {
		startHorizScrollbarDrag(cfg, frame, w, clickX-thumbY)
	} else if maxScrollX > 0 {
		st.ScrollX = clickX / trackW * maxScrollX
		clampScrollX(st, maxScrollX)
	}
}

// startHorizScrollbarDrag initiates horizontal thumb drag via
// MouseLock. dragOffset is click X relative to the thumb left edge.
func startHorizScrollbarDrag(
	cfg EditorCfg,
	frame *editorFrameData,
	w *gui.Window,
	dragOffset float32,
) {
	w.MouseLock(gui.MouseLockCfg{
		MouseMove: func(_ *gui.Layout, e *gui.Event, w *gui.Window) {
			if !frame.valid {
				return
			}
			lx := e.MouseX - frame.canvasOriginX - frame.gutterW
			if lx != lx { // NaN
				return
			}
			textAreaW := cfg.Width - frame.gutterW - frame.padLeft
			if textAreaW <= 0 {
				return
			}
			contentW := frame.maxContentW + cursorScrollPad
			maxScrollX := max(contentW-textAreaW, 0)
			vertVisible := scrollbarVisible(cfg.Scrollbar,
				frame.totalVisRows, frame.lineHeight, cfg.Height)
			trackW := cfg.Width - frame.gutterW
			if vertVisible {
				trackW -= scrollbarWidth
			}
			_, thumbH, hasThumb := scrollbarGeometry(
				int(contentW), 1, textAreaW, 0, trackW)
			if !hasThumb || trackW <= 0 {
				return
			}
			thumbRange := trackW - thumbH
			if thumbRange <= 0 || maxScrollX <= 0 {
				return
			}
			st := loadState(w, cfg.IDFocus)
			st.ScrollX = (lx - dragOffset) / thumbRange * maxScrollX
			clampScrollX(&st, maxScrollX)
			storeState(w, cfg.IDFocus, st)
		},
		MouseUp: func(_ *gui.Layout, _ *gui.Event, w *gui.Window) {
			w.MouseUnlock()
		},
	})
}

// startScrollbarDrag initiates thumb drag via MouseLock. dragOffset
// is the click Y position relative to the top of the thumb.
func startScrollbarDrag(
	cfg EditorCfg,
	frame *editorFrameData,
	w *gui.Window,
	dragOffset float32,
) {
	w.MouseLock(gui.MouseLockCfg{
		MouseMove: func(_ *gui.Layout, e *gui.Event, w *gui.Window) {
			if !frame.valid {
				return
			}
			// MouseLock delivers window coords; convert to canvas-local.
			ly := e.MouseY - frame.canvasOriginY
			if ly != ly { // NaN
				return
			}
			lh := frame.lineHeight
			if lh <= 0 {
				return
			}
			trackH := cfg.Height
			_, thumbH, hasThumb := scrollbarGeometry(
				frame.totalVisRows, lh, cfg.Height, 0, trackH)
			if !hasThumb {
				return
			}
			thumbRange := trackH - thumbH
			contentH := float32(frame.totalVisRows) * lh
			maxScroll := contentH - cfg.Height
			if thumbRange <= 0 || maxScroll <= 0 {
				return
			}
			st := loadState(w, cfg.IDFocus)
			st.ScrollY = (ly - dragOffset) / thumbRange * maxScroll
			clampScroll(&st, cfg, frame, lh)
			storeState(w, cfg.IDFocus, st)
		},
		MouseUp: func(_ *gui.Layout, _ *gui.Event, w *gui.Window) {
			w.MouseUnlock()
		},
	})
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
			maxScrollX := max(frame.maxContentW+cursorScrollPad-textAreaW, 0)
			clampScrollX(&st, maxScrollX)
		}
		storeState(w, cfg.IDFocus, st)
		e.IsHandled = true
	}
}

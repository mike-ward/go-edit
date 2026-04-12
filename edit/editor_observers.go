package edit

import (
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// syncSearchObserver manages the search match observer lifecycle
// and recomputes matches when needed.
func syncSearchObserver(
	cfg EditorCfg, st *editorState, w *gui.Window,
	remove func(),
) func() {
	if st.Search.Active && len(st.Search.Query) > 0 &&
		st.Search.needsRecompute() {
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
		// Suppress the highlight when the scan was capped.
		bpos, m, ok, capped := findMatchingBracket(
			cfg.Buffer, st.Cursors[0].Cursor)
		if ok && !capped {
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
	stickyOn := resolveBoolOverride(
		cfg.StickyScroll, st.StickyScrollOverride)
	if !stickyOn || lh <= 0 || lh != lh { // NaN
		return
	}
	sy := st.ScrollY
	if sy != sy || sy < 0 { // NaN or negative
		sy = 0
	}
	firstVis := max(int(sy/lh), 0)
	stickyMax := cfg.StickyScrollMax
	if stickyMax <= 0 {
		stickyMax = defaultStickyMax
	}
	tw := text.DefaultTabWidth
	if st.Measurer != nil {
		tw = st.Measurer.TabWidth
	}
	hasFolds := cfg.EnableFolding && len(st.FoldedRanges) > 0
	firstLogical, _ := visRowToStartLine(
		cfg.Buffer, st.Measurer, frame, st.FoldedRanges,
		hasFolds, frame.wrapActive, firstVis)
	frame.stickyLines = findScopeHeaders(
		cfg.Buffer, firstLogical, stickyMax, tw)
}

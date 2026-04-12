package edit

import "math"

// floatBitsStable returns a hash-stable uint64 representation of
// f. NaN has many bit patterns and +0/-0 compare equal but have
// different bits; canonicalize both so the draw-version fold
// doesn't thrash on upstream guard failures.
func floatBitsStable(f float32) uint64 {
	if f != f { // NaN
		return 0x7fc00000 // canonical quiet NaN
	}
	if f == 0 { // merges +0 and -0
		return 0
	}
	return uint64(math.Float32bits(f))
}

// computeDrawVersion folds every frame-visible input into a single
// uint64. go-gui's DrawCanvas cache is keyed by (ID, Version,
// TessWidth, TessHeight); when the fold matches the prior frame
// OnDraw is skipped and the cached tessellation is re-used.
//
// Inputs are hashed via FNV-1a. Float fields are converted through
// floatBitsStable so NaN and ±0 fold identically. Cursor blink
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
	v = fold(v, floatBitsStable(st.ScrollY))
	v = fold(v, floatBitsStable(st.ScrollX))
	v = fold(v, cursorFoldHash(st.Cursors))
	v = fold(v, uint64(len(st.FoldedRanges))<<32|
		uint64(len(st.Cursors)))
	// Fold search state. The find bar is drawn inside the main
	// editor canvas (not a separate overlay), so every visually
	// observable field of searchState must flow into the hash or
	// the DrawCanvas cache will replay stale pixels when only
	// find-bar state changed.
	var searchFlags uint64
	if st.Search.Active {
		searchFlags |= 1
	}
	if st.Search.CaseSensitive {
		searchFlags |= 1 << 1
	}
	if st.Search.IsRegex {
		searchFlags |= 1 << 2
	}
	if st.Search.InSelection {
		searchFlags |= 1 << 3
	}
	if st.Search.ShowReplace {
		searchFlags |= 1 << 4
	}
	if st.Search.FocusReplace {
		searchFlags |= 1 << 5
	}
	v = fold(v, searchFlags)
	v = fold(v, uint64(len(st.Search.Query)))
	v = fold(v, uint64(len(st.Search.ReplaceText)))
	v = fold(v, uint64(st.Search.FieldCursor))
	v = fold(v, uint64(st.Search.CurrentMatch))
	v = fold(v, uint64(len(st.Search.Matches)))
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
	v = fold(v, floatBitsStable(frame.wrapWidth))
	v = fold(v, floatBitsStable(st.HelpScrollY))
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

package edit

import (
	"slices"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
)

// maxLineRowsCacheLines caps the persistent per-line cache size.
// Above this count (≈ 8 MiB of ints at 1M lines), updateVisRowsCache
// falls back to a per-frame full walk so adversarial all-newline
// inputs don't produce a 256 MiB int slice. Exposed as a var so
// tests can lower the cap and exercise the fallback without
// constructing a 1M-line buffer.
var maxLineRowsCacheLines = 1 << 20 // 1M lines → ~8 MiB cache

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
	// Hardening: an all-newline 32 MiB buffer has ~32M lines;
	// the lineRowsCache allocation would be ~256 MiB. Fall back
	// to the full-walk (no persistent cache) path when line
	// count exceeds maxLineRowsCacheLines — the full walk is
	// O(lines) per frame but avoids the big slice.
	if wrapActive && st.Measurer != nil &&
		total <= maxLineRowsCacheLines {
		frame.totalVisRows, frame.lineRowsCache =
			buildLineRowsCache(cfg.Buffer, st.Measurer,
				frame.wrapWidth, st.FoldedRanges,
				frame.lineRowsCache)
	} else if wrapActive && st.Measurer != nil {
		frame.lineRowsCache = nil
		frame.totalVisRows = totalVisualRowsForBuffer(
			cfg.Buffer, st.Measurer,
			frame.wrapWidth, st.FoldedRanges)
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
	if buf == nil || m == nil {
		return 0, nil
	}
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

	// Compute new per-line rows into the frame-scoped scratch
	// slice. Reusing across edits keeps steady-state typing
	// allocation-free on the observer path.
	newCount := endLineNew - startLine + 1
	oldCount := endLineOld - startLine + 1
	scratch := frame.visRowsDeltaScratch[:0]
	if cap(scratch) < newCount {
		scratch = make([]int, 0, newCount)
	}
	newSum := 0
	for i := range newCount {
		line := startLine + i
		rows := wrapLineVisualRowCount(buf.Line(line), m, ww)
		scratch = append(scratch, rows)
		newSum += rows
	}
	frame.visRowsDeltaScratch = scratch

	// Splice scratch in place of the old range.
	if newCount == oldCount {
		copy(frame.lineRowsCache[startLine:], scratch)
	} else {
		frame.lineRowsCache = slices.Replace(
			frame.lineRowsCache, startLine, endLineOld+1, scratch...)
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

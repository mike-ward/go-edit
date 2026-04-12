package edit

import (
	"bytes"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// maxMatches caps the number of highlighted matches to avoid
// unbounded memory and slow draw for pathological queries.
const maxMatches = 10_000

// maxFieldLen caps search/replace field length to prevent
// unbounded memory from programmatic input.
const maxFieldLen = 10_000

// searchState holds the find/replace bar state, persisted in
// editorState across frames.
type searchState struct {
	Active         bool
	Query          string
	ReplaceText    string
	IsRegex        bool
	CaseSensitive  bool
	InSelection    bool         // restrict matches to SelectionScope
	SelectionScope buffer.Range // locked when InSelection enabled
	ShowReplace    bool         // show replace input row
	FocusReplace   bool         // false=query field, true=replace field
	FieldCursor    int          // byte offset in active field

	Matches      []buffer.Range // sorted by position; recomputed on change
	CurrentMatch int            // index into Matches; -1 = none

	// cached compiled regex; recomputed when query/flags change
	compiled     *regexp.Regexp
	lastQuery    string // query at last recompute
	lastFlags    uint8  // packed CaseSensitive|IsRegex at last recompute
	matchesDirty bool   // set true when buffer edited while search active
}

// packFlags packs boolean search flags into a uint8 for dirty
// comparison.
func (ss *searchState) packFlags() uint8 {
	var f uint8
	if ss.CaseSensitive {
		f |= 1
	}
	if ss.IsRegex {
		f |= 2
	}
	if ss.InSelection {
		f |= 4
	}
	return f
}

// activeField returns a pointer to the currently focused field.
func (ss *searchState) activeField() *string {
	if ss.FocusReplace && ss.ShowReplace {
		return &ss.ReplaceText
	}
	return &ss.Query
}

// clampFieldCursor ensures FieldCursor is within [0, len(field)].
func (ss *searchState) clampFieldCursor() {
	field := ss.activeField()
	if ss.FieldCursor < 0 {
		ss.FieldCursor = 0
	}
	if ss.FieldCursor > len(*field) {
		ss.FieldCursor = len(*field)
	}
}

// ---------- core search functions ----------

// findAllMatches returns all match ranges in buf for the given query.
// When scope is non-empty, only matches within that range are returned.
// Returns nil on empty query or invalid regex. Cap at maxMatches.
func findAllMatches(
	buf *buffer.Buffer,
	query string,
	caseSensitive, isRegex bool,
	scope buffer.Range,
) ([]buffer.Range, *regexp.Regexp) {
	if len(query) == 0 {
		return nil, nil
	}
	var matches []buffer.Range
	var re *regexp.Regexp
	if isRegex {
		matches, re = findAllRegex(buf, query, caseSensitive)
	} else {
		matches = findAllLiteral(buf, query, caseSensitive)
	}
	if !scope.Empty() && len(matches) > 0 {
		matches = filterToScope(matches, scope)
	}
	return matches, re
}

// filterToScope returns only matches fully contained within scope.
// Returns the original slice unchanged if all matches are in scope.
func filterToScope(matches []buffer.Range, scope buffer.Range) []buffer.Range {
	// Quick check: if first and last match are in scope, all are.
	inScope := func(m buffer.Range) bool {
		return !m.Start.Before(scope.Start) && !m.End.After(scope.End)
	}
	if inScope(matches[0]) && inScope(matches[len(matches)-1]) {
		return matches
	}
	var out []buffer.Range
	for _, m := range matches {
		if inScope(m) {
			out = append(out, m)
		}
	}
	return out
}

func findAllLiteral(
	buf *buffer.Buffer,
	query string,
	caseSensitive bool,
) []buffer.Range {
	needle := []byte(query)
	var lowerNeedle []byte
	if !caseSensitive {
		lowerNeedle = bytes.ToLower(needle)
	}

	// Reusable scratch buffer for case-insensitive lowering to
	// avoid per-line allocation.
	var lowerBuf []byte

	var matches []buffer.Range
	total := buf.LineCount()
	for li := range total {
		line := buf.Line(li)
		searchLine := line
		if !caseSensitive {
			searchLine = toLowerReuse(line, &lowerBuf)
		}
		n := lowerNeedle
		if caseSensitive {
			n = needle
		}
		off := 0
		for {
			idx := bytes.Index(searchLine[off:], n)
			if idx < 0 {
				break
			}
			col := off + idx
			matches = append(matches, buffer.Range{
				Start: buffer.Position{Line: li, ByteCol: col},
				End:   buffer.Position{Line: li, ByteCol: col + len(needle)},
			})
			if len(matches) >= maxMatches {
				return matches
			}
			off = col + max(1, len(needle))
			if off >= len(searchLine) {
				break
			}
		}
	}
	return matches
}

// toLowerReuse lowercases src into a reusable buffer to avoid
// per-call allocation. ASCII fast path copies in-place; non-ASCII
// falls back to bytes.ToLower (which must allocate due to
// potential UTF-8 length changes).
func toLowerReuse(src []byte, buf *[]byte) []byte {
	if len(src) == 0 {
		return src
	}
	if !text.IsASCII(src) {
		return bytes.ToLower(src)
	}
	if cap(*buf) < len(src) {
		*buf = make([]byte, len(src))
	}
	*buf = (*buf)[:len(src)]
	for i, b := range src {
		if b >= 'A' && b <= 'Z' {
			(*buf)[i] = b + 32
		} else {
			(*buf)[i] = b
		}
	}
	return *buf
}

func findAllRegex(
	buf *buffer.Buffer,
	query string,
	caseSensitive bool,
) ([]buffer.Range, *regexp.Regexp) {
	pattern := query
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, nil
	}
	var matches []buffer.Range
	total := buf.LineCount()
	for li := range total {
		line := buf.Line(li)
		locs := re.FindAllIndex(line, -1)
		for _, loc := range locs {
			matches = append(matches, buffer.Range{
				Start: buffer.Position{Line: li, ByteCol: loc[0]},
				End:   buffer.Position{Line: li, ByteCol: loc[1]},
			})
			if len(matches) >= maxMatches {
				return matches, re
			}
		}
	}
	return matches, re
}

// recomputeMatches rebuilds the match list when query or flags
// changed or the buffer was edited. Updates CurrentMatch to the
// nearest match at or after the primary cursor.
func recomputeMatches(st *editorState, buf *buffer.Buffer) {
	ss := &st.Search
	var scope buffer.Range
	if ss.InSelection {
		scope = ss.SelectionScope
	}
	ss.Matches, ss.compiled = findAllMatches(
		buf, ss.Query, ss.CaseSensitive, ss.IsRegex, scope,
	)
	ss.lastQuery = ss.Query
	ss.lastFlags = ss.packFlags()
	ss.matchesDirty = false

	if len(ss.Matches) == 0 {
		ss.CurrentMatch = -1
		return
	}

	// Find nearest match at or after primary cursor.
	cursor := st.primary().Cursor
	idx := sort.Search(len(ss.Matches), func(i int) bool {
		m := ss.Matches[i].Start
		if m.Line != cursor.Line {
			return m.Line > cursor.Line
		}
		return m.ByteCol >= cursor.ByteCol
	})
	if idx >= len(ss.Matches) {
		idx = 0 // wrap
	}
	ss.CurrentMatch = idx
}

// needsRecompute reports whether matches need rebuilding.
func (ss *searchState) needsRecompute() bool {
	if ss.matchesDirty {
		return true
	}
	if ss.Query != ss.lastQuery {
		return true
	}
	if ss.packFlags() != ss.lastFlags {
		return true
	}
	return false
}

// matchesForLine returns the sub-slice of sorted matches that touch
// the given line. Returns a slice into the original matches array
// (no allocation). Uses binary search for the first candidate.
func matchesForLine(matches []buffer.Range, line int) []buffer.Range {
	if len(matches) == 0 {
		return nil
	}
	// Binary search for first match whose End.Line >= line.
	lo := sort.Search(len(matches), func(i int) bool {
		return matches[i].End.Line >= line
	})
	if lo >= len(matches) {
		return nil
	}
	// Scan forward for contiguous matches on this line.
	hi := lo
	for hi < len(matches) && matches[hi].Start.Line <= line {
		hi++
	}
	if hi == lo {
		return nil
	}
	return matches[lo:hi]
}

// ---------- replace ----------

// prepareReplace ensures matches are current. Returns false if
// replace should be skipped (read-only or no matches).
func prepareReplace(cfg EditorCfg, st *editorState, buf *buffer.Buffer) bool {
	if cfg.ReadOnly {
		return false
	}
	ss := &st.Search
	if ss.needsRecompute() || (len(ss.Matches) == 0 && len(ss.Query) > 0) {
		recomputeMatches(st, buf)
	}
	return len(ss.Matches) > 0
}

// replaceCurrentMatch replaces the match at CurrentMatch and
// advances to the next match.
func replaceCurrentMatch(
	cfg EditorCfg,
	st *editorState,
	buf *buffer.Buffer,
) {
	if !prepareReplace(cfg, st, buf) {
		return
	}
	ss := &st.Search
	if ss.CurrentMatch < 0 || ss.CurrentMatch >= len(ss.Matches) {
		return
	}

	m := ss.Matches[ss.CurrentMatch]
	replacement := replaceBytes(ss, buf, m)

	buf.SetUndoCursorState(buildUndoCursorState(st))
	buf.BeginGroup()
	c := buf.Apply(buffer.Edit{
		Range:    m,
		NewBytes: replacement,
	})
	buf.EndGroup()

	// Move cursor to end of replacement.
	st.primary().Cursor = c.AppliedRange.End
	st.primary().ClearSelection()

	recomputeMatches(st, buf)
}

// replaceAllMatches replaces every match in a single undo group.
// Applies in reverse position order to avoid invalidation.
func replaceAllMatches(
	cfg EditorCfg,
	st *editorState,
	buf *buffer.Buffer,
) {
	if !prepareReplace(cfg, st, buf) {
		return
	}
	ss := &st.Search

	buf.SetUndoCursorState(buildUndoCursorState(st))
	buf.BeginGroup()

	// Reverse order to preserve positions.
	for i := len(ss.Matches) - 1; i >= 0; i-- {
		m := ss.Matches[i]
		replacement := replaceBytes(ss, buf, m)
		buf.Apply(buffer.Edit{
			Range:    m,
			NewBytes: replacement,
		})
	}

	buf.EndGroup()
	recomputeMatches(st, buf)
}

// replaceBytes computes the replacement bytes for a match. For
// regex mode, expands $1 etc. via the compiled pattern.
func replaceBytes(
	ss *searchState,
	buf *buffer.Buffer,
	m buffer.Range,
) []byte {
	replText := []byte(ss.ReplaceText)
	if !ss.IsRegex || ss.compiled == nil {
		return replText
	}
	src := []byte(buf.TextInRange(m))
	submatch := ss.compiled.FindSubmatchIndex(src)
	if submatch == nil {
		return replText
	}
	return ss.compiled.Expand(nil, replText, src, submatch)
}

// ---------- input routing ----------

// handleSearchKey processes key events when the find bar is active.
// Returns true if the key was consumed.
func handleSearchKey(
	cfg EditorCfg,
	st *editorState,
	buf *buffer.Buffer,
	e *gui.Event,
) bool {
	ss := &st.Search
	ss.clampFieldCursor()
	// Ctrl+Enter / Cmd+Enter → replace all (before regular Enter).
	if e.KeyCode == gui.KeyEnter && ss.ShowReplace &&
		(e.Modifiers.Has(gui.ModCtrl) || e.Modifiers.Has(gui.ModSuper)) {
		replaceAllMatches(cfg, st, buf)
		return true
	}

	switch e.KeyCode {
	case gui.KeyEscape:
		ss.Active = false
		ss.Matches = nil
		ss.CurrentMatch = -1
		ss.InSelection = false
		return true

	case gui.KeyEnter:
		if e.Modifiers.Has(gui.ModShift) {
			navigateMatch(st, -1)
		} else {
			navigateMatch(st, +1)
		}
		return true

	case gui.KeyBackspace:
		field := ss.activeField()
		if ss.FieldCursor > 0 && len(*field) > 0 {
			_, sz := utf8.DecodeLastRuneInString((*field)[:ss.FieldCursor])
			*field = spliceField(*field, ss.FieldCursor-sz, ss.FieldCursor, "")
			ss.FieldCursor -= sz
			if !ss.FocusReplace {
				recomputeMatches(st, buf)
			}
		}
		return true

	case gui.KeyDelete:
		field := ss.activeField()
		if ss.FieldCursor < len(*field) {
			_, sz := utf8.DecodeRuneInString((*field)[ss.FieldCursor:])
			*field = spliceField(*field, ss.FieldCursor, ss.FieldCursor+sz, "")
			if !ss.FocusReplace {
				recomputeMatches(st, buf)
			}
		}
		return true

	case gui.KeyLeft:
		if ss.FieldCursor > 0 {
			field := ss.activeField()
			_, sz := utf8.DecodeLastRuneInString((*field)[:ss.FieldCursor])
			ss.FieldCursor -= sz
		}
		return true

	case gui.KeyRight:
		field := ss.activeField()
		if ss.FieldCursor < len(*field) {
			_, sz := utf8.DecodeRuneInString((*field)[ss.FieldCursor:])
			ss.FieldCursor += sz
		}
		return true

	case gui.KeyHome:
		ss.FieldCursor = 0
		return true

	case gui.KeyEnd:
		ss.FieldCursor = len(*ss.activeField())
		return true

	case gui.KeyTab:
		if ss.ShowReplace {
			ss.FocusReplace = !ss.FocusReplace
			ss.FieldCursor = len(*ss.activeField())
		}
		return true

	default:
		return handleSearchModKey(cfg, st, buf, e)
	}
}

// handleSearchModKey handles modifier+letter keys in the find bar
// (Ctrl+F, Ctrl+H, Alt+R, Alt+C, Alt+S, Ctrl+R).
func handleSearchModKey(
	cfg EditorCfg,
	st *editorState,
	buf *buffer.Buffer,
	e *gui.Event,
) bool {
	ss := &st.Search
	ctrlOrSuper := e.Modifiers.Has(gui.ModCtrl) ||
		e.Modifiers.Has(gui.ModSuper)

	switch e.KeyCode {
	case gui.KeyF:
		if ctrlOrSuper {
			ss.FocusReplace = false
			ss.FieldCursor = len(ss.Query)
			return true
		}

	case gui.KeyH:
		if ctrlOrSuper {
			ss.ShowReplace = !ss.ShowReplace
			if ss.ShowReplace {
				ss.FocusReplace = true
				ss.FieldCursor = len(ss.ReplaceText)
			} else {
				ss.FocusReplace = false
				ss.FieldCursor = len(ss.Query)
			}
			return true
		}

	case gui.KeyR:
		if e.Modifiers == gui.ModAlt {
			ss.IsRegex = !ss.IsRegex
			recomputeMatches(st, buf)
			return true
		}
		if ss.ShowReplace && ctrlOrSuper &&
			!e.Modifiers.Has(gui.ModShift) {
			replaceCurrentMatch(cfg, st, buf)
			return true
		}

	case gui.KeyC:
		if e.Modifiers == gui.ModAlt {
			ss.CaseSensitive = !ss.CaseSensitive
			recomputeMatches(st, buf)
			return true
		}

	case gui.KeyS:
		if e.Modifiers == gui.ModAlt {
			ss.InSelection = !ss.InSelection
			if ss.InSelection {
				p := st.primary()
				if p.HasSelection() {
					ss.SelectionScope = p.SelectionRange()
				} else {
					ss.InSelection = false
				}
			}
			recomputeMatches(st, buf)
			return true
		}
	}

	return false
}

// handleSearchChar inserts a character into the active search field.
func handleSearchChar(st *editorState, buf *buffer.Buffer, r rune) {
	var rb [4]byte
	n := utf8.EncodeRune(rb[:], r)
	handleSearchInsert(st, buf, string(rb[:n]))
}

// handleSearchString inserts a string (e.g. IME commit) into
// the active search field.
func handleSearchString(st *editorState, buf *buffer.Buffer, s string) {
	handleSearchInsert(st, buf, s)
}

func handleSearchInsert(st *editorState, buf *buffer.Buffer, s string) {
	ss := &st.Search
	ss.clampFieldCursor()
	field := ss.activeField()
	if len(*field)+len(s) > maxFieldLen {
		return
	}
	*field = spliceField(*field, ss.FieldCursor, ss.FieldCursor, s)
	ss.FieldCursor += len(s)
	if !ss.FocusReplace {
		recomputeMatches(st, buf)
	}
}

// spliceField replaces s[lo:hi] with repl in a single allocation.
// Out-of-bounds lo/hi are clamped.
func spliceField(s string, lo, hi int, repl string) string {
	lo = max(lo, 0)
	hi = max(hi, lo)
	if lo > len(s) {
		lo = len(s)
	}
	if hi > len(s) {
		hi = len(s)
	}
	var b strings.Builder
	b.Grow(len(s) - (hi - lo) + len(repl))
	b.WriteString(s[:lo])
	b.WriteString(repl)
	b.WriteString(s[hi:])
	return b.String()
}

// navigateMatch moves CurrentMatch by delta (+1 = next, -1 = prev)
// and positions the primary cursor at the match.
func navigateMatch(st *editorState, delta int) {
	ss := &st.Search
	if len(ss.Matches) == 0 {
		return
	}
	ss.CurrentMatch += delta
	if ss.CurrentMatch >= len(ss.Matches) {
		ss.CurrentMatch = 0
	}
	if ss.CurrentMatch < 0 {
		ss.CurrentMatch = len(ss.Matches) - 1
	}
	m := ss.Matches[ss.CurrentMatch]
	// Select the match.
	p := st.primary()
	p.Anchor = m.Start
	p.Cursor = m.End
	p.DesiredCol = m.End.ByteCol
	// Collapse to primary cursor when navigating.
	collapseToPrimary(st)
}

// openFindBar opens the find bar, optionally populating query from
// the current selection.
func openFindBar(st *editorState, buf *buffer.Buffer, showReplace bool) {
	ss := &st.Search
	ss.Active = true
	ss.ShowReplace = showReplace

	// Populate query from selection if any.
	p := st.primary()
	if p.HasSelection() {
		selRange := p.SelectionRange()
		sel := buf.TextInRange(selRange)
		if strings.ContainsRune(sel, '\n') {
			// Multi-line selection: auto-enable find-in-selection.
			ss.InSelection = true
			ss.SelectionScope = selRange
		} else {
			// Single-line selection: populate query.
			if len(sel) > maxFieldLen {
				sel = sel[:maxFieldLen]
			}
			ss.Query = sel
			ss.InSelection = false
		}
	}

	ss.FocusReplace = false // always start in query field
	ss.FieldCursor = len(ss.Query)
	recomputeMatches(st, buf)
}

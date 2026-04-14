package highlight

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/mike-ward/go-edit/edit/buffer"
)

// Token is a pre-computed styled span within a line.
type Token struct {
	Start  int    // byte offset in line
	End    int    // byte offset in line
	Fg     uint32 // 0xRRGGBBAA
	Bold   bool
	Italic bool
}

// maxInt is the "pristine" sentinel for dirtyLineStart.
const maxInt = int(^uint(0) >> 1)

// maxTokensPerLine caps per-line token storage to bound memory
// on pathological inputs (e.g. minified files).
const maxTokensPerLine = 4096

// Highlighter is a DecorationProvider backed by chroma. It
// tokenizes the buffer on demand, caches per-line tokens, and
// re-tokenizes incrementally from the earliest dirty line on
// each edit.
type Highlighter struct {
	mu    sync.Mutex
	lexer chroma.Lexer
	style *chroma.Style
	buf   *buffer.Buffer
	// tokens is the per-line token cache; tokens[i] is the
	// styled spans on logical line i (nil for untouched lines
	// before the first tokenize).
	tokens [][]Token
	// lineContinues[i] is true iff line i ended while chroma
	// was still inside a multi-line token (string/comment/etc).
	// Populated by retokenizeFrom. Used by prevLineIsContinuation
	// to back off the incremental restart point off to safe
	// ground without guessing from token colors.
	lineContinues []bool
	// primed becomes true after the first successful retokenize;
	// before that, dirtyLineStart is ignored and Decorate runs a
	// full walk.
	primed bool
	// dirtyLineStart is the lowest logical line index invalidated
	// since the last successful retokenize. maxInt = clean.
	// Decorate re-lexes from here to the end of buffer and
	// splices new tokens into the cache, preserving the pristine
	// prefix.
	dirtyLineStart int
	// lastViewport caches the most recent Decorate viewport so
	// retokenizeFrom can cap work to viewport + lookahead.
	lastViewport   buffer.Viewport
	invalidate     func() // RequestRedraw thunk; may be nil
	removeEdit     func() // remove handle for OnEdit observer
	overrideColors map[chroma.TokenType]uint32
}

// New creates a Highlighter for buf. Language is autodetected
// from buf.Props.FilePath if language is empty. If style is nil,
// "monokai" is used. Returns nil if no lexer matches.
func New(buf *buffer.Buffer, language string, style *chroma.Style) *Highlighter {
	var lex chroma.Lexer
	if language != "" {
		lex = lexers.Get(language)
	}
	if lex == nil && buf.Props.FilePath != "" {
		lex = lexers.Match(filepath.Base(buf.Props.FilePath))
	}
	if lex == nil {
		return nil
	}
	if style == nil {
		style = styles.Get("monokai")
	}

	h := &Highlighter{
		lexer:          lex,
		style:          style,
		buf:            buf,
		dirtyLineStart: maxInt,
	}
	h.removeEdit = buf.OnEdit(func(c buffer.Change) {
		h.mu.Lock()
		start := c.Applied.Range.Start.Line
		if start < h.dirtyLineStart {
			h.dirtyLineStart = start
		}
		inv := h.invalidate
		h.mu.Unlock()
		if inv != nil {
			inv()
		}
	})
	return h
}

// SetInvalidateFunc stores the RequestRedraw thunk.
func (h *Highlighter) SetInvalidateFunc(fn func()) {
	h.mu.Lock()
	h.invalidate = fn
	h.mu.Unlock()
}

// SetTokenOverrides installs per-token-type color overrides.
// These take priority over the chroma style. Pass nil to clear.
func (h *Highlighter) SetTokenOverrides(m map[chroma.TokenType]uint32) {
	h.mu.Lock()
	h.overrideColors = m
	// Overrides reshuffle token colors everywhere; the per-line
	// token cache still has the right ranges but the wrong
	// colors. Force a full retokenize next Decorate.
	h.primed = false
	h.dirtyLineStart = 0
	h.mu.Unlock()
}

// Close removes the edit observer.
func (h *Highlighter) Close() {
	if h.removeEdit != nil {
		h.removeEdit()
	}
}

// Decorate implements buffer.DecorationProvider. It appends
// DecoToken decorations for the visible viewport to out and
// returns the extended slice. Invalid (or never-tokenized)
// lines are tokenized synchronously.
func (h *Highlighter) Decorate(
	vp buffer.Viewport, out []buffer.Decoration,
) []buffer.Decoration {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastViewport = vp

	if !h.primed {
		h.retokenizeFrom(0)
		h.primed = true
		h.dirtyLineStart = maxInt
	} else if h.dirtyLineStart < maxInt {
		h.retokenizeFrom(h.dirtyLineStart)
	}

	// Clamp viewport to valid range.
	if vp.FirstLine < 0 {
		vp.FirstLine = 0
	}
	if vp.LastLine < vp.FirstLine {
		return out
	}

	for i := vp.FirstLine; i <= vp.LastLine && i < len(h.tokens); i++ {
		for _, tok := range h.tokens[i] {
			if tok.Fg == 0 {
				continue // default color; skip decoration
			}
			out = append(out, buffer.Decoration{
				Kind: buffer.DecoToken,
				Range: buffer.Range{
					Start: buffer.Position{Line: i, ByteCol: tok.Start},
					End:   buffer.Position{Line: i, ByteCol: tok.End},
				},
				FgColor: tok.Fg,
				Bold:    tok.Bold,
				Italic:  tok.Italic,
			})
		}
	}
	return out
}

// retokenizeLookahead is the number of lines beyond the viewport
// that retokenizeFrom processes before stopping. This caps the work
// for edits near the top of large files to O(viewport + lookahead)
// instead of O(file). The remaining dirty suffix is processed on
// subsequent Decorate calls as the user scrolls.
const retokenizeLookahead = 200

// buildTailText returns the buffer text from line `from` to
// `stopLine` (exclusive) as a string for chroma to tokenize.
// Builds directly from per-line data to avoid a full-buffer
// Bytes() allocation.
func (h *Highlighter) buildTailText(from, stopLine, lc int) string {
	if stopLine > lc {
		stopLine = lc
	}
	if from >= stopLine {
		return ""
	}
	// Estimate capacity: average 40 bytes/line.
	var sb strings.Builder
	sb.Grow((stopLine - from) * 40)
	for i := from; i < stopLine; i++ {
		if i > from {
			sb.WriteByte('\n')
		}
		sb.Write(h.buf.Line(i))
	}
	return sb.String()
}

// retokenizeFrom re-lexes from line `from`, backing up past any
// multi-line continuation. Capped at viewport + lookahead when
// primed. Must be called with h.mu held.
func (h *Highlighter) retokenizeFrom(from int) {
	lc := h.buf.LineCount()
	if from < 0 {
		from = 0
	}
	if from >= lc {
		// Nothing to do; just resize the caches to match.
		h.tokens = resizeSlice(h.tokens, lc)
		h.lineContinues = resizeSlice(h.lineContinues, lc)
		return
	}
	// Walk `from` backwards past any line still inside a
	// multi-line token. lineContinues[i] is authoritative if the
	// cache is primed for that line, which it is whenever i is
	// before our current restart point.
	for from > 0 && h.prevLineIsContinuation(from) {
		from--
	}

	// Compute the stop line: viewport + lookahead. When primed,
	// cap the text passed to chroma so edits near the top of
	// large files don't re-lex the entire buffer.
	stopLine := lc // default: full buffer
	if h.primed && h.lastViewport.LastLine > 0 {
		sl := h.lastViewport.LastLine + retokenizeLookahead + 1
		if sl < lc {
			stopLine = sl
		}
	}
	// Ensure stopLine is at least past `from`.
	if stopLine <= from {
		stopLine = lc
	}

	tailText := h.buildTailText(from, stopLine, lc)

	iter, err := h.lexer.Tokenise(nil, tailText)
	if err != nil {
		// Fall back to clearing the processed range; draw path
		// handles nil slices as "no decorations."
		h.tokens = resizeSlice(h.tokens, lc)
		h.lineContinues = resizeSlice(h.lineContinues, lc)
		for i := from; i < stopLine; i++ {
			h.tokens[i] = h.tokens[i][:0]
			h.lineContinues[i] = false
		}
		return
	}

	// Prepare cache slots from `from` to `stopLine`. Lines beyond
	// stopLine keep their existing cached tokens so previously
	// highlighted off-screen text doesn't flash to default coloring.
	// Reset len to 0 but retain capacity — tokenize-append below
	// reuses the backing array, cutting per-edit token allocs.
	h.tokens = resizeSlice(h.tokens, lc)
	h.lineContinues = resizeSlice(h.lineContinues, lc)
	for i := from; i < stopLine; i++ {
		h.tokens[i] = h.tokens[i][:0]
		h.lineContinues[i] = false
	}

	line := from
	col := 0
	for tok := iter(); tok.Type != chroma.EOFType; tok = iter() {
		fg, bold, italic := h.resolveToken(tok.Type)
		multi := isMultilineTokenType(tok.Type)
		val := tok.Value
		first := true
		for len(val) > 0 || first {
			first = false
			nl := strings.IndexByte(val, '\n')
			part := val
			if nl >= 0 {
				part = val[:nl]
			}
			if line >= lc || line >= stopLine {
				break
			}
			end := col + len(part)
			if len(part) > 0 &&
				len(h.tokens[line]) < maxTokensPerLine {
				h.tokens[line] = append(h.tokens[line], Token{
					Start:  col,
					End:    end,
					Fg:     fg,
					Bold:   bold,
					Italic: italic,
				})
			}
			col = end
			if nl < 0 {
				break
			}
			// Line boundary crossed; mark continuation.
			if multi && line < lc {
				h.lineContinues[line] = true
			}
			line++
			col = 0
			val = val[nl+1:]
		}
		if line >= lc || line >= stopLine {
			break
		}
	}

	// If we capped the input before EOF, mark the remainder as
	// still dirty so the next Decorate call continues.
	if stopLine < lc {
		// The last processed line may have ended in a
		// continuation. If so, the next Decorate pass must
		// restart from at least that line.
		if stopLine < h.dirtyLineStart || h.dirtyLineStart == maxInt {
			h.dirtyLineStart = stopLine
		}
	} else {
		h.dirtyLineStart = maxInt
	}
}

// isMultilineTokenType reports whether tt can span line
// boundaries (strings, comments). String == LiteralString in
// chroma, so one check covers both.
func isMultilineTokenType(tt chroma.TokenType) bool {
	return tt.InCategory(chroma.Comment) ||
		tt.InCategory(chroma.String)
}

// prevLineIsContinuation reports whether the line at index from-1
// ended inside a multi-line token. Reads the lineContinues cache
// populated by retokenizeFrom.
//
// Must be called with h.mu held.
func (h *Highlighter) prevLineIsContinuation(from int) bool {
	if from <= 0 || from-1 >= len(h.lineContinues) {
		return false
	}
	return h.lineContinues[from-1]
}

// resizeSlice grows or shrinks s to length n, preserving
// existing entries where possible.
func resizeSlice[T any](s []T, n int) []T {
	if cap(s) >= n {
		grown := s[:n]
		if n > len(s) {
			clear(grown[len(s):])
		}
		return grown
	}
	grown := make([]T, n)
	copy(grown, s)
	return grown
}

// resolveToken returns the color for a token type, checking
// overrides first, then falling back to the chroma style.
// Must be called with h.mu held.
func (h *Highlighter) resolveToken(
	tt chroma.TokenType,
) (fg uint32, bold, italic bool) {
	if c, ok := h.overrideColors[tt]; ok && c != 0 {
		return c, false, false
	}
	return mapEntry(h.style.Get(tt))
}

// mapEntry extracts RGBA + style flags from a chroma StyleEntry.
func mapEntry(e chroma.StyleEntry) (fg uint32, bold, italic bool) {
	if e.Colour.IsSet() {
		r := uint32(e.Colour.Red())
		g := uint32(e.Colour.Green())
		b := uint32(e.Colour.Blue())
		fg = (r << 24) | (g << 16) | (b << 8) | 0xFF
	}
	bold = e.Bold == chroma.Yes
	italic = e.Italic == chroma.Yes
	return
}

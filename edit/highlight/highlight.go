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

// maxTokensPerLine caps the number of Token spans stored for a
// single logical line. Beyond this, additional tokens from
// chroma are dropped so pathological minified inputs can't grow
// the per-line cache unboundedly. Visually, clipped tokens render
// as default text — acceptable degradation under duress.
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

	if !h.primed {
		h.retokenize()
		h.primed = true
		h.dirtyLineStart = maxInt
	} else if h.dirtyLineStart < maxInt {
		h.retokenizeFrom(h.dirtyLineStart)
		h.dirtyLineStart = maxInt
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

// retokenize runs chroma over the full buffer and rebuilds the
// per-line token cache. Must be called with h.mu held.
func (h *Highlighter) retokenize() {
	h.retokenizeFrom(0)
}

// retokenizeFrom incrementally re-lexes from line `from` to the
// end of the buffer, preserving tokens[0..from-1] intact. Any
// multi-line token that started before `from` forces a restart at
// the nearest pristine line; the authoritative signal is
// lineContinues[from-1], populated from chroma's token stream
// the last time we tokenized that range. If every candidate
// restart is still "inside" a multi-line construct, we fall all
// the way back to 0 and full-walk.
//
// Must be called with h.mu held.
func (h *Highlighter) retokenizeFrom(from int) {
	lc := h.buf.LineCount()
	if from < 0 {
		from = 0
	}
	if from >= lc {
		// Nothing to do; just resize the caches to match.
		h.tokens = resizeTokens(h.tokens, lc)
		h.lineContinues = resizeBools(h.lineContinues, lc)
		return
	}
	// Walk `from` backwards past any line still inside a
	// multi-line token. lineContinues[i] is authoritative if the
	// cache is primed for that line, which it is whenever i is
	// before our current restart point.
	for from > 0 && h.prevLineIsContinuation(from) {
		from--
	}

	var prefixBytes int
	for i := range from {
		prefixBytes += len(h.buf.Line(i)) + 1 // +1 for '\n'
	}
	fullBytes := h.buf.Bytes()
	tailText := string(fullBytes[prefixBytes:])

	iter, err := h.lexer.Tokenise(nil, tailText)
	if err != nil {
		// Fall back to clearing the suffix; draw path handles
		// nil slices as "no decorations."
		h.tokens = resizeTokens(h.tokens, lc)
		h.lineContinues = resizeBools(h.lineContinues, lc)
		for i := from; i < lc; i++ {
			h.tokens[i] = nil
			h.lineContinues[i] = false
		}
		return
	}

	// Prepare cache slots from `from` onward.
	h.tokens = resizeTokens(h.tokens, lc)
	h.lineContinues = resizeBools(h.lineContinues, lc)
	for i := from; i < lc; i++ {
		h.tokens[i] = nil
		h.lineContinues[i] = false
	}

	line := from
	col := 0
	for tok := iter(); tok.Type != chroma.EOFType; tok = iter() {
		fg, bold, italic := h.resolveToken(tok.Type)
		multi := isMultilineTokenType(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for pi, part := range parts {
			if pi > 0 {
				// Line boundary crossed inside `tok`. The
				// line we just left ended inside the token,
				// so mark it as a continuation.
				if multi && line < lc {
					h.lineContinues[line] = true
				}
				line++
				col = 0
			}
			if line >= lc {
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
		}
		if line >= lc {
			break
		}
	}
}

// isMultilineTokenType reports whether tt is a chroma token type
// that can legitimately span line boundaries — strings, comments,
// and their sub-categories. A boundary crossed inside such a
// token forces the incremental restart to back up past the
// affected lines.
func isMultilineTokenType(tt chroma.TokenType) bool {
	return tt.InCategory(chroma.Comment) ||
		tt.InCategory(chroma.String) ||
		tt.InCategory(chroma.LiteralString)
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

// resizeBools grows or shrinks b to length lc, preserving
// existing entries where possible. Extra entries are false.
func resizeBools(b []bool, lc int) []bool {
	if cap(b) >= lc {
		grown := b[:lc]
		// Zero any previously-used slots beyond the old length.
		for i := len(b); i < lc; i++ {
			grown[i] = false
		}
		return grown
	}
	grown := make([]bool, lc)
	copy(grown, b)
	return grown
}

// resizeTokens grows or shrinks tokens to length lc, preserving
// existing entries where possible.
func resizeTokens(tokens [][]Token, lc int) [][]Token {
	if cap(tokens) >= lc {
		return tokens[:lc]
	}
	grown := make([][]Token, lc)
	copy(grown, tokens)
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

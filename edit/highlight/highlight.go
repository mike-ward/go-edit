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

// Highlighter is a DecorationProvider backed by chroma. It
// tokenizes the buffer on demand, caches per-line tokens, and
// invalidates on edits.
type Highlighter struct {
	mu         sync.Mutex
	lexer      chroma.Lexer
	style      *chroma.Style
	buf        *buffer.Buffer
	tokens     [][]Token // per-line cache
	valid      bool      // false → retokenize on next Decorate
	invalidate func()    // RequestRedraw thunk; may be nil
	removeEdit func()    // remove handle for OnEdit observer
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
		lexer: lex,
		style: style,
		buf:   buf,
	}
	h.removeEdit = buf.OnEdit(func(_ buffer.Change) {
		h.mu.Lock()
		h.valid = false
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

// Close removes the edit observer.
func (h *Highlighter) Close() {
	if h.removeEdit != nil {
		h.removeEdit()
	}
}

// Decorate implements buffer.DecorationProvider. It returns
// DecoToken decorations for the visible viewport. Invalid
// (or never-tokenized) lines are tokenized synchronously.
func (h *Highlighter) Decorate(vp buffer.Viewport) []buffer.Decoration {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.valid {
		h.retokenize()
		h.valid = true
	}

	// Clamp viewport to valid range.
	if vp.FirstLine < 0 {
		vp.FirstLine = 0
	}
	if vp.LastLine < vp.FirstLine {
		return nil
	}

	var decos []buffer.Decoration
	for i := vp.FirstLine; i <= vp.LastLine && i < len(h.tokens); i++ {
		for _, tok := range h.tokens[i] {
			if tok.Fg == 0 {
				continue // default color; skip decoration
			}
			decos = append(decos, buffer.Decoration{
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
	return decos
}

// retokenize runs chroma over the full buffer and rebuilds the
// per-line token cache. Must be called with h.mu held.
func (h *Highlighter) retokenize() {
	text := h.buf.String()
	iter, err := h.lexer.Tokenise(nil, text)
	if err != nil {
		h.tokens = nil
		return
	}

	lc := h.buf.LineCount()
	tokens := make([][]Token, lc)
	line := 0
	col := 0

	for tok := iter(); tok.Type != chroma.EOFType; tok = iter() {
		entry := h.style.Get(tok.Type)
		fg, bold, italic := mapEntry(entry)

		// A single chroma token can span multiple lines
		// (e.g. multi-line strings/comments).
		parts := strings.Split(tok.Value, "\n")
		for pi, part := range parts {
			if pi > 0 {
				line++
				col = 0
			}
			if line >= lc {
				break
			}
			end := col + len(part)
			if len(part) > 0 {
				tokens[line] = append(tokens[line], Token{
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

	h.tokens = tokens
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

package edit

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/mike-ward/go-edit/edit/buffer"
)

// LangConfig provides per-language editor settings.
type LangConfig struct {
	TabWidth          int    // 0 = use buffer autodetect
	UseTabs           bool   // only applied when TabWidth > 0
	CommentLine       string // e.g. "//", "#", "--"
	CommentBlockStart string // e.g. "/*"
	CommentBlockEnd   string // e.g. "*/"
}

// resolveLangConfig looks up the LangConfig for the current
// buffer. Resolution order: exact language name match (from
// highlighter detection), then file extension match (with dot
// prefix), then zero value.
func resolveLangConfig(cfg EditorCfg) LangConfig {
	if len(cfg.LangConfigs) == 0 {
		return LangConfig{}
	}
	fp := cfg.Buffer.Props.FilePath

	// Try file extension match.
	if fp != "" {
		ext := strings.ToLower(filepath.Ext(fp))
		if ext != "" {
			if lc, ok := cfg.LangConfigs[ext]; ok {
				return lc
			}
		}
		// Try filename match (e.g. "Makefile").
		base := filepath.Base(fp)
		if lc, ok := cfg.LangConfigs[base]; ok {
			return lc
		}
	}
	return LangConfig{}
}

// toggleComment toggles line comments for the given cursor's
// line or selection range. If all affected lines have the
// comment prefix, it removes it; otherwise it adds it.
func toggleComment(
	cfg EditorCfg,
	cs *CursorState,
	buf *buffer.Buffer,
) {
	lc := resolveLangConfig(cfg)
	if lc.CommentLine == "" {
		return
	}
	prefix := []byte(lc.CommentLine + " ")
	prefixNoSpace := []byte(lc.CommentLine)

	startLine := cs.Cursor.Line
	endLine := cs.Cursor.Line
	if cs.HasSelection() {
		sel := cs.SelectionRange()
		startLine = sel.Start.Line
		endLine = sel.End.Line
		// Skip last line if cursor at col 0.
		if sel.End.ByteCol == 0 && endLine > startLine {
			endLine--
		}
	}
	// Clamp to buffer bounds.
	total := buf.LineCount()
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= total {
		endLine = total - 1
	}
	if startLine > endLine {
		return
	}

	// Single pass: check if all non-blank lines are commented and
	// find the minimum indent for aligned insertion.
	allCommented := true
	minIndent := -1
	for li := startLine; li <= endLine; li++ {
		line := buf.Line(li)
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 {
			continue
		}
		ws := len(line) - len(trimmed)
		if minIndent < 0 || ws < minIndent {
			minIndent = ws
		}
		if allCommented && !bytes.HasPrefix(trimmed, prefixNoSpace) {
			allCommented = false
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	buf.BeginGroup()
	if allCommented {
		// Remove comment prefix (last → first).
		for li := endLine; li >= startLine; li-- {
			line := buf.Line(li)
			trimmed := bytes.TrimLeft(line, " \t")
			if len(trimmed) == 0 {
				continue
			}
			wsLen := len(line) - len(trimmed)
			removeLen := len(prefixNoSpace)
			// Also remove trailing space after prefix.
			if wsLen+removeLen < len(line) &&
				line[wsLen+removeLen] == ' ' {
				removeLen++
			}
			start := buffer.Position{
				Line: li, ByteCol: wsLen,
			}
			end := buffer.Position{
				Line: li, ByteCol: wsLen + removeLen,
			}
			buf.Apply(buffer.Edit{
				Range: buffer.Range{Start: start, End: end},
			})
		}
	} else {
		// Add comment prefix (last → first).
		for li := endLine; li >= startLine; li-- {
			line := buf.Line(li)
			trimmed := bytes.TrimLeft(line, " \t")
			if len(trimmed) == 0 {
				continue
			}
			pos := buffer.Position{
				Line: li, ByteCol: minIndent,
			}
			buf.Apply(buffer.Edit{
				Range:    buffer.Range{Start: pos, End: pos},
				NewBytes: prefix,
			})
		}
	}
	buf.EndGroup()
}

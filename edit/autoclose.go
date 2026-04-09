package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// AutoClosePair defines a pair of characters that auto-close.
type AutoClosePair struct{ Open, Close byte }

// DefaultAutoClosePairs is the default set of auto-close pairs.
var DefaultAutoClosePairs = []AutoClosePair{
	{'(', ')'},
	{'[', ']'},
	{'{', '}'},
	{'"', '"'},
	{'\'', '\''},
	{'`', '`'},
}

// autoCloseFilter returns an EditFilter that appends a closing
// character when an opener is inserted at a suitable position.
// pairs must not be nil.
func autoCloseFilter(
	pairs []AutoClosePair,
) buffer.EditFilter {
	// Build lookup tables.
	closerFor := make(map[byte]byte, len(pairs))
	isCloser := make(map[byte]bool, len(pairs))
	for _, p := range pairs {
		closerFor[p.Open] = p.Close
		isCloser[p.Close] = true
	}

	return func(b *buffer.Buffer, e *buffer.Edit) buffer.FilterResult {
		// Only act on single-byte inserts (not paste, not delete).
		if len(e.NewBytes) != 1 || !e.Range.Empty() {
			return buffer.FilterAccept
		}
		ch := e.NewBytes[0]
		closer, ok := closerFor[ch]
		if !ok {
			return buffer.FilterAccept
		}

		// For same-char pairs (quotes), don't auto-close if the
		// char at cursor is the same (would be a skip-over
		// situation handled elsewhere) or if preceded by an
		// alphanumeric byte.
		if ch == closer {
			pos := e.Range.Start
			line := b.Line(pos.Line)
			// Don't auto-close if preceded by alphanum.
			if pos.ByteCol > 0 && isAlphaNum(line[pos.ByteCol-1]) {
				return buffer.FilterAccept
			}
		}

		// Only auto-close if the next char is whitespace, a closer,
		// or EOL.
		pos := e.Range.Start
		line := b.Line(pos.Line)
		if pos.ByteCol < len(line) {
			next := line[pos.ByteCol]
			if !isWhitespace(next) && !isCloser[next] {
				return buffer.FilterAccept
			}
		}
		// EOL or suitable next char — append closer.
		e.NewBytes = append(e.NewBytes, closer)
		return buffer.FilterAccept
	}
}

// shouldSkipCloser reports whether typing ch at pos should skip
// over an existing closer rather than inserting it.
func shouldSkipCloser(
	buf *buffer.Buffer,
	pos buffer.Position,
	ch byte,
	pairs []AutoClosePair,
) bool {
	if buf == nil || pos.Line < 0 || pos.Line >= buf.LineCount() {
		return false
	}
	isCloser := false
	for _, p := range pairs {
		if p.Close == ch {
			isCloser = true
			break
		}
	}
	if !isCloser {
		return false
	}
	line := buf.Line(pos.Line)
	return pos.ByteCol < len(line) && line[pos.ByteCol] == ch
}

// shouldDeletePair reports whether backspacing at pos should delete
// both the opener before the cursor and the closer after it.
func shouldDeletePair(
	buf *buffer.Buffer,
	pos buffer.Position,
	pairs []AutoClosePair,
) bool {
	if buf == nil || pos.Line < 0 || pos.Line >= buf.LineCount() {
		return false
	}
	if pos.ByteCol == 0 {
		return false
	}
	line := buf.Line(pos.Line)
	if pos.ByteCol >= len(line) {
		return false
	}
	before := line[pos.ByteCol-1]
	after := line[pos.ByteCol]
	for _, p := range pairs {
		if p.Open == before && p.Close == after {
			return true
		}
	}
	return false
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

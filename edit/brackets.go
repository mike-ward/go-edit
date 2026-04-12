package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// maxBracketScan limits how many bytes the bracket matcher will
// traverse before giving up. Prevents pathological scans on
// minified files.
const maxBracketScan = 10_000

// bracketPairs maps openers to closers and vice versa.
var bracketPairs = map[byte]byte{
	'(': ')', ')': '(',
	'[': ']', ']': '[',
	'{': '}', '}': '{',
}

// isOpener reports whether b is an opening bracket.
func isOpener(b byte) bool {
	return b == '(' || b == '[' || b == '{'
}

// bracketAtCursor returns the bracket byte and its position
// relative to the cursor. It checks the byte at the cursor first,
// then the byte before. Returns 0 if neither is a bracket.
func bracketAtCursor(buf *buffer.Buffer, pos buffer.Position) (byte, buffer.Position) {
	if buf == nil || pos.Line < 0 || pos.Line >= buf.LineCount() {
		return 0, buffer.Position{}
	}
	line := buf.Line(pos.Line)
	// Check at cursor.
	if pos.ByteCol < len(line) {
		if _, ok := bracketPairs[line[pos.ByteCol]]; ok {
			return line[pos.ByteCol], pos
		}
	}
	// Check before cursor.
	if pos.ByteCol > 0 {
		bc := pos.ByteCol - 1
		if _, ok := bracketPairs[line[bc]]; ok {
			return line[bc], buffer.Position{Line: pos.Line, ByteCol: bc}
		}
	}
	return 0, buffer.Position{}
}

// findMatchingBracket scans from pos for the matching bracket.
// Returns (bracket position, match position, found, capped).
// capped=true means the scan hit maxBracketScan before finding a
// match; callers rendering a highlight should suppress it.
func findMatchingBracket(
	buf *buffer.Buffer, pos buffer.Position,
) (bracketPos, matchPos buffer.Position, found, capped bool) {
	if buf == nil {
		return
	}
	b, bpos := bracketAtCursor(buf, pos)
	if b == 0 {
		return
	}
	match := bracketPairs[b]
	if isOpener(b) {
		m, ok, cap := scanForward(buf, bpos, b, match)
		return bpos, m, ok, cap
	}
	m, ok, cap := scanBackward(buf, bpos, b, match)
	return bpos, m, ok, cap
}

// scanForward searches forward from pos for match, tracking nesting
// with open. pos itself is skipped (it's the opener).
func scanForward(
	buf *buffer.Buffer,
	pos buffer.Position,
	open, close byte,
) (buffer.Position, bool, bool) {
	depth := 1
	scanned := 0
	line := pos.Line
	col := pos.ByteCol + 1 // skip the opener itself

	for line < buf.LineCount() {
		lb := buf.Line(line)
		for col < len(lb) {
			scanned++
			if scanned > maxBracketScan {
				return buffer.Position{}, false, true
			}
			switch lb[col] {
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return buffer.Position{
						Line: line, ByteCol: col,
					}, true, false
				}
			}
			col++
		}
		line++
		col = 0
	}
	return buffer.Position{}, false, false
}

// scanBackward searches backward from pos for match.
func scanBackward(
	buf *buffer.Buffer,
	pos buffer.Position,
	close, open byte,
) (buffer.Position, bool, bool) {
	depth := 1
	scanned := 0
	line := pos.Line
	col := pos.ByteCol - 1 // skip the closer itself; may be -1

	for {
		if col < 0 {
			// Move to previous line.
			line--
			if line < 0 {
				break
			}
			col = len(buf.Line(line)) - 1
			if col < 0 {
				continue // empty line
			}
		}
		lb := buf.Line(line)
		for col >= 0 {
			scanned++
			if scanned > maxBracketScan {
				return buffer.Position{}, false, true
			}
			switch lb[col] {
			case close:
				depth++
			case open:
				depth--
				if depth == 0 {
					return buffer.Position{
						Line: line, ByteCol: col,
					}, true, false
				}
			}
			col--
		}
	}
	return buffer.Position{}, false, false
}

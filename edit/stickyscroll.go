package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// defaultStickyMax is the maximum number of sticky scroll lines.
const defaultStickyMax = 5

// resolveStickyScroll returns whether sticky scroll is active,
// applying the runtime override.
func resolveStickyScroll(cfg bool, override int) bool {
	switch override {
	case 1:
		return true
	case 2:
		return false
	default:
		return cfg
	}
}

// findScopeHeaders walks backward from firstVisibleLine,
// collecting lines with strictly decreasing indentation. These
// represent enclosing scope openers (functions, blocks, etc.).
// Returns up to max lines, outermost first.
func findScopeHeaders(
	buf *buffer.Buffer, firstVisibleLine, maxHeaders, tabWidth int,
) []int {
	if buf == nil || maxHeaders <= 0 || firstVisibleLine <= 0 {
		return nil
	}
	if firstVisibleLine >= buf.LineCount() {
		firstVisibleLine = buf.LineCount() - 1
	}
	if tabWidth <= 0 {
		tabWidth = 4
	}
	var headers []int
	// Current indent ceiling — only lines with strictly less
	// indent qualify as enclosing scopes.
	ceiling := lineIndent(buf.Line(firstVisibleLine), tabWidth)

	for line := firstVisibleLine - 1; line >= 0; line-- {
		lb := buf.Line(line)
		if isBlankLine(lb) {
			continue
		}
		ind := lineIndent(lb, tabWidth)
		if ind < ceiling {
			headers = append(headers, line)
			ceiling = ind
			if len(headers) >= maxHeaders {
				break
			}
			if ceiling == 0 {
				break // can't go further out
			}
		}
	}
	// Reverse so outermost is first.
	for l, r := 0, len(headers)-1; l < r; l, r = l+1, r-1 {
		headers[l], headers[r] = headers[r], headers[l]
	}
	return headers
}

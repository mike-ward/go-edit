package edit

import (
	"slices"

	"github.com/mike-ward/go-edit/edit/buffer"
)

// defaultStickyMax is the maximum number of sticky scroll lines.
const defaultStickyMax = 5

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
	slices.Reverse(headers)
	return headers
}

package buffer

// maxIndentScanLines is the number of non-empty lines inspected
// for indent detection.
const maxIndentScanLines = 1000

// detectIndent examines the first maxIndentScanLines non-empty lines
// and returns the dominant indent style. Returns UseTabs=true,
// Width=4 when the file has no indented lines.
func detectIndent(b *Buffer) IndentStyle {
	tabLines := 0
	spaceLines := 0
	// widthCounts[w] = number of lines whose leading-space count is
	// divisible by w (for w in 2,4,8).
	var widthCounts [9]int

	scanned := 0
	for i := 0; i < b.LineCount() && scanned < maxIndentScanLines; i++ {
		line := b.Line(i)
		if len(line) == 0 {
			continue
		}
		scanned++

		switch line[0] {
		case '\t':
			tabLines++
		case ' ':
			// Count leading spaces (byte loop — ASCII only).
			spaces := 0
			for spaces < len(line) && line[spaces] == ' ' {
				spaces++
			}
			if spaces >= 2 {
				spaceLines++
				for _, w := range []int{2, 4, 8} {
					if spaces%w == 0 {
						widthCounts[w]++
					}
				}
			}
		}
	}

	if tabLines == 0 && spaceLines == 0 {
		return IndentStyle{UseTabs: true, Width: 4}
	}

	if tabLines >= spaceLines {
		return IndentStyle{UseTabs: true, Width: 4}
	}

	// Find the highest match count across candidate widths.
	width := 4
	best := 0
	for _, w := range []int{2, 4, 8} {
		if widthCounts[w] > best {
			best = widthCounts[w]
		}
	}
	// Among widths tied for the best count, pick the largest.
	// widthCounts[2] >= widthCounts[4] >= widthCounts[8] always
	// (divisibility), so the largest tied width is the true indent.
	for _, w := range []int{8, 4, 2} {
		if widthCounts[w] == best {
			width = w
			break
		}
	}

	return IndentStyle{UseTabs: false, Width: width}
}

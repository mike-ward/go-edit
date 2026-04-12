package buffer

// line holds the raw bytes of a single logical line (no trailing '\n').
//
// Phase 1a uses a plain []byte for correctness. A per-line gap buffer
// is the Phase 1 target per ROADMAP but only pays off once bench
// pressure shows repeated single-line typing as a bottleneck. Swap in
// place when that happens; the type is unexported.
type line struct {
	b []byte
}

func newLine(b []byte) *line {
	// Copy so the buffer owns its bytes independently of the loader.
	cp := make([]byte, len(b))
	copy(cp, b)
	return &line{b: cp}
}

func (l *line) len() int { return len(l.b) }

// bytes returns the line's current bytes. The slice is owned by the
// line and must not be retained across a mutation.
func (l *line) bytes() []byte { return l.b }

// insert inserts p at byte column col. col is clamped to [0, len].
// Reuses backing capacity when possible to avoid allocation on
// single-character typing.
func (l *line) insert(col int, p []byte) {
	if col < 0 {
		col = 0
	}
	if col > len(l.b) {
		col = len(l.b)
	}
	newLen := len(l.b) + len(p)
	if cap(l.b) >= newLen {
		l.b = l.b[:newLen]
		copy(l.b[col+len(p):], l.b[col:newLen-len(p)])
		copy(l.b[col:], p)
		return
	}
	out := make([]byte, newLen)
	copy(out, l.b[:col])
	copy(out[col:], p)
	copy(out[col+len(p):], l.b[col:])
	l.b = out
}

// deleteRange removes bytes in [start, end). Both are clamped.
func (l *line) deleteRange(start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(l.b) {
		end = len(l.b)
	}
	if start >= end {
		return
	}
	l.b = append(l.b[:start], l.b[end:]...)
}

// split returns the bytes at and after col, truncating the line to col.
func (l *line) split(col int) []byte {
	if col < 0 {
		col = 0
	}
	if col > len(l.b) {
		col = len(l.b)
	}
	tail := make([]byte, len(l.b)-col)
	copy(tail, l.b[col:])
	l.b = l.b[:col]
	return tail
}

// truncate discards bytes at and after col.
func (l *line) truncate(col int) {
	if col < len(l.b) {
		l.b = l.b[:col]
	}
}

func (l *line) appendBytes(p []byte) {
	l.b = append(l.b, p...)
}

package buffer

// Position identifies a byte location in the buffer by line number and
// byte column within that line. Line and ByteCol are 0-based.
//
// ByteCol is a byte offset into the line's raw bytes, not a rune or
// grapheme index. Cursor movement may advance by grapheme clusters, but
// storage is always bytes.
type Position struct {
	Line    int
	ByteCol int
}

// Before reports whether p sorts strictly before q.
func (p Position) Before(q Position) bool {
	if p.Line != q.Line {
		return p.Line < q.Line
	}
	return p.ByteCol < q.ByteCol
}

// After reports whether p sorts strictly after q.
func (p Position) After(q Position) bool {
	if p.Line != q.Line {
		return p.Line > q.Line
	}
	return p.ByteCol > q.ByteCol
}

// Range is a half-open [Start, End) byte span. Start <= End.
type Range struct {
	Start, End Position
}

// Empty reports whether the range covers zero bytes.
func (r Range) Empty() bool { return r.Start == r.End }

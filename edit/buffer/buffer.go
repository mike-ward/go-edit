package buffer

import (
	"bytes"
	"fmt"
	"io"
)

// MaxLoadBytes caps the number of bytes Buffer.Load will read from
// an io.Reader. Larger files return an error rather than risking
// OOM from an unbounded io.ReadAll.
const MaxLoadBytes = 1 << 28 // 256 MiB

// Buffer is the document model: a slice of lines plus the single edit
// choke point Apply. A Buffer always contains at least one line (the
// empty line for an empty document).
type Buffer struct {
	lines []*line
}

// New returns an empty buffer containing a single empty line.
func New() *Buffer {
	return &Buffer{lines: []*line{newLine(nil)}}
}

// Load reads r fully and splits on '\n'. Bytes are preserved verbatim;
// no EOL normalization. A trailing newline produces a final empty line,
// matching standard text-file semantics.
//
// Load caps the read at MaxLoadBytes and returns an error if r
// would produce more. Callers that need to load larger files can
// stream into a buffer manually.
func Load(r io.Reader) (*Buffer, error) {
	if r == nil {
		return New(), nil
	}
	// +1 so we can detect over-limit reads.
	limited := io.LimitReader(r, MaxLoadBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxLoadBytes {
		return nil, fmt.Errorf("buffer: input exceeds %d bytes", MaxLoadBytes)
	}
	return FromBytes(raw), nil
}

// FromBytes builds a buffer from raw bytes without I/O.
func FromBytes(raw []byte) *Buffer {
	if len(raw) == 0 {
		return New()
	}
	parts := bytes.Split(raw, []byte{'\n'})
	lines := make([]*line, len(parts))
	for i, p := range parts {
		lines[i] = newLine(p)
	}
	return &Buffer{lines: lines}
}

// LineCount returns the number of lines. Always >= 1.
func (b *Buffer) LineCount() int { return len(b.lines) }

// Line returns the raw bytes of line i (no trailing newline). The slice
// is owned by the buffer and must not be retained across a mutation.
func (b *Buffer) Line(i int) []byte { return b.lines[i].bytes() }

// Len returns the total byte length including inter-line newlines.
func (b *Buffer) Len() int {
	n := 0
	for _, l := range b.lines {
		n += l.len()
	}
	// One '\n' between each pair of lines.
	n += len(b.lines) - 1
	return n
}

// String returns the full buffer as a string. Testing only — allocates.
func (b *Buffer) String() string {
	var out bytes.Buffer
	out.Grow(b.Len())
	for i, l := range b.lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.Write(l.bytes())
	}
	return out.String()
}

// Apply is the single mutation choke point. Every edit routes through
// here so EditFilters, marks, and undo (Phases 1.5, 3) can observe one
// API. Returns a Change suitable for the undo stack.
//
// Apply panics on internal invariant break (e.g. negative line index).
// It tolerates out-of-range columns by clamping. It never panics on
// user input. Panic policy locked in Phase -1.
func (b *Buffer) Apply(e Edit) Change {
	e.Range = b.clampRange(e.Range)
	old := b.bytesInRange(e.Range)

	// Delete the range.
	b.deleteRange(e.Range)

	// Insert NewBytes at the collapsed start position.
	endPos := b.insertAt(e.Range.Start, e.NewBytes)

	return Change{
		Applied:      e,
		OldBytes:     old,
		AppliedRange: Range{Start: e.Range.Start, End: endPos},
	}
}

// clampRange normalizes a range to valid buffer coordinates and
// guarantees Start <= End.
func (b *Buffer) clampRange(r Range) Range {
	r.Start = b.clampPos(r.Start)
	r.End = b.clampPos(r.End)
	if r.End.Before(r.Start) {
		r.Start, r.End = r.End, r.Start
	}
	return r
}

func (b *Buffer) clampPos(p Position) Position {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= len(b.lines) {
		p.Line = len(b.lines) - 1
	}
	if p.ByteCol < 0 {
		p.ByteCol = 0
	}
	if ll := b.lines[p.Line].len(); p.ByteCol > ll {
		p.ByteCol = ll
	}
	return p
}

// bytesInRange returns a newly allocated copy of the bytes in r,
// including '\n' separators for multi-line spans.
func (b *Buffer) bytesInRange(r Range) []byte {
	if r.Empty() {
		return nil
	}
	if r.Start.Line == r.End.Line {
		src := b.lines[r.Start.Line].bytes()[r.Start.ByteCol:r.End.ByteCol]
		out := make([]byte, len(src))
		copy(out, src)
		return out
	}
	var out bytes.Buffer
	// First line: from Start.ByteCol to end of line, plus '\n'.
	first := b.lines[r.Start.Line].bytes()
	out.Write(first[r.Start.ByteCol:])
	out.WriteByte('\n')
	// Middle lines: full + '\n'.
	for li := r.Start.Line + 1; li < r.End.Line; li++ {
		out.Write(b.lines[li].bytes())
		out.WriteByte('\n')
	}
	// Last line: up to End.ByteCol.
	last := b.lines[r.End.Line].bytes()
	out.Write(last[:r.End.ByteCol])
	return out.Bytes()
}

// deleteRange removes the bytes in r from the buffer. Lines are joined
// as needed. After return, r.Start is the collapsed position.
func (b *Buffer) deleteRange(r Range) {
	if r.Empty() {
		return
	}
	if r.Start.Line == r.End.Line {
		b.lines[r.Start.Line].deleteRange(r.Start.ByteCol, r.End.ByteCol)
		return
	}
	// Truncate first line at Start.ByteCol.
	first := b.lines[r.Start.Line]
	first.b = first.b[:r.Start.ByteCol]
	// Append tail of last line.
	last := b.lines[r.End.Line]
	first.appendBytes(last.bytes()[r.End.ByteCol:])
	// Drop lines (Start.Line+1 .. End.Line] inclusive of End.Line.
	// Nil stale pointers before the splice so GC can collect them.
	for i := r.Start.Line + 1; i <= r.End.Line; i++ {
		b.lines[i] = nil
	}
	b.lines = append(b.lines[:r.Start.Line+1], b.lines[r.End.Line+1:]...)
}

// insertAt inserts p at position pos and returns the position
// immediately after the inserted bytes.
func (b *Buffer) insertAt(pos Position, p []byte) Position {
	if len(p) == 0 {
		return pos
	}
	segs := bytes.Split(p, []byte{'\n'})

	if len(segs) == 1 {
		b.lines[pos.Line].insert(pos.ByteCol, segs[0])
		return Position{Line: pos.Line, ByteCol: pos.ByteCol + len(segs[0])}
	}

	// Multi-segment insert: split current line, build new lines.
	cur := b.lines[pos.Line]
	tail := cur.split(pos.ByteCol)
	cur.appendBytes(segs[0])

	// Final segment becomes start of what was cur.split's tail.
	lastSeg := segs[len(segs)-1]
	newLast := newLine(nil)
	newLast.appendBytes(lastSeg)
	newLast.appendBytes(tail)

	// Middle segments become standalone lines.
	mid := make([]*line, 0, len(segs)-1)
	for _, s := range segs[1 : len(segs)-1] {
		mid = append(mid, newLine(s))
	}
	mid = append(mid, newLast)

	// Splice mid after pos.Line.
	tailLines := append([]*line{}, b.lines[pos.Line+1:]...)
	b.lines = append(b.lines[:pos.Line+1], mid...)
	b.lines = append(b.lines, tailLines...)

	return Position{
		Line:    pos.Line + len(segs) - 1,
		ByteCol: len(lastSeg),
	}
}

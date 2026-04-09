package buffer

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// MaxLoadBytes caps the number of bytes Buffer.Load will read from
// an io.Reader. Larger files return an error rather than risking
// OOM from an unbounded io.ReadAll.
const MaxLoadBytes = 1 << 28 // 256 MiB

// Buffer is the document model: a slice of lines plus the single edit
// choke point Apply. A Buffer always contains at least one line (the
// empty line for an empty document).
type Buffer struct {
	lines    []*line
	Props    FileProps
	dirty    bool
	filters  []EditFilter
	postEdit []PostEditFunc
	marks    *MarkSet
}

// New returns an empty buffer containing a single empty line.
func New() *Buffer {
	return &Buffer{
		lines: []*line{newLine(nil)},
		Props: DefaultFileProps(),
	}
}

// Dirty reports whether the buffer has been modified since the last
// load or save.
func (b *Buffer) Dirty() bool { return b.dirty }

// Marks returns the buffer's mark set, creating it lazily.
func (b *Buffer) Marks() *MarkSet {
	if b.marks == nil {
		b.marks = &MarkSet{}
	}
	return b.marks
}

// MarkClean clears the dirty flag, typically after a successful save.
func (b *Buffer) MarkClean() { b.dirty = false }

// Load reads r fully, detects encoding and EOL convention,
// transcodes to UTF-8, normalizes line endings to LF, and splits
// into lines. Detected metadata is stored in Props.
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
	return fromRawBytes(raw)
}

// LoadFile opens path, reads its contents, and returns a Buffer
// with Props.FilePath, FileMode, and ModTime populated from stat.
func LoadFile(path string) (*Buffer, error) {
	if path == "" {
		return nil, fmt.Errorf("buffer: empty file path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	buf, err := Load(f)
	if err != nil {
		return nil, err
	}
	buf.Props.FilePath = path
	buf.Props.FileMode = info.Mode()
	buf.Props.ModTime = info.ModTime()
	return buf, nil
}

// fromRawBytes detects encoding/EOL, transcodes, normalizes, and
// builds the buffer.
func fromRawBytes(raw []byte) (*Buffer, error) {
	if len(raw) == 0 {
		return New(), nil
	}

	enc, hasBOM := sniffEncoding(raw)

	utf8Data, err := decodeToUTF8(raw, enc)
	if err != nil {
		return nil, fmt.Errorf("buffer: decode %v: %w", enc, err)
	}

	// Detect EOL after transcoding so UTF-16 \r\n (multi-byte code
	// units) are correctly recognized as CRLF, not mixed CR+LF.
	eol := detectEOL(utf8Data)

	if enc != EncodingRaw {
		utf8Data = normalizeEOL(utf8Data)
	}

	b := FromBytes(utf8Data)
	b.Props = FileProps{
		EOL:          eol,
		Encoding:     enc,
		HasBOM:       hasBOM,
		FinalNewline: true,
		PreserveBOM:  true,
		IndentStyle:  detectIndent(b),
	}
	return b, nil
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
	return &Buffer{lines: lines, Props: DefaultFileProps()}
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

// Bytes returns the full buffer content as LF-separated bytes.
func (b *Buffer) Bytes() []byte {
	var out bytes.Buffer
	out.Grow(b.Len())
	for i, l := range b.lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.Write(l.bytes())
	}
	return out.Bytes()
}

// String returns the full buffer as a string. Testing only — allocates.
func (b *Buffer) String() string { return string(b.Bytes()) }

// Apply is the single mutation choke point. Every edit routes through
// here so EditFilters, marks, and undo (Phases 1.5, 3) can observe one
// API. Returns a Change suitable for the undo stack.
//
// Apply panics on internal invariant break (e.g. negative line index).
// It tolerates out-of-range columns by clamping. It never panics on
// user input. Panic policy locked in Phase -1.
func (b *Buffer) Apply(e Edit) Change {
	e.Range = b.clampRange(e.Range)

	// Run filter chain. Any rejection aborts the edit.
	for _, f := range b.filters {
		if f == nil {
			continue
		}
		if f(b, &e) == FilterReject {
			return Change{}
		}
	}

	b.dirty = true
	old := b.bytesInRange(e.Range)

	// Delete the range.
	b.deleteRange(e.Range)

	// Insert NewBytes at the collapsed start position.
	endPos := b.insertAt(e.Range.Start, e.NewBytes)

	// Update marks for the edit.
	b.marks.adjust(e, endPos)

	c := Change{
		Applied:      e,
		OldBytes:     old,
		AppliedRange: Range{Start: e.Range.Start, End: endPos},
	}

	// Notify post-edit observers.
	for _, fn := range b.postEdit {
		if fn != nil {
			fn(c)
		}
	}

	return c
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

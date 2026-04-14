package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxLoadBytes caps the number of bytes Buffer.Load will read from
// an io.Reader. Larger files return an error rather than risking
// OOM from an unbounded io.ReadAll. The `[]*line` line store
// costs roughly 2x the file size in Go heap; 32 MiB keeps widget
// memory bounded until the per-line gap-buffer lands.
const MaxLoadBytes = 1 << 25 // 32 MiB

// MaxLineBytes caps the byte length of any single line in the
// buffer. Edits that would produce a line exceeding this limit
// are rejected by Apply (returning a zero Change); the loader
// hard-splits over-long segments at `fromRawBytes` time so
// pathological inputs (minified JS, 2 MB single-line logs) open
// read-only-ish instead of breaking the wrap/measurer paths.
const MaxLineBytes = 1 << 20 // 1 MiB

// Buffer is the document model: a slice of lines plus the single edit
// choke point Apply. A Buffer always contains at least one line (the
// empty line for an empty document).
type Buffer struct {
	lines    []*line
	Props    FileProps
	dirty    bool
	version  uint64 // monotonic; bumped on every applyCore mutation
	filters  []EditFilter
	postEdit []PostEditFunc
	marks    *MarkSet
	undo     *undoStack
}

// Version returns a monotonic counter bumped on every buffer
// mutation. Widgets use this as a cheap "has the buffer changed
// since I last looked" check without diffing content.
func (b *Buffer) Version() uint64 {
	if b == nil {
		return 0
	}
	return b.version
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
// When undo is enabled, records the current stack depth so that
// undoing back to this point clears dirty.
func (b *Buffer) MarkClean() {
	b.dirty = false
	if b.undo != nil {
		b.undo.cleanIdx = len(b.undo.undo)
	}
}

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
		return nil, errors.New("buffer: empty file path")
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

// FromBytes builds a buffer from raw bytes without I/O. Any
// natural line whose length exceeds MaxLineBytes is hard-split
// into synthetic continuation lines of MaxLineBytes each; the
// split is driven by byte length only, not by character
// boundaries, so a split inside a multi-byte rune is possible
// but visually benign because all render paths treat bytes as
// bytes until Phase 2 grapheme support lands.
func FromBytes(raw []byte) *Buffer {
	if len(raw) == 0 {
		return New()
	}
	parts := bytes.Split(raw, []byte{'\n'})
	lines := make([]*line, 0, len(parts))
	for _, p := range parts {
		if len(p) <= MaxLineBytes {
			lines = append(lines, newLine(p))
			continue
		}
		for start := 0; start < len(p); start += MaxLineBytes {
			end := min(start+MaxLineBytes, len(p))
			lines = append(lines, newLine(p[start:end]))
		}
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

// TextInRange returns the text in the given range as a string.
// The range is clamped to valid coordinates.
func (b *Buffer) TextInRange(r Range) string {
	r = b.clampRange(r)
	return string(b.bytesInRange(r))
}

// Apply is the single mutation choke point. Every edit routes through
// here so EditFilters, marks, and undo can observe one API. Returns
// a Change suitable for the undo stack.
//
// Apply panics on internal invariant break (e.g. negative line index).
// It tolerates out-of-range columns by clamping. It never panics on
// user input. Panic policy locked in Phase -1.
//
// Apply rejects edits that would produce a line exceeding
// MaxLineBytes; rejection returns a zero Change just like a
// filter veto.
func (b *Buffer) Apply(e Edit) Change {
	// Clamp here so filters see valid coordinates. applyCore
	// does not re-clamp on the Apply path (record=true) since
	// the edit is already normalized.
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

	if b.editWouldExceedMaxLine(e) {
		return Change{}
	}

	c := b.applyCore(e, true)
	return c
}

// editWouldExceedMaxLine reports whether applying e would produce
// any resulting line longer than MaxLineBytes. Used by Apply to
// reject pathological edits before they reach applyCore.
//
// Hardening: any NewBytes payload larger than MaxLoadBytes is
// rejected unconditionally — no valid edit should introduce more
// bytes than the whole-buffer cap. This short-circuits the
// bytes.Split allocation on adversarial 1 GiB paste payloads.
func (b *Buffer) editWouldExceedMaxLine(e Edit) bool {
	if len(e.NewBytes) > MaxLoadBytes {
		return true
	}
	// Fast path: new bytes empty or small and no line grows.
	if len(e.NewBytes) == 0 {
		// Deletion only: never grows a line beyond its current
		// length unless it joins two lines. The collapsed line
		// is first[:Start.ByteCol] + last[End.ByteCol:].
		if e.Range.Start.Line == e.Range.End.Line {
			return false
		}
		lastLen := len(b.lines[e.Range.End.Line].bytes())
		joined := e.Range.Start.ByteCol +
			(lastLen - e.Range.End.ByteCol)
		return joined > MaxLineBytes
	}
	// Insertion or replace. Fast path: no newline in NewBytes
	// means a single-segment insert — avoids bytes.Split alloc.
	if bytes.IndexByte(e.NewBytes, '\n') < 0 {
		prefix := e.Range.Start.ByteCol
		suffix := len(b.lines[e.Range.End.Line].bytes()) -
			e.Range.End.ByteCol
		return prefix+len(e.NewBytes)+suffix > MaxLineBytes
	}
	segs := bytes.Split(e.NewBytes, []byte{'\n'})
	if len(segs) == 1 {
		// Single-segment insert collapses to one line: the
		// prefix + segs[0] + suffix of the original end line.
		prefix := e.Range.Start.ByteCol
		suffix := len(b.lines[e.Range.End.Line].bytes()) -
			e.Range.End.ByteCol
		return prefix+len(segs[0])+suffix > MaxLineBytes
	}
	// Multi-segment: first resulting line = prefix + segs[0];
	// last resulting line = segs[last] + suffix; middle lines
	// are segs[1..last-1] as-is.
	first := e.Range.Start.ByteCol + len(segs[0])
	if first > MaxLineBytes {
		return true
	}
	lastIdx := len(segs) - 1
	suffix := len(b.lines[e.Range.End.Line].bytes()) -
		e.Range.End.ByteCol
	last := len(segs[lastIdx]) + suffix
	if last > MaxLineBytes {
		return true
	}
	for i := 1; i < lastIdx; i++ {
		if len(segs[i]) > MaxLineBytes {
			return true
		}
	}
	return false
}

// applyCore performs the buffer mutation and notifies post-edit
// observers. When record is true the change is pushed to the undo
// stack. Undo/redo replay calls with record=false to avoid
// recursion and to skip filters (mechanical reversal, not user
// input).
//
// Apply pre-clamps the edit; undo/redo replay edits are
// constructed from known-valid ranges but clamped defensively.
func (b *Buffer) applyCore(e Edit, record bool) Change {
	if !record {
		e.Range = b.clampRange(e.Range)
	}

	b.dirty = true
	b.version++
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

	if record && b.undo != nil {
		cur := b.undo.curBefore
		hasCur := b.undo.hasCurBefore
		b.undo.hasCurBefore = false
		b.undo.record(c, cur, hasCur)
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
	first.truncate(r.Start.ByteCol)
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
	// Fast path: no newline avoids bytes.Split allocation.
	if bytes.IndexByte(p, '\n') < 0 {
		b.lines[pos.Line].insert(pos.ByteCol, p)
		return Position{Line: pos.Line, ByteCol: pos.ByteCol + len(p)}
	}
	segs := bytes.Split(p, []byte{'\n'})
	// len(segs) >= 2 guaranteed (we checked for '\n' above).

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

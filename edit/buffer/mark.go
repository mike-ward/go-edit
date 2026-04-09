package buffer

// Gravity determines what happens when an insert occurs exactly
// at a mark's position.
type Gravity int

const (
	// GravityRight means an insert at the mark pushes it right.
	GravityRight Gravity = iota
	// GravityLeft means the mark stays; insert goes to its right.
	GravityLeft
)

// Mark is a tracked position in the buffer. Its position is
// automatically updated by Buffer.Apply.
type Mark struct {
	pos     Position
	gravity Gravity
	id      uint32
}

// Pos returns the mark's current position.
func (m *Mark) Pos() Position { return m.pos }

// Gravity returns the mark's gravity.
func (m *Mark) Gravity() Gravity { return m.gravity }

// ID returns the mark's unique identifier within its MarkSet.
func (m *Mark) ID() uint32 { return m.id }

// TrackedRange is a pair of marks. Start has GravityRight and
// End has GravityLeft so that inserts inside the range expand it.
type TrackedRange struct {
	Start *Mark
	End   *Mark
}

// Range returns the current range as a buffer.Range.
func (tr TrackedRange) Range() Range {
	return Range{Start: tr.Start.pos, End: tr.End.pos}
}

package buffer

import (
	"math/rand"
	"testing"
)

func TestMarkInsertBefore(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	m := buf.Marks().NewMark(Position{0, 3}, GravityRight)
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("XX"),
	})
	if got := m.Pos(); got != (Position{0, 5}) {
		t.Fatalf("got %v, want {0,5}", got)
	}
}

func TestMarkInsertAfter(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	m := buf.Marks().NewMark(Position{0, 2}, GravityRight)
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 4}, End: Position{0, 4}},
		NewBytes: []byte("!!"),
	})
	if got := m.Pos(); got != (Position{0, 2}) {
		t.Fatalf("got %v, want {0,2}", got)
	}
}

func TestMarkInsertAtGravityRight(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	m := buf.Marks().NewMark(Position{0, 3}, GravityRight)
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 3}},
		NewBytes: []byte("X"),
	})
	// GravityRight: insert pushes mark right.
	if got := m.Pos(); got != (Position{0, 4}) {
		t.Fatalf("got %v, want {0,4}", got)
	}
}

func TestMarkInsertAtGravityLeft(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	m := buf.Marks().NewMark(Position{0, 3}, GravityLeft)
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 3}},
		NewBytes: []byte("X"),
	})
	// GravityLeft: mark stays.
	if got := m.Pos(); got != (Position{0, 3}) {
		t.Fatalf("got %v, want {0,3}", got)
	}
}

func TestMarkDeleteContaining(t *testing.T) {
	buf := FromBytes([]byte("hello world"))
	m := buf.Marks().NewMark(Position{0, 6}, GravityRight)
	buf.Apply(Edit{
		Range: Range{Start: Position{0, 3}, End: Position{0, 9}},
	})
	// Mark inside deleted range collapses.
	got := m.Pos()
	if got.Line != 0 {
		t.Fatalf("line = %d", got.Line)
	}
	// GravityRight inside delete → collapse to endPos (= delStart
	// since no insert).
	if got != (Position{0, 3}) {
		t.Fatalf("got %v, want {0,3}", got)
	}
}

func TestMarkMultiLineInsert(t *testing.T) {
	buf := FromBytes([]byte("aaa\nbbb\nccc"))
	m := buf.Marks().NewMark(Position{2, 1}, GravityRight)
	// Insert newline at start of line 1.
	buf.Apply(Edit{
		Range:    Range{Start: Position{1, 0}, End: Position{1, 0}},
		NewBytes: []byte("XX\nYY\n"),
	})
	// Line 2 ("ccc") shifted down by 2 lines.
	if got := m.Pos(); got != (Position{4, 1}) {
		t.Fatalf("got %v, want {4,1}", got)
	}
}

func TestMarkMultiLineDelete(t *testing.T) {
	buf := FromBytes([]byte("aaa\nbbb\nccc\nddd"))
	m := buf.Marks().NewMark(Position{3, 1}, GravityRight)
	// Delete lines 1-2 entirely.
	buf.Apply(Edit{
		Range: Range{Start: Position{0, 3}, End: Position{2, 3}},
	})
	// "ddd" was line 3, now line 1 (deleted 2 lines). Col unchanged.
	if got := m.Pos(); got != (Position{1, 1}) {
		t.Fatalf("got %v, want {1,1}", got)
	}
}

func TestTrackedRangeExpands(t *testing.T) {
	buf := FromBytes([]byte("hello world"))
	tr := buf.Marks().NewRange(Position{0, 2}, Position{0, 7})
	// Insert inside range.
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 4}, End: Position{0, 4}},
		NewBytes: []byte("XXX"),
	})
	r := tr.Range()
	// Start has GravityRight but insert is after start → stays.
	if r.Start != (Position{0, 2}) {
		t.Fatalf("start = %v, want {0,2}", r.Start)
	}
	// End has GravityLeft: insert before end pushes end right.
	if r.End != (Position{0, 10}) {
		t.Fatalf("end = %v, want {0,10}", r.End)
	}
}

func TestMarkReplaceThroughMark(t *testing.T) {
	buf := FromBytes([]byte("hello world"))
	mR := buf.Marks().NewMark(Position{0, 7}, GravityRight)
	mL := buf.Marks().NewMark(Position{0, 7}, GravityLeft)
	// Replace "lo wo" (3..8) with "XY".
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 8}},
		NewBytes: []byte("XY"),
	})
	// "helXYrld" — mark was inside deleted range.
	// GravityRight → collapse to endPos (after "XY" = col 5).
	if got := mR.Pos(); got != (Position{0, 5}) {
		t.Fatalf("GravityRight: got %v, want {0,5}", got)
	}
	// GravityLeft → collapse to delStart (col 3).
	if got := mL.Pos(); got != (Position{0, 3}) {
		t.Fatalf("GravityLeft: got %v, want {0,3}", got)
	}
}

func TestMarkAtDeleteEnd(t *testing.T) {
	buf := FromBytes([]byte("abcdef"))
	m := buf.Marks().NewMark(Position{0, 4}, GravityRight)
	// Delete "bc" (1..3). Mark at col 4 is after delEnd.
	buf.Apply(Edit{
		Range: Range{Start: Position{0, 1}, End: Position{0, 3}},
	})
	// "adef" — mark shifts left by 2.
	if got := m.Pos(); got != (Position{0, 2}) {
		t.Fatalf("got %v, want {0,2}", got)
	}
}

func TestMarkID(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	ms := buf.Marks()
	m1 := ms.NewMark(Position{0, 0}, GravityRight)
	m2 := ms.NewMark(Position{0, 1}, GravityRight)
	if m1.ID() == m2.ID() {
		t.Fatal("marks should have distinct IDs")
	}
	if m1.ID() == 0 || m2.ID() == 0 {
		t.Fatal("mark ID should not be zero")
	}
}

func TestTrackedRange_Range(t *testing.T) {
	buf := FromBytes([]byte("hello world"))
	tr := buf.Marks().NewRange(Position{0, 2}, Position{0, 7})
	r := tr.Range()
	if r.Start != (Position{0, 2}) || r.End != (Position{0, 7}) {
		t.Fatalf("got %v, want [{0,2},{0,7})", r)
	}
}

func TestMarkSetLenAfterAddRemove(t *testing.T) {
	ms := &MarkSet{}
	if ms.Len() != 0 {
		t.Fatalf("empty: %d", ms.Len())
	}
	m1 := ms.NewMark(Position{0, 0}, GravityRight)
	m2 := ms.NewMark(Position{0, 1}, GravityRight)
	if ms.Len() != 2 {
		t.Fatalf("after add 2: %d", ms.Len())
	}
	ms.Remove(m1)
	if ms.Len() != 1 {
		t.Fatalf("after remove 1: %d", ms.Len())
	}
	ms.Remove(m2)
	if ms.Len() != 0 {
		t.Fatalf("after remove 2: %d", ms.Len())
	}
}

func TestBufferMarksLazyInit(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	ms1 := buf.Marks()
	ms2 := buf.Marks()
	if ms1 != ms2 {
		t.Fatal("Marks() should return same instance")
	}
}

func TestMarkRemove(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	ms := buf.Marks()
	m := ms.NewMark(Position{0, 2}, GravityRight)
	if ms.Len() != 1 {
		t.Fatalf("len = %d", ms.Len())
	}
	ms.Remove(m)
	if ms.Len() != 0 {
		t.Fatalf("len after remove = %d", ms.Len())
	}
}

func TestMarkRemoveNil(t *testing.T) {
	ms := &MarkSet{}
	ms.Remove(nil) // should not panic
}

func TestMarkStress(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	buf := FromBytes([]byte("line one\nline two\nline three\nline four"))
	ms := buf.Marks()

	// Place 1000 marks at random valid positions.
	marks := make([]*Mark, 1000)
	for i := range marks {
		line := rng.Intn(buf.LineCount())
		col := 0
		if ll := len(buf.Line(line)); ll > 0 {
			col = rng.Intn(ll + 1)
		}
		g := GravityRight
		if rng.Intn(2) == 0 {
			g = GravityLeft
		}
		marks[i] = ms.NewMark(Position{line, col}, g)
	}

	// Apply 100 random edits.
	for range 100 {
		line := rng.Intn(buf.LineCount())
		col := 0
		if ll := len(buf.Line(line)); ll > 0 {
			col = rng.Intn(ll + 1)
		}
		e := Edit{
			Range: Range{
				Start: Position{line, col},
				End:   Position{line, col},
			},
		}
		switch rng.Intn(3) {
		case 0: // insert
			e.NewBytes = []byte("xyz")
		case 1: // insert newline
			e.NewBytes = []byte("\n")
		case 2: // delete forward 1 byte
			if col < len(buf.Line(line)) {
				e.Range.End.ByteCol = col + 1
			}
		}
		buf.Apply(e)
	}

	// All marks should have valid positions.
	for i, m := range marks {
		p := m.Pos()
		if p.Line < 0 || p.Line >= buf.LineCount() {
			t.Fatalf("mark %d: line %d out of range [0,%d)",
				i, p.Line, buf.LineCount())
		}
		if p.ByteCol < 0 || p.ByteCol > len(buf.Line(p.Line)) {
			t.Fatalf("mark %d: col %d out of range [0,%d]",
				i, p.ByteCol, len(buf.Line(p.Line)))
		}
	}
}

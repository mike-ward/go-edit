package buffer

import "testing"

func TestMarkSet_MaxMarks(t *testing.T) {
	ms := &MarkSet{}
	// Don't actually create 1M marks; just test the cap check.
	ms.marks = make([]*Mark, MaxMarks)
	m := ms.NewMark(Position{0, 0}, GravityRight)
	if m != nil {
		t.Fatal("expected nil when at MaxMarks")
	}
}

func TestMarkSet_IDWrap(t *testing.T) {
	ms := &MarkSet{nextID: ^uint32(0)} // max uint32
	m := ms.NewMark(Position{0, 0}, GravityRight)
	if m == nil {
		t.Fatal("nil mark")
	}
	if m.ID() == 0 {
		t.Fatal("ID should skip zero on wrap")
	}
}

func TestMarkSet_RemoveFromEmpty(t *testing.T) {
	ms := &MarkSet{}
	ms.Remove(&Mark{}) // should not panic
}

func TestMarkSet_AdjustNilReceiver(t *testing.T) {
	var ms *MarkSet
	// Should not panic.
	ms.adjust(Edit{}, Position{})
}

func TestMarkSet_NewRangeNilOnCap(t *testing.T) {
	ms := &MarkSet{}
	ms.marks = make([]*Mark, MaxMarks)
	tr := ms.NewRange(Position{0, 0}, Position{0, 5})
	if tr.Start != nil || tr.End != nil {
		t.Fatal("expected nil marks when at cap")
	}
}

package buffer

import "testing"

func TestAddFilter_Nil(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	remove := buf.AddFilter(nil)
	remove() // should not panic
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("x"),
	})
	if buf.String() != "xhello" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestOnEdit_Nil(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	remove := buf.OnEdit(nil)
	remove() // should not panic
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("x"),
	})
}

func TestAddFilter_DoubleRemove(t *testing.T) {
	buf := FromBytes([]byte("ab"))
	count := 0
	remove := buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		count++
		return FilterAccept
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 2}, End: Position{0, 2}},
		NewBytes: []byte("c"),
	})
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	remove()
	remove() // double remove — must not nil another slot
	// Add a second filter; it should work.
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		count += 10
		return FilterAccept
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 3}},
		NewBytes: []byte("d"),
	})
	if count != 11 {
		t.Fatalf("count after double-remove = %d, want 11", count)
	}
}

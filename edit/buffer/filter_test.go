package buffer

import (
	"bytes"
	"testing"
)

func TestFilterObserve(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	count := 0
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		count++
		return FilterAccept
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 5}, End: Position{0, 5}},
		NewBytes: []byte("!"),
	})
	if count != 1 {
		t.Fatalf("filter called %d times, want 1", count)
	}
	if buf.String() != "hello!" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestFilterTransform(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	buf.AddFilter(func(_ *Buffer, e *Edit) FilterResult {
		e.NewBytes = bytes.ToUpper(e.NewBytes)
		return FilterAccept
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 5}, End: Position{0, 5}},
		NewBytes: []byte(" world"),
	})
	if got := buf.String(); got != "hello WORLD" {
		t.Fatalf("got %q", got)
	}
}

func TestFilterReject(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	buf.AddFilter(func(_ *Buffer, e *Edit) FilterResult {
		if !e.Range.Empty() {
			return FilterReject
		}
		return FilterAccept
	})
	// Delete should be rejected.
	buf.Apply(Edit{Range: Range{
		Start: Position{0, 0}, End: Position{0, 3},
	}})
	if got := buf.String(); got != "hello" {
		t.Fatalf("delete not rejected: got %q", got)
	}
	// Insert should pass.
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("!"),
	})
	if got := buf.String(); got != "!hello" {
		t.Fatalf("insert rejected: got %q", got)
	}
}

func TestFilterRejectNotDirty(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		return FilterReject
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("x"),
	})
	if buf.Dirty() {
		t.Fatal("vetoed edit should not dirty buffer")
	}
}

func TestFilterChainOrder(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	var order []int
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		order = append(order, 1)
		return FilterAccept
	})
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		order = append(order, 2)
		return FilterReject
	})
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("x"),
	})
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("order = %v, want [1, 2]", order)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("edit not vetoed: got %q", got)
	}
}

func TestFilterRemove(t *testing.T) {
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
		t.Fatalf("count = %d, want 1", count)
	}
	remove()
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 3}},
		NewBytes: []byte("d"),
	})
	if count != 1 {
		t.Fatalf("count after remove = %d, want 1", count)
	}
}

func TestPostEditObserver(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	var got Change
	buf.OnEdit(func(c Change) { got = c })
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 5}, End: Position{0, 5}},
		NewBytes: []byte("!"),
	})
	if got.AppliedRange.End.ByteCol != 6 {
		t.Fatalf("observer got end col %d, want 6",
			got.AppliedRange.End.ByteCol)
	}
}

func TestPostEditObserverNotCalledOnReject(t *testing.T) {
	buf := FromBytes([]byte("hello"))
	buf.AddFilter(func(_ *Buffer, _ *Edit) FilterResult {
		return FilterReject
	})
	called := false
	buf.OnEdit(func(_ Change) { called = true })
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 0}, End: Position{0, 0}},
		NewBytes: []byte("x"),
	})
	if called {
		t.Fatal("observer should not fire on rejected edit")
	}
}

func TestPostEditRemove(t *testing.T) {
	buf := FromBytes([]byte("ab"))
	count := 0
	remove := buf.OnEdit(func(_ Change) { count++ })
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 2}, End: Position{0, 2}},
		NewBytes: []byte("c"),
	})
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	remove()
	buf.Apply(Edit{
		Range:    Range{Start: Position{0, 3}, End: Position{0, 3}},
		NewBytes: []byte("d"),
	})
	if count != 1 {
		t.Fatalf("count after remove = %d", count)
	}
}

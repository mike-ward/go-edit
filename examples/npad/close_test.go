package main

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func newCleanState(t *testing.T) *appState {
	t.Helper()
	return &appState{Buf: buffer.New()}
}

func newDirtyState(t *testing.T) *appState {
	t.Helper()
	s := &appState{Buf: buffer.New()}
	pos := buffer.Position{Line: 0, ByteCol: 0}
	s.Buf.Apply(buffer.Edit{
		Range:    buffer.Range{Start: pos, End: pos},
		NewBytes: []byte("x"),
	})
	if !s.Buf.Dirty() {
		t.Fatal("setup: buffer should be dirty after Apply")
	}
	return s
}

func TestDecideClose_NilState(t *testing.T) {
	if decideClose(nil) != closeNow {
		t.Fatal("nil state must close immediately")
	}
}

func TestDecideClose_NilBuffer(t *testing.T) {
	if decideClose(&appState{}) != closeNow {
		t.Fatal("nil Buf must close immediately")
	}
}

func TestDecideClose_CleanBuffer(t *testing.T) {
	if got := decideClose(newCleanState(t)); got != closeNow {
		t.Fatalf("clean buffer: got %d, want closeNow", got)
	}
}

func TestDecideClose_DirtyBuffer(t *testing.T) {
	if got := decideClose(newDirtyState(t)); got != closePrompt {
		t.Fatalf("dirty buffer: got %d, want closePrompt", got)
	}
}

func TestDecideClose_DirtyButClosing(t *testing.T) {
	s := newDirtyState(t)
	s.closing = true
	if got := decideClose(s); got != closeIgnore {
		t.Fatalf("closing=true: got %d, want closeIgnore", got)
	}
}

func TestDecideClose_CleanButClosing(t *testing.T) {
	// Guard runs before dirty check; reentrancy suppression wins.
	s := newCleanState(t)
	s.closing = true
	if got := decideClose(s); got != closeIgnore {
		t.Fatalf("closing=true clean: got %d, want closeIgnore", got)
	}
}

func TestShouldFinishClose_NilState(t *testing.T) {
	if !shouldFinishClose(nil) {
		t.Fatal("nil state should finish close")
	}
}

func TestShouldFinishClose_NilBuffer(t *testing.T) {
	if !shouldFinishClose(&appState{}) {
		t.Fatal("nil Buf should finish close")
	}
}

func TestShouldFinishClose_SaveSucceeded(t *testing.T) {
	// Simulate: was dirty, save ran, MarkClean called.
	s := newDirtyState(t)
	s.Buf.MarkClean()
	if !shouldFinishClose(s) {
		t.Fatal("clean after save: must finish close")
	}
}

func TestShouldFinishClose_SaveFailed(t *testing.T) {
	// Buffer still dirty → save failed; keep window open.
	s := newDirtyState(t)
	if shouldFinishClose(s) {
		t.Fatal("dirty after save: must NOT finish close")
	}
}

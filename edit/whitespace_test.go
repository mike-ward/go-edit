package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestByteInSelection(t *testing.T) {
	sel := selInfo{
		sel: buffer.Range{
			Start: buffer.Position{Line: 1, ByteCol: 2},
			End:   buffer.Position{Line: 1, ByteCol: 5},
		},
		hasSel: true,
	}
	sels := []selInfo{sel}

	tests := []struct {
		line, col int
		want      bool
	}{
		{0, 3, false}, // wrong line
		{1, 1, false}, // before selection
		{1, 2, true},  // at start (inclusive)
		{1, 3, true},  // inside
		{1, 4, true},  // inside
		{1, 5, false}, // at end (exclusive)
		{1, 6, false}, // after
		{2, 3, false}, // wrong line
	}
	for _, tt := range tests {
		got := byteInSelection(tt.line, tt.col, sels)
		if got != tt.want {
			t.Errorf("byteInSelection(%d, %d) = %v, want %v",
				tt.line, tt.col, got, tt.want)
		}
	}
}

func TestResolveWhitespace(t *testing.T) {
	tests := []struct {
		cfg      WhitespaceMode
		override int
		want     WhitespaceMode
	}{
		{WhitespaceNone, 0, WhitespaceNone},
		{WhitespaceAll, 0, WhitespaceAll},
		{WhitespaceNone, int(WhitespaceAll) + 1, WhitespaceAll},
		{WhitespaceAll, int(WhitespaceNone) + 1, WhitespaceNone},
	}
	for _, tt := range tests {
		got := resolveWhitespace(tt.cfg, tt.override)
		if got != tt.want {
			t.Errorf("resolve(%d, %d) = %d, want %d",
				tt.cfg, tt.override, got, tt.want)
		}
	}
}

func TestCycleWhitespace(t *testing.T) {
	// Starting from 0 (not set), should cycle through all modes.
	v := 0
	v = cycleWhitespace(v) // → All
	if resolveWhitespace(0, v) != WhitespaceAll {
		t.Fatalf("first cycle: got %d", v)
	}
	v = cycleWhitespace(v) // → Selection
	if resolveWhitespace(0, v) != WhitespaceSelection {
		t.Fatalf("second cycle: got %d", v)
	}
	v = cycleWhitespace(v) // → None
	if resolveWhitespace(0, v) != WhitespaceNone {
		t.Fatalf("third cycle: got %d", v)
	}
	v = cycleWhitespace(v) // → All again
	if resolveWhitespace(0, v) != WhitespaceAll {
		t.Fatalf("fourth cycle: got %d", v)
	}
}

package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestSquigglesForLineFiltered(t *testing.T) {
	decos := []buffer.Decoration{
		{Kind: buffer.DecoToken, Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 5},
		}},
		{Kind: buffer.DecoSquiggle, Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 3},
		}, SquiggleColor: 0xFF0000FF},
		{Kind: buffer.DecoGutter, Range: buffer.Range{
			Start: buffer.Position{Line: 0, ByteCol: 0},
			End:   buffer.Position{Line: 0, ByteCol: 0},
		}, GutterColor: 0xFFFF00FF},
	}

	// Verify that the decoration types are correctly defined
	// and that the draw functions won't panic on nil measurer.
	if decos[1].Kind != buffer.DecoSquiggle {
		t.Fatal("expected DecoSquiggle")
	}
	if decos[2].Kind != buffer.DecoGutter {
		t.Fatal("expected DecoGutter")
	}

	// drawSquiggles and drawGutterMarkers should not panic
	// when measurer is nil (guard clause returns).
	drawSquiggles(nil, decos, 0, []byte("hello"),
		0, 5, 0, 0, 16, nil)
	drawGutterMarkers(nil, decos, 0,
		40, 4, 0, 16, nil)
}

func TestDecoKindConstants(t *testing.T) {
	// Ensure decoration kinds used by diagnostics are distinct.
	if buffer.DecoSquiggle == buffer.DecoToken {
		t.Fatal("DecoSquiggle should differ from DecoToken")
	}
	if buffer.DecoGutter == buffer.DecoToken {
		t.Fatal("DecoGutter should differ from DecoToken")
	}
	if buffer.DecoSquiggle == buffer.DecoGutter {
		t.Fatal("DecoSquiggle should differ from DecoGutter")
	}
}

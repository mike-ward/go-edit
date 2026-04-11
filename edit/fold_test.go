package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestFoldRangeAt(t *testing.T) {
	buf := bufFromLines(
		"func main() {",
		"    a := 1",
		"    b := 2",
		"}",
	)
	fr, ok := foldRangeAt(buf, 0, 4)
	if !ok {
		t.Fatal("expected foldable")
	}
	if fr.StartLine != 0 || fr.EndLine != 2 {
		t.Fatalf("got %+v", fr)
	}
}

func TestFoldRangeAt_NotFoldable(t *testing.T) {
	buf := bufFromLines("a", "b", "c")
	_, ok := foldRangeAt(buf, 0, 4)
	if ok {
		t.Fatal("flat code should not be foldable")
	}
}

func TestFoldRangeAt_Nested(t *testing.T) {
	buf := bufFromLines(
		"if true {",
		"    if false {",
		"        x",
		"    }",
		"}",
	)
	// Fold at line 0 should cover lines 1-3.
	fr, ok := foldRangeAt(buf, 0, 4)
	if !ok {
		t.Fatal("expected foldable")
	}
	if fr.EndLine != 3 {
		t.Fatalf("got end %d, want 3", fr.EndLine)
	}
	// Fold at line 1 should cover lines 2-2.
	fr2, ok := foldRangeAt(buf, 1, 4)
	if !ok {
		t.Fatal("expected nested foldable")
	}
	if fr2.StartLine != 1 || fr2.EndLine != 2 {
		t.Fatalf("got %+v", fr2)
	}
}

func TestFoldRangeAt_BlankLines(t *testing.T) {
	buf := bufFromLines(
		"func f() {",
		"    a",
		"",
		"    b",
		"}",
	)
	fr, ok := foldRangeAt(buf, 0, 4)
	if !ok {
		t.Fatal("expected foldable")
	}
	// Should include lines through the last indented line.
	if fr.EndLine != 3 {
		t.Fatalf("got end %d, want 3", fr.EndLine)
	}
}

func TestFoldRangeAt_LastLine(t *testing.T) {
	buf := bufFromLines("a")
	_, ok := foldRangeAt(buf, 0, 4)
	if ok {
		t.Fatal("last line cannot be foldable")
	}
}

func TestToggleFold(t *testing.T) {
	buf := bufFromLines(
		"func main() {",
		"    x",
		"}",
	)
	var folds []FoldRange
	folds = toggleFold(folds, buf, 0, 4)
	if len(folds) != 1 {
		t.Fatalf("expected 1 fold, got %d", len(folds))
	}
	// Toggle again → remove.
	folds = toggleFold(folds, buf, 0, 4)
	if len(folds) != 0 {
		t.Fatalf("expected 0 folds, got %d", len(folds))
	}
}

func TestFoldAll(t *testing.T) {
	buf := bufFromLines(
		"func a() {",
		"    x",
		"}",
		"func b() {",
		"    y",
		"}",
	)
	folds := foldAll(buf, 4)
	if len(folds) != 2 {
		t.Fatalf("expected 2 folds, got %d", len(folds))
	}
}

func TestIsFolded(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	if isFolded(folds, 1) {
		t.Error("header should not be folded")
	}
	if !isFolded(folds, 2) {
		t.Error("line 2 should be folded")
	}
	if !isFolded(folds, 3) {
		t.Error("line 3 should be folded")
	}
	if isFolded(folds, 4) {
		t.Error("line 4 should not be folded")
	}
}

func TestNextVisible(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	if got := nextVisible(folds, 0); got != 0 {
		t.Errorf("got %d", got)
	}
	if got := nextVisible(folds, 1); got != 1 {
		t.Errorf("header: got %d", got)
	}
	if got := nextVisible(folds, 2); got != 4 {
		t.Errorf("inside fold: got %d, want 4", got)
	}
}

func TestPrevVisible(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	if got := prevVisible(folds, 4); got != 4 {
		t.Errorf("got %d", got)
	}
	if got := prevVisible(folds, 3); got != 1 {
		t.Errorf("inside fold: got %d, want 1", got)
	}
	if got := prevVisible(folds, 1); got != 1 {
		t.Errorf("header: got %d, want 1", got)
	}
}

func TestVisibleLineCount(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	got := visibleLineCount(10, folds)
	if got != 8 {
		t.Fatalf("got %d, want 8", got)
	}
}

func TestVisibleToLogical(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	// vis 0 → logical 0
	// vis 1 → logical 1 (fold header)
	// vis 2 → logical 4 (after fold)
	tests := []struct{ vis, logical int }{
		{0, 0},
		{1, 1},
		{2, 4},
		{3, 5},
	}
	for _, tt := range tests {
		got := visibleToLogical(tt.vis, folds)
		if got != tt.logical {
			t.Errorf("vis %d → logical %d, want %d",
				tt.vis, got, tt.logical)
		}
	}
}

func TestLogicalToVisible(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	tests := []struct{ logical, vis int }{
		{0, 0},
		{1, 1},
		{4, 2},
		{5, 3},
	}
	for _, tt := range tests {
		got := logicalToVisible(tt.logical, folds)
		if got != tt.vis {
			t.Errorf("logical %d → vis %d, want %d",
				tt.logical, got, tt.vis)
		}
	}
}

func TestSnapCursorOutOfFold(t *testing.T) {
	folds := []FoldRange{{StartLine: 1, EndLine: 3}}
	cs := CursorState{
		Cursor: buffer.Position{Line: 2, ByteCol: 5},
	}
	snapCursorOutOfFold(&cs, folds)
	if cs.Cursor.Line != 1 || cs.Cursor.ByteCol != 0 {
		t.Fatalf("got %v", cs.Cursor)
	}
}

func TestInvalidateFolds(t *testing.T) {
	folds := []FoldRange{
		{StartLine: 1, EndLine: 3},
		{StartLine: 5, EndLine: 7},
	}
	c := buffer.Change{
		AppliedRange: buffer.Range{
			Start: buffer.Position{Line: 2},
			End:   buffer.Position{Line: 2, ByteCol: 1},
		},
	}
	result := invalidateFolds(folds, c)
	if len(result) != 1 {
		t.Fatalf("expected 1 fold, got %d", len(result))
	}
	if result[0].StartLine != 5 {
		t.Fatalf("wrong fold remaining: %+v", result[0])
	}
}

func TestCheckFoldInvariant_DetectsBreaks(t *testing.T) {
	cases := []struct {
		name  string
		folds []FoldRange
		want  string // "" = valid
	}{
		{"empty", nil, ""},
		{"single", []FoldRange{{StartLine: 0, EndLine: 3}}, ""},
		{"valid multi",
			[]FoldRange{
				{StartLine: 0, EndLine: 2},
				{StartLine: 5, EndLine: 7},
				{StartLine: 10, EndLine: 10},
			}, ""},
		{"end < start",
			[]FoldRange{{StartLine: 3, EndLine: 1}},
			"end < start"},
		{"unsorted",
			[]FoldRange{
				{StartLine: 5, EndLine: 6},
				{StartLine: 2, EndLine: 4},
			},
			"unsorted StartLine"},
		{"duplicate start",
			[]FoldRange{
				{StartLine: 0, EndLine: 2},
				{StartLine: 0, EndLine: 2},
			},
			"unsorted StartLine"},
		{"overlapping",
			[]FoldRange{
				{StartLine: 0, EndLine: 5},
				{StartLine: 3, EndLine: 8},
			},
			"overlapping ranges"},
		{"touching (overlap)",
			[]FoldRange{
				{StartLine: 0, EndLine: 5},
				{StartLine: 5, EndLine: 8},
			},
			"overlapping ranges"},
	}
	for _, tt := range cases {
		got := checkFoldInvariant(tt.folds)
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// linearIsFolded is the pre-refactor reference implementation; used
// as a differential oracle in TestFold_BinarySearchMatchesLinear.
func linearIsFolded(folds []FoldRange, line int) bool {
	for _, f := range folds {
		if line > f.StartLine && line <= f.EndLine {
			return true
		}
	}
	return false
}

func linearNextVisible(folds []FoldRange, line int) int {
	for _, f := range folds {
		if line > f.StartLine && line <= f.EndLine {
			return f.EndLine + 1
		}
	}
	return line
}

func linearPrevVisible(folds []FoldRange, line int) int {
	for i := len(folds) - 1; i >= 0; i-- {
		f := folds[i]
		if line > f.StartLine && line <= f.EndLine {
			return f.StartLine
		}
	}
	return line
}

func linearIsFoldHeader(folds []FoldRange, line int) bool {
	for _, f := range folds {
		if f.StartLine == line {
			return true
		}
	}
	return false
}

func TestFold_BinarySearchMatchesLinear(t *testing.T) {
	// Build a deterministic collection of non-overlapping sorted
	// fold sets and verify binary search matches the linear oracle
	// across every queried line.
	cases := [][]FoldRange{
		{},
		{{StartLine: 0, EndLine: 3}},
		{{StartLine: 5, EndLine: 9}},
		{{StartLine: 0, EndLine: 2}, {StartLine: 4, EndLine: 6}, {StartLine: 10, EndLine: 15}},
		{{StartLine: 1, EndLine: 1}, {StartLine: 3, EndLine: 8}, {StartLine: 9, EndLine: 9}},
	}
	for _, folds := range cases {
		if msg := checkFoldInvariant(folds); msg != "" {
			t.Fatalf("bad fixture: %s: %+v", msg, folds)
		}
	}
	for idx, folds := range cases {
		for line := -1; line <= 20; line++ {
			if got, want := isFolded(folds, line), linearIsFolded(folds, line); got != want {
				t.Fatalf("case %d line %d: isFolded=%v want %v", idx, line, got, want)
			}
			if got, want := nextVisible(folds, line), linearNextVisible(folds, line); got != want {
				t.Fatalf("case %d line %d: nextVisible=%d want %d", idx, line, got, want)
			}
			if got, want := prevVisible(folds, line), linearPrevVisible(folds, line); got != want {
				t.Fatalf("case %d line %d: prevVisible=%d want %d", idx, line, got, want)
			}
			if got, want := isFoldHeader(folds, line), linearIsFoldHeader(folds, line); got != want {
				t.Fatalf("case %d line %d: isFoldHeader=%v want %v", idx, line, got, want)
			}
		}
	}
}

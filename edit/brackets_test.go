package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func bufFromLines(lines ...string) *buffer.Buffer {
	buf := buffer.New()
	for i, l := range lines {
		if i > 0 {
			buf.Apply(buffer.Edit{
				Range:    buffer.Range{Start: endOfBuf(buf), End: endOfBuf(buf)},
				NewBytes: []byte("\n"),
			})
		}
		if len(l) > 0 {
			buf.Apply(buffer.Edit{
				Range:    buffer.Range{Start: endOfBuf(buf), End: endOfBuf(buf)},
				NewBytes: []byte(l),
			})
		}
	}
	return buf
}

func endOfBuf(buf *buffer.Buffer) buffer.Position {
	last := buf.LineCount() - 1
	return buffer.Position{Line: last, ByteCol: len(buf.Line(last))}
}

func TestFindMatchingBracket(t *testing.T) {
	tests := []struct {
		name   string
		lines  []string
		cursor buffer.Position
		want   buffer.Position
		wantOK bool
	}{
		{
			name:   "parens simple",
			lines:  []string{"(hello)"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 6},
			wantOK: true,
		},
		{
			name:   "parens from closer",
			lines:  []string{"(hello)"},
			cursor: buffer.Position{Line: 0, ByteCol: 6},
			want:   buffer.Position{Line: 0, ByteCol: 0},
			wantOK: true,
		},
		{
			name:   "cursor after closer",
			lines:  []string{"(hello)"},
			cursor: buffer.Position{Line: 0, ByteCol: 7},
			want:   buffer.Position{Line: 0, ByteCol: 0},
			wantOK: true,
		},
		{
			name:   "nested",
			lines:  []string{"(([]))"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 5},
			wantOK: true,
		},
		{
			name:   "nested inner",
			lines:  []string{"(([]))"},
			cursor: buffer.Position{Line: 0, ByteCol: 1},
			want:   buffer.Position{Line: 0, ByteCol: 4},
			wantOK: true,
		},
		{
			name:   "brackets",
			lines:  []string{"[a, b]"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 5},
			wantOK: true,
		},
		{
			name:   "braces",
			lines:  []string{"{x}"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 2},
			wantOK: true,
		},
		{
			name:   "cross line",
			lines:  []string{"func() {", "  return", "}"},
			cursor: buffer.Position{Line: 0, ByteCol: 7},
			want:   buffer.Position{Line: 2, ByteCol: 0},
			wantOK: true,
		},
		{
			name:   "cross line backward",
			lines:  []string{"func() {", "  return", "}"},
			cursor: buffer.Position{Line: 2, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 7},
			wantOK: true,
		},
		{
			name:   "unmatched opener",
			lines:  []string{"(hello"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			wantOK: false,
		},
		{
			name:   "unmatched closer",
			lines:  []string{"hello)"},
			cursor: buffer.Position{Line: 0, ByteCol: 5},
			wantOK: false,
		},
		{
			name:   "no bracket at cursor",
			lines:  []string{"hello"},
			cursor: buffer.Position{Line: 0, ByteCol: 2},
			wantOK: false,
		},
		{
			name:   "at buffer start",
			lines:  []string{"(a)"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 2},
			wantOK: true,
		},
		{
			name:   "at buffer end",
			lines:  []string{"(a)"},
			cursor: buffer.Position{Line: 0, ByteCol: 3}, // after ')'
			want:   buffer.Position{Line: 0, ByteCol: 0},
			wantOK: true,
		},
		{
			name:   "mixed bracket types",
			lines:  []string{"([{}])"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 5},
			wantOK: true,
		},
		{
			name:   "empty parens",
			lines:  []string{"()"},
			cursor: buffer.Position{Line: 0, ByteCol: 0},
			want:   buffer.Position{Line: 0, ByteCol: 1},
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bufFromLines(tt.lines...)
			_, got, ok, capped := findMatchingBracket(buf, tt.cursor)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if capped {
				t.Fatalf("unexpected capped=true for small input")
			}
			if ok && got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBracketScanCap(t *testing.T) {
	// Build a line longer than maxBracketScan with no closer.
	long := make([]byte, maxBracketScan+100)
	long[0] = '('
	for i := 1; i < len(long); i++ {
		long[i] = 'x'
	}
	buf := bufFromLines(string(long))
	_, _, ok, capped := findMatchingBracket(buf, buffer.Position{})
	if ok {
		t.Fatal("expected no match beyond scan cap")
	}
	if !capped {
		t.Fatal("expected capped=true when scan hits cap")
	}
}

func TestBracketScanCap_BackwardDirection(t *testing.T) {
	// A closer at the end of a long line with no opener should
	// also report capped.
	long := make([]byte, maxBracketScan+100)
	for i := range long {
		long[i] = 'x'
	}
	long[len(long)-1] = ')'
	buf := bufFromLines(string(long))
	_, _, ok, capped := findMatchingBracket(buf,
		buffer.Position{Line: 0, ByteCol: len(long) - 1})
	if ok {
		t.Fatal("expected no match for unbalanced closer")
	}
	if !capped {
		t.Fatal("expected capped=true on backward scan hitting cap")
	}
}

package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestAutoCloseFilter_InsertOpener(t *testing.T) {
	buf := buffer.New()
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	// Insert '(' at empty buffer → should get "()"
	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("("),
	})
	if got := buf.String(); got != "()" {
		t.Fatalf("got %q, want %q", got, "()")
	}
}

func TestAutoCloseFilter_InsertBracket(t *testing.T) {
	buf := buffer.New()
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("["),
	})
	if got := buf.String(); got != "[]" {
		t.Fatalf("got %q, want %q", got, "[]")
	}
}

func TestAutoCloseFilter_InsertBrace(t *testing.T) {
	buf := buffer.New()
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("{"),
	})
	if got := buf.String(); got != "{}" {
		t.Fatalf("got %q, want %q", got, "{}")
	}
}

func TestAutoCloseFilter_NoCloseBeforeAlpha(t *testing.T) {
	// If next char is alphanumeric, don't auto-close.
	buf := bufFromLines("abc")
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("("),
	})
	if got := buf.String(); got != "(abc" {
		t.Fatalf("got %q, want %q", got, "(abc")
	}
}

func TestAutoCloseFilter_CloseBeforeCloser(t *testing.T) {
	// If next char is a closer, auto-close is allowed.
	buf := bufFromLines(")")
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("("),
	})
	if got := buf.String(); got != "())" {
		t.Fatalf("got %q, want %q", got, "())")
	}
}

func TestAutoCloseFilter_QuoteAfterAlpha(t *testing.T) {
	// Quote after alphanumeric → don't auto-close.
	buf := bufFromLines("abc")
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	pos := buffer.Position{Line: 0, ByteCol: 3}
	buf.Apply(buffer.Edit{
		Range:    buffer.Range{Start: pos, End: pos},
		NewBytes: []byte("\""),
	})
	if got := buf.String(); got != "abc\"" {
		t.Fatalf("got %q, want %q", got, "abc\"")
	}
}

func TestAutoCloseFilter_QuoteAtLineStart(t *testing.T) {
	buf := buffer.New()
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("\""),
	})
	if got := buf.String(); got != "\"\"" {
		t.Fatalf("got %q, want %q", got, "\"\"")
	}
}

func TestAutoCloseFilter_PasteIgnored(t *testing.T) {
	// Multi-byte inserts (paste) should not trigger auto-close.
	buf := buffer.New()
	buf.AddFilter(autoCloseFilter(DefaultAutoClosePairs))

	buf.Apply(buffer.Edit{
		Range:    buffer.Range{},
		NewBytes: []byte("(("),
	})
	if got := buf.String(); got != "((" {
		t.Fatalf("got %q, want %q", got, "((")
	}
}

func TestShouldSkipCloser(t *testing.T) {
	buf := bufFromLines("()")
	pos := buffer.Position{Line: 0, ByteCol: 1} // between ( and )

	if !shouldSkipCloser(buf, pos, ')', DefaultAutoClosePairs) {
		t.Fatal("should skip closer")
	}
	if shouldSkipCloser(buf, pos, '(', DefaultAutoClosePairs) {
		t.Fatal("should not skip opener")
	}
	if shouldSkipCloser(buf, pos, 'a', DefaultAutoClosePairs) {
		t.Fatal("should not skip non-closer")
	}
}

func TestShouldDeletePair(t *testing.T) {
	buf := bufFromLines("()")
	pos := buffer.Position{Line: 0, ByteCol: 1} // between ( and )

	if !shouldDeletePair(buf, pos, DefaultAutoClosePairs) {
		t.Fatal("should delete pair")
	}

	// Not at a pair boundary.
	buf2 := bufFromLines("ab")
	if shouldDeletePair(buf2, buffer.Position{Line: 0, ByteCol: 1},
		DefaultAutoClosePairs) {
		t.Fatal("should not delete non-pair")
	}

	// At line start.
	if shouldDeletePair(buf, buffer.Position{Line: 0, ByteCol: 0},
		DefaultAutoClosePairs) {
		t.Fatal("should not delete at col 0")
	}
}

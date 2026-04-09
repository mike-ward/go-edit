package edit

import "testing"

func TestFindScopeHeaders(t *testing.T) {
	buf := bufFromLines(
		"package main",   // 0: indent 0
		"",               // 1
		"func main() {",  // 2: indent 0
		"    if true {",  // 3: indent 4
		"        x := 1", // 4: indent 8
		"        y := 2", // 5: indent 8
		"    }",          // 6: indent 4
		"}",              // 7: indent 0
	)
	// First visible line = 5 (indent 8).
	// Walk back: line 4 (indent 8, not < 8). Line 3 (indent 4 < 8)
	// → header. Line 2 (indent 0 < 4) → header. Stop.
	headers := findScopeHeaders(buf, 5, 5, 4)
	if len(headers) != 2 {
		t.Fatalf("got %d headers, want 2: %v", len(headers), headers)
	}
	if headers[0] != 2 || headers[1] != 3 {
		t.Fatalf("got %v, want [2, 3]", headers)
	}
}

func TestFindScopeHeaders_FirstLine(t *testing.T) {
	buf := bufFromLines("a", "b")
	headers := findScopeHeaders(buf, 0, 5, 4)
	if len(headers) != 0 {
		t.Fatalf("expected no headers at line 0")
	}
}

func TestFindScopeHeaders_Flat(t *testing.T) {
	buf := bufFromLines("a", "b", "c", "d")
	headers := findScopeHeaders(buf, 3, 5, 4)
	if len(headers) != 0 {
		t.Fatalf("expected no headers for flat code")
	}
}

func TestFindScopeHeaders_MaxCap(t *testing.T) {
	buf := bufFromLines(
		"a {",
		"  b {",
		"    c {",
		"      d {",
		"        e",
	)
	headers := findScopeHeaders(buf, 4, 2, 2)
	if len(headers) != 2 {
		t.Fatalf("got %d, want 2 (capped)", len(headers))
	}
}

func TestFindScopeHeaders_SkipBlank(t *testing.T) {
	buf := bufFromLines(
		"func f() {",
		"",
		"    x",
	)
	headers := findScopeHeaders(buf, 2, 5, 4)
	if len(headers) != 1 || headers[0] != 0 {
		t.Fatalf("got %v, want [0]", headers)
	}
}

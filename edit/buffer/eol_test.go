package buffer

import "testing"

func TestDetectEOL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want EOL
	}{
		{"empty", "", EOLUnknown},
		{"no newlines", "hello", EOLUnknown},
		{"LF only", "a\nb\nc\n", EOLLF},
		{"CRLF only", "a\r\nb\r\nc\r\n", EOLCRLF},
		{"CR only", "a\rb\rc\r", EOLCR},
		{"mixed LF+CRLF", "a\nb\r\nc\n", EOLMixed},
		{"mixed CR+LF", "a\rb\nc", EOLMixed},
		{"single LF", "\n", EOLLF},
		{"single CRLF", "\r\n", EOLCRLF},
		{"single CR", "\r", EOLCR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectEOL([]byte(tt.in))
			if got != tt.want {
				t.Errorf("detectEOL(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeEOL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"LF unchanged", "a\nb\n", "a\nb\n"},
		{"CRLF to LF", "a\r\nb\r\n", "a\nb\n"},
		{"CR to LF", "a\rb\r", "a\nb\n"},
		{"mixed", "a\r\nb\rc\n", "a\nb\nc\n"},
		{"empty", "", ""},
		{"no newlines", "hello", "hello"},
		{"lone CR at end", "abc\r", "abc\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(normalizeEOL([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("normalizeEOL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyEOL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		eol  EOL
		want string
	}{
		{"LF passthrough", "a\nb\n", EOLLF, "a\nb\n"},
		{"to CRLF", "a\nb\n", EOLCRLF, "a\r\nb\r\n"},
		{"to CR", "a\nb\n", EOLCR, "a\rb\r"},
		{"unknown passthrough", "a\nb\n", EOLUnknown, "a\nb\n"},
		{"no newlines", "hello", EOLCRLF, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(applyEOL([]byte(tt.in), tt.eol))
			if got != tt.want {
				t.Errorf("applyEOL(%q, %d) = %q, want %q",
					tt.in, tt.eol, got, tt.want)
			}
		})
	}
}

func TestReplaceLF_ExceedsCap(t *testing.T) {
	// Craft a replacement that would exceed maxReplaceLFBytes.
	// A single \n with a huge replacement string.
	data := []byte{'\n'}
	hugeRep := make([]byte, maxReplaceLFBytes+1)
	got := replaceLF(data, hugeRep)
	// Should return input unchanged (refused expansion).
	if &got[0] != &data[0] {
		t.Error("expected original slice returned for pathological expansion")
	}
}

func TestNormalizeApplyRoundTrip(t *testing.T) {
	original := "line1\r\nline2\r\nline3\r\n"
	normalized := normalizeEOL([]byte(original))
	restored := applyEOL(normalized, EOLCRLF)
	if string(restored) != original {
		t.Errorf("round-trip failed: got %q, want %q",
			string(restored), original)
	}
}

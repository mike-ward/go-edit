package buffer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTo_LF(t *testing.T) {
	b, err := Load(strings.NewReader("hello\nworld"))
	if err != nil {
		t.Fatal(err)
	}
	b.Props.FinalNewline = false

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\nworld" {
		t.Errorf("got %q", got)
	}
}

func TestWriteTo_CRLF(t *testing.T) {
	b, err := Load(strings.NewReader("hello\r\nworld\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	b.Props.FinalNewline = false

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\r\nworld\r\n" {
		t.Errorf("got %q, want CRLF preserved", got)
	}
}

func TestWriteTo_FinalNewline(t *testing.T) {
	b, err := Load(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	b.Props.FinalNewline = true

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\n" {
		t.Errorf("got %q, want trailing newline", got)
	}
}

func TestWriteTo_TrimTrailingWS(t *testing.T) {
	b, err := Load(strings.NewReader("hello   \nworld\t\t\n"))
	if err != nil {
		t.Fatal(err)
	}
	b.Props.TrimTrailingWS = true
	b.Props.FinalNewline = false

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\nworld\n" {
		t.Errorf("got %q", got)
	}
}

func TestSaveFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write a CRLF file.
	original := "line1\r\nline2\r\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load, edit, save.
	b, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b.Apply(Edit{
		Range:    Range{Start: Position{0, 5}, End: Position{0, 5}},
		NewBytes: []byte("!"),
	})

	if err := b.SaveFile(""); err != nil {
		t.Fatal(err)
	}

	// Verify CRLF preserved, content modified.
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "line1!\r\nline2\r\n"
	if string(saved) != want {
		t.Errorf("saved = %q, want %q", string(saved), want)
	}
	if b.Dirty() {
		t.Error("buffer should be clean after save")
	}
}

func TestSaveFileNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	b := New()
	b.Apply(Edit{NewBytes: []byte("hello")})
	if err := b.SaveFile(path); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Default: FinalNewline=true, EOL=LF.
	if string(saved) != "hello\n" {
		t.Errorf("saved = %q", string(saved))
	}
}

func TestSaveFileNoPath(t *testing.T) {
	b := New()
	if err := b.SaveFile(""); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestWriteTo_UTF16RoundTrip(t *testing.T) {
	// Build a buffer that looks like it was loaded from UTF-16 LE.
	b := FromBytes([]byte("hello\nworld"))
	b.Props.Encoding = EncodingUTF16LE
	b.Props.HasBOM = true
	b.Props.PreserveBOM = true
	b.Props.FinalNewline = false

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()

	// Should start with UTF-16 LE BOM.
	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xFE {
		t.Fatal("missing UTF-16 LE BOM")
	}

	// Decode back and verify content.
	b2, err := Load(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := b2.String(); got != "hello\nworld" {
		t.Errorf("round-trip content = %q", got)
	}
}

func TestWriteTo_EOLMixed_UsesLF(t *testing.T) {
	b := FromBytes([]byte("a\nb"))
	b.Props.EOL = EOLMixed
	b.Props.FinalNewline = false

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	// Mixed EOL → applyEOL default → LF preserved.
	if got := buf.String(); got != "a\nb" {
		t.Errorf("got %q, want LF", got)
	}
}

func TestTrimTrailingWS_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"whitespace-only line", "  \t \n", "\n"},
		{"empty lines preserved", "\n\n", "\n\n"},
		{"mixed tabs spaces", "a \t \nb\t \n", "a\nb\n"},
		{"no trailing WS", "abc\ndef\n", "abc\ndef\n"},
		{"empty input", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(trimTrailingWhitespace([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSaveFileRoundTrip_UTF16BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utf16.txt")

	// Write a UTF-16 LE BOM file.
	original := []byte{
		0xFF, 0xFE, // BOM
		'H', 0, 'i', 0, '\r', 0, '\n', 0,
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// Load, edit, save.
	b, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Props.Encoding != EncodingUTF16LE {
		t.Fatalf("Encoding = %d, want UTF16LE", b.Props.Encoding)
	}
	b.Apply(Edit{
		Range:    Range{Start: Position{0, 2}, End: Position{0, 2}},
		NewBytes: []byte("!"),
	})

	if err := b.SaveFile(""); err != nil {
		t.Fatal(err)
	}

	// Reload and verify.
	b2, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Props.Encoding != EncodingUTF16LE {
		t.Errorf("re-read Encoding = %d", b2.Props.Encoding)
	}
	if got := b2.String(); got != "Hi!\n" {
		t.Errorf("content = %q", got)
	}
}

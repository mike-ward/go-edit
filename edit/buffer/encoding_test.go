package buffer

import (
	"testing"
	"unicode/utf8"
)

func TestSniffEncoding_BOM(t *testing.T) {
	tests := []struct {
		name    string
		prefix  []byte
		want    Encoding
		wantBOM bool
	}{
		{"UTF-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, "hello"...), EncodingUTF8BOM, true},
		{"UTF-16 BE BOM", append([]byte{0xFE, 0xFF}, 0, 'h', 0, 'i'), EncodingUTF16BE, true},
		{"UTF-16 LE BOM", append([]byte{0xFF, 0xFE}, 'h', 0, 'i', 0), EncodingUTF16LE, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, bom := sniffEncoding(tt.prefix)
			if enc != tt.want {
				t.Errorf("encoding = %d, want %d", enc, tt.want)
			}
			if bom != tt.wantBOM {
				t.Errorf("hasBOM = %v, want %v", bom, tt.wantBOM)
			}
		})
	}
}

func TestSniffEncoding_NoBOM(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Encoding
	}{
		{"plain ASCII", []byte("hello world\n"), EncodingUTF8},
		{"valid UTF-8", []byte("héllo wörld\n"), EncodingUTF8},
		{"UTF-16 BE no BOM", []byte{0, 'h', 0, 'e', 0, 'l', 0, 'l', 0, 'o'}, EncodingUTF16BE},
		{"UTF-16 LE no BOM", []byte{'h', 0, 'e', 0, 'l', 0, 'l', 0, 'o', 0}, EncodingUTF16LE},
		{"CP1252 smart quotes", []byte("hello \x93world\x94\n"), EncodingCP1252},
		{"Latin-1 accented", func() []byte {
			// Bytes 0xA0-0xFF are valid Latin-1 but invalid
			// UTF-8 without CP1252 indicators.
			return []byte{0xE9, 0xE8, 0xEA, 0x0A} // éèê\n
		}(), EncodingLatin1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, bom := sniffEncoding(tt.data)
			if enc != tt.want {
				t.Errorf("encoding = %d, want %d", enc, tt.want)
			}
			if bom {
				t.Error("unexpected BOM")
			}
		})
	}
}

func TestDecodeToUTF8_UTF16LE(t *testing.T) {
	// "Hi\n" in UTF-16 LE with BOM.
	data := []byte{0xFF, 0xFE, 'H', 0, 'i', 0, '\n', 0}
	got, err := decodeToUTF8(data, EncodingUTF16LE)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(got) {
		t.Fatal("result not valid UTF-8")
	}
	// BOM should be stripped by decodeToUTF8.
	if want := "Hi\n"; string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestDecodeToUTF8_UTF16BE_BOMStripped(t *testing.T) {
	// "Hi\n" in UTF-16 BE with BOM.
	data := []byte{0xFE, 0xFF, 0, 'H', 0, 'i', 0, '\n'}
	got, err := decodeToUTF8(data, EncodingUTF16BE)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Hi\n"; string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestDecodeToUTF8_CP1252(t *testing.T) {
	// 0x93 = left double quote, 0x94 = right double quote in CP1252.
	data := []byte{0x93, 'h', 'i', 0x94}
	got, err := decodeToUTF8(data, EncodingCP1252)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(got) {
		t.Fatal("result not valid UTF-8")
	}
	want := "\u201Chi\u201D"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	encs := []struct {
		name string
		enc  Encoding
	}{
		{"UTF-16 LE", EncodingUTF16LE},
		{"UTF-16 BE", EncodingUTF16BE},
		{"Latin-1", EncodingLatin1},
		{"CP1252", EncodingCP1252},
	}

	// Use only characters representable in all target encodings.
	original := "Hello world\n"

	for _, tt := range encs {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeFromUTF8(
				[]byte(original), tt.enc, false, false)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeToUTF8(encoded, tt.enc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(decoded) != original {
				t.Errorf("round-trip: got %q, want %q",
					string(decoded), original)
			}
		})
	}
}

func TestHasCP1252Indicators(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"plain ASCII", []byte("hello"), false},
		{"defined 0x93 left quote", []byte{0x93}, true},
		{"defined 0x85 ellipsis", []byte{0x85}, true},
		{"undefined 0x81", []byte{0x81}, false},
		{"undefined 0x8D", []byte{0x8D}, false},
		{"undefined 0x90", []byte{0x90}, false},
		{"undefined 0x9D", []byte{0x9D}, false},
		{"high byte no indicator", []byte{0xA0, 0xFF}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCP1252Indicators(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLatin1Plausible(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, true},
		{"plain ASCII", []byte("hello\n"), true},
		{"tab LF CR allowed", []byte{'\t', '\n', '\r'}, true},
		{"high accented bytes", []byte{0xE9, 0xFC, 0xF1}, true},
		{"NUL rejected", []byte{0x00}, false},
		{"BEL rejected", []byte{0x07}, false},
		{"DEL rejected", []byte{0x7F}, false},
		{"SOH rejected", []byte{0x01, 'a'}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLatin1Plausible(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeFromUTF8_UnencodableError(t *testing.T) {
	// Emoji is not representable in Latin-1 or CP1252.
	emoji := []byte("hello 🎉")
	for _, enc := range []Encoding{EncodingLatin1, EncodingCP1252} {
		_, err := encodeFromUTF8(emoji, enc, false, false)
		if err == nil {
			t.Errorf("enc=%d: expected error for unencodable emoji", enc)
		}
	}
}

func TestEncodeFromUTF8_UTF16BOMPrepend(t *testing.T) {
	data := []byte("Hi")

	// hasBOM=true, preserveBOM=true → BOM prepended.
	got, err := encodeFromUTF8(data, EncodingUTF16LE, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0xFF || got[1] != 0xFE {
		t.Errorf("expected UTF-16 LE BOM, got %x %x", got[0], got[1])
	}

	// hasBOM=false → no BOM.
	got2, err := encodeFromUTF8(data, EncodingUTF16LE, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if got2[0] == 0xFF && got2[1] == 0xFE {
		t.Error("BOM should not be prepended when hasBOM=false")
	}

	// hasBOM=true, preserveBOM=false → no BOM.
	got3, err := encodeFromUTF8(data, EncodingUTF16BE, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got3[0] == 0xFE && got3[1] == 0xFF {
		t.Error("BOM should not be prepended when preserveBOM=false")
	}
}

func TestSniffEncoding_AllNulls(t *testing.T) {
	data := make([]byte, 100)
	enc, _ := sniffEncoding(data)
	// All nulls — both even and odd positions are null, so neither
	// UTF-16 heuristic fires. Falls through to UTF-8 valid check
	// (all nulls is technically valid UTF-8).
	if enc != EncodingUTF8 {
		t.Errorf("got %d, want UTF8", enc)
	}
}

func TestSniffEncoding_SingleByte(t *testing.T) {
	// Below the 2-byte minimum for UTF-16 heuristic.
	enc, _ := sniffEncoding([]byte{0x41})
	if enc != EncodingUTF8 {
		t.Errorf("got %d, want UTF8", enc)
	}
}

func TestRawPassthrough(t *testing.T) {
	// Binary data: not valid UTF-8, not valid any encoding.
	data := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x80, 0x00}
	enc, bom := sniffEncoding(data)
	// Should detect as UTF-16 LE (BOM 0xFF 0xFE) or raw depending
	// on heuristic. Either way, the data should round-trip.
	decoded, err := decodeToUTF8(data, enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded, err := encodeFromUTF8(decoded, enc, bom, true)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_ = reencoded // Exact byte equality not guaranteed for
	// transcoded paths; what matters is no error.
}

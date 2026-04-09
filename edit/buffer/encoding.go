package buffer

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// sniffSize is the number of bytes inspected for encoding detection.
const sniffSize = 8192

// BOM signatures.
var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16BE = []byte{0xFE, 0xFF}
	bomUTF16LE = []byte{0xFF, 0xFE}
)

// sniffEncoding examines up to sniffSize bytes and returns the
// detected encoding and whether a BOM was found.
func sniffEncoding(data []byte) (Encoding, bool) {
	// BOM check — deterministic.
	if bytes.HasPrefix(data, bomUTF8) {
		return EncodingUTF8BOM, true
	}
	if bytes.HasPrefix(data, bomUTF16BE) {
		return EncodingUTF16BE, true
	}
	if bytes.HasPrefix(data, bomUTF16LE) {
		return EncodingUTF16LE, true
	}

	sample := data
	if len(sample) > sniffSize {
		sample = sample[:sniffSize]
	}

	// Null-byte heuristic for BOM-less UTF-16.
	if len(sample) >= 2 {
		oddNull, evenNull := 0, 0
		limit := len(sample) & ^1 // even length
		for i := 0; i < limit; i += 2 {
			if sample[i] == 0 {
				evenNull++
			}
			if sample[i+1] == 0 {
				oddNull++
			}
		}
		total := limit / 2
		// If >20% of even-position bytes are null → UTF-16BE;
		// if >20% of odd-position bytes are null → UTF-16LE.
		thresh := max(total/5, 1)
		if evenNull > thresh && oddNull == 0 {
			return EncodingUTF16BE, false
		}
		if oddNull > thresh && evenNull == 0 {
			return EncodingUTF16LE, false
		}
	}

	// Valid UTF-8 → UTF-8.
	if utf8.Valid(sample) {
		return EncodingUTF8, false
	}

	// Check for CP1252 indicators (bytes 0x80–0x9F that are defined
	// in CP1252 but undefined in Latin-1).
	if hasCP1252Indicators(sample) {
		return EncodingCP1252, false
	}

	// All single-byte values are valid Latin-1.
	if isLatin1Plausible(sample) {
		return EncodingLatin1, false
	}

	return EncodingRaw, false
}

// hasCP1252Indicators returns true if sample contains bytes in the
// 0x80–0x9F range that map to printable CP1252 characters (smart
// quotes, dashes, etc.) and not to Latin-1 C1 controls.
func hasCP1252Indicators(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 && b <= 0x9F {
			// 0x81, 0x8D, 0x8F, 0x90, 0x9D are undefined in CP1252.
			switch b {
			case 0x81, 0x8D, 0x8F, 0x90, 0x9D:
				return false // undefined → not CP1252
			default:
				return true // defined → CP1252 indicator
			}
		}
	}
	return false
}

// isLatin1Plausible returns true if every byte in data is a valid
// Latin-1 code point (no C0 controls except TAB, LF, CR).
func isLatin1Plausible(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
		// 0x7F (DEL) is suspicious in text files.
		if b == 0x7F {
			return false
		}
	}
	return true
}

// utf8BOM is the UTF-8 encoding of U+FEFF (BOM / zero-width
// no-break space). Used to strip decoded BOMs from UTF-16 output.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// decodeToUTF8 transcodes data from enc to UTF-8. Strips BOM if
// present (both raw BOM bytes and decoded U+FEFF). Returns data
// unchanged for UTF-8 and Raw.
func decodeToUTF8(data []byte, enc Encoding) ([]byte, error) {
	switch enc {
	case EncodingUTF8:
		return data, nil
	case EncodingUTF8BOM:
		if bytes.HasPrefix(data, bomUTF8) {
			return data[len(bomUTF8):], nil
		}
		return data, nil
	case EncodingUTF16BE:
		dec := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
		out, err := dec.NewDecoder().Bytes(data)
		if err != nil {
			return nil, err
		}
		// IgnoreBOM decodes the BOM as U+FEFF; strip it so the
		// buffer text doesn't contain a stale BOM character.
		return bytes.TrimPrefix(out, utf8BOM), nil
	case EncodingUTF16LE:
		dec := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
		out, err := dec.NewDecoder().Bytes(data)
		if err != nil {
			return nil, err
		}
		return bytes.TrimPrefix(out, utf8BOM), nil
	case EncodingLatin1:
		return charmap.ISO8859_1.NewDecoder().Bytes(data)
	case EncodingCP1252:
		return charmap.Windows1252.NewDecoder().Bytes(data)
	default: // Raw
		return data, nil
	}
}

// encodeFromUTF8 transcodes UTF-8 data back to enc. Prepends BOM
// when hasBOM and preserveBOM are both true. Returns data unchanged
// for UTF-8 and Raw.
func encodeFromUTF8(data []byte, enc Encoding, hasBOM, preserveBOM bool) ([]byte, error) {
	switch enc {
	case EncodingUTF8:
		return data, nil
	case EncodingUTF8BOM:
		if hasBOM && preserveBOM {
			out := make([]byte, len(bomUTF8)+len(data))
			copy(out, bomUTF8)
			copy(out[len(bomUTF8):], data)
			return out, nil
		}
		return data, nil
	case EncodingUTF16BE:
		enc16 := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
		encoded, err := enc16.NewEncoder().Bytes(data)
		if err != nil {
			return nil, err
		}
		if hasBOM && preserveBOM {
			out := make([]byte, len(bomUTF16BE)+len(encoded))
			copy(out, bomUTF16BE)
			copy(out[len(bomUTF16BE):], encoded)
			return out, nil
		}
		return encoded, nil
	case EncodingUTF16LE:
		enc16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
		encoded, err := enc16.NewEncoder().Bytes(data)
		if err != nil {
			return nil, err
		}
		if hasBOM && preserveBOM {
			out := make([]byte, len(bomUTF16LE)+len(encoded))
			copy(out, bomUTF16LE)
			copy(out[len(bomUTF16LE):], encoded)
			return out, nil
		}
		return encoded, nil
	case EncodingLatin1:
		return charmap.ISO8859_1.NewEncoder().Bytes(data)
	case EncodingCP1252:
		return charmap.Windows1252.NewEncoder().Bytes(data)
	default: // Raw
		return data, nil
	}
}

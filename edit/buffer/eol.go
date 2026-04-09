package buffer

import "bytes"

// detectEOL scans raw bytes and classifies the line-ending convention.
func detectEOL(data []byte) EOL {
	var hasLF, hasCRLF, hasCR bool
	i := 0
	for i < len(data) {
		switch data[i] {
		case '\r':
			if i+1 < len(data) && data[i+1] == '\n' {
				hasCRLF = true
				i += 2
			} else {
				hasCR = true
				i++
			}
		case '\n':
			hasLF = true
			i++
		default:
			i++
		}
	}

	count := 0
	var found EOL
	if hasCRLF {
		count++
		found = EOLCRLF
	}
	if hasLF {
		count++
		found = EOLLF
	}
	if hasCR {
		count++
		found = EOLCR
	}
	switch count {
	case 0:
		return EOLUnknown
	case 1:
		return found
	default:
		return EOLMixed
	}
}

// normalizeEOL converts CRLF and CR to LF in-place (result may be
// shorter than input). Returns the normalized slice.
func normalizeEOL(data []byte) []byte {
	// Fast path: no \r at all.
	if bytes.IndexByte(data, '\r') < 0 {
		return data
	}

	w := 0
	for r := 0; r < len(data); r++ {
		if data[r] == '\r' {
			data[w] = '\n'
			w++
			// Skip the \n in a \r\n pair.
			if r+1 < len(data) && data[r+1] == '\n' {
				r++
			}
		} else {
			data[w] = data[r]
			w++
		}
	}
	return data[:w]
}

// applyEOL converts LF in data to the specified line ending.
// Returns a new slice if the EOL differs from LF.
func applyEOL(data []byte, eol EOL) []byte {
	switch eol {
	case EOLCRLF:
		return replaceLF(data, []byte{'\r', '\n'})
	case EOLCR:
		return replaceLF(data, []byte{'\r'})
	default:
		return data
	}
}

// maxReplaceLFBytes caps the output of replaceLF to guard against
// pathological inputs (e.g. all-newline data with a long replacement).
const maxReplaceLFBytes = MaxLoadBytes * 2

// replaceLF replaces every \n in data with rep.
func replaceLF(data, rep []byte) []byte {
	// Count newlines to pre-allocate.
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	if n == 0 {
		return data
	}
	sz := len(data) + n*(len(rep)-1)
	if sz < 0 || sz > maxReplaceLFBytes {
		return data // refuse pathological expansion
	}
	out := make([]byte, 0, sz)
	for _, b := range data {
		if b == '\n' {
			out = append(out, rep...)
		} else {
			out = append(out, b)
		}
	}
	return out
}

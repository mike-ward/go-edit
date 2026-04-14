package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// SaveFile writes the buffer to path (or Props.FilePath if path is
// empty) using atomic write. Applies save policies (trim trailing
// whitespace, final newline), EOL conversion, and encoding. Updates
// Props.ModTime and clears the dirty flag on success.
func (b *Buffer) SaveFile(path string) error {
	if path == "" {
		path = b.Props.FilePath
	}
	if path == "" {
		return errors.New("buffer: no file path")
	}

	var buf bytes.Buffer
	buf.Grow(b.Len() + 64) // +64 for EOL expansion / BOM
	if _, err := b.WriteTo(&buf); err != nil {
		return err
	}

	mode := b.Props.FileMode
	if mode == 0 {
		mode = 0o644
	}

	if err := atomicWrite(path, buf.Bytes(), mode); err != nil {
		return err
	}

	// Update metadata.
	if info, err := os.Stat(path); err == nil {
		b.Props.ModTime = info.ModTime()
	}
	b.Props.FilePath = path
	b.MarkClean()
	return nil
}

// WriteTo serializes the buffer to w, applying save policies, EOL
// conversion, and encoding. Implements io.WriterTo.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	if w == nil {
		return 0, errors.New("buffer: nil writer")
	}
	data := b.Bytes()
	data = b.applySavePolicies(data)

	// EOL conversion.
	if b.Props.Encoding != EncodingRaw {
		data = applyEOL(data, b.Props.EOL)
	}

	// Encode from UTF-8 to target encoding.
	encoded, err := encodeFromUTF8(
		data, b.Props.Encoding,
		b.Props.HasBOM, b.Props.PreserveBOM)
	if err != nil {
		return 0, fmt.Errorf("buffer: encode: %w", err)
	}

	n, err := w.Write(encoded)
	return int64(n), err
}

// applySavePolicies applies trim-trailing-whitespace and
// final-newline policies.
func (b *Buffer) applySavePolicies(data []byte) []byte {
	if b.Props.TrimTrailingWS {
		data = trimTrailingWhitespace(data)
	}
	if b.Props.FinalNewline && (len(data) == 0 || data[len(data)-1] != '\n') {
		data = append(data, '\n')
	}
	return data
}

// trimTrailingWhitespace removes trailing spaces and tabs from
// each line (lines separated by \n).
func trimTrailingWhitespace(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))
	start := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == '\n' {
			line := data[start:i]
			line = bytes.TrimRight(line, " \t")
			out.Write(line)
			if i < len(data) {
				out.WriteByte('\n')
			}
			start = i + 1
		}
	}
	return out.Bytes()
}

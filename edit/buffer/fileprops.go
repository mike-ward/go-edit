package buffer

import (
	"os"
	"time"
)

// EOL identifies the line-ending convention detected in a file.
type EOL byte

// EOL line-ending conventions.
const (
	EOLUnknown EOL = iota // no line endings detected
	EOLLF                 // Unix LF
	EOLCRLF               // Windows CRLF
	EOLCR                 // Classic Mac CR
	EOLMixed              // more than one convention found
)

// Encoding identifies the character encoding detected in a file.
type Encoding byte

// Encoding character encodings.
const (
	EncodingUTF8    Encoding = iota // UTF-8 without BOM
	EncodingUTF8BOM                 // UTF-8 with BOM
	EncodingUTF16LE                 // UTF-16 Little Endian
	EncodingUTF16BE                 // UTF-16 Big Endian
	EncodingLatin1                  // ISO 8859-1
	EncodingCP1252                  // Windows-1252
	EncodingRaw                     // unrecognized; saved verbatim
)

// IndentStyle describes the indentation convention detected in a file.
type IndentStyle struct {
	UseTabs bool
	Width   int // number of spaces per indent level; 0 = not detected
}

// FileProps carries metadata detected at load time and policies
// applied at save time. Zero value is a sensible default for
// in-memory buffers (UTF-8, LF, final-newline on).
type FileProps struct {
	EOL         EOL
	Encoding    Encoding
	HasBOM      bool
	IndentStyle IndentStyle

	// Save policies.
	FinalNewline   bool // append trailing newline on save; default true
	TrimTrailingWS bool // strip trailing whitespace per line; default false
	PreserveBOM    bool // re-emit BOM on save if detected; default true

	// Filesystem metadata, populated by LoadFile.
	FilePath string
	FileMode os.FileMode
	ModTime  time.Time
}

// DefaultFileProps returns a FileProps with sensible defaults for
// a new in-memory buffer.
func DefaultFileProps() FileProps {
	return FileProps{
		EOL:          EOLLF,
		Encoding:     EncodingUTF8,
		FinalNewline: true,
		PreserveBOM:  true,
	}
}

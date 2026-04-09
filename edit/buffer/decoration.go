package buffer

// DecorationKind classifies a decoration for rendering.
type DecorationKind int

const (
	// DecoToken is inline text coloring (syntax tokens).
	DecoToken DecorationKind = iota
	// DecoBackground is a full-line background color.
	DecoBackground
	// DecoSquiggle is a wavy underline (diagnostics).
	DecoSquiggle
	// DecoGutter is a gutter icon/marker.
	DecoGutter
	// DecoInlineText is virtual inline text (ghost text, AI).
	DecoInlineText
	// DecoBlockText is virtual block text (above/below line).
	DecoBlockText
)

// Decoration is a single visual annotation on the document.
// Only DecoToken rendering is implemented in Phase 1.5; other
// kinds have types defined for interface validation.
type Decoration struct {
	Kind     DecorationKind
	Range    Range
	Priority int // higher wins conflicts

	// Token coloring (DecoToken).
	FgColor   uint32 // 0xRRGGBBAA; 0 = use default
	BgColor   uint32 // 0 = transparent
	Bold      bool
	Italic    bool
	Underline bool

	// Squiggle (DecoSquiggle).
	SquiggleColor uint32

	// Gutter (DecoGutter).
	GutterIcon  string
	GutterColor uint32

	// Virtual text (DecoInlineText, DecoBlockText).
	VirtualText string
	VirtualFg   uint32
}

// Viewport describes the visible line range for decoration
// queries.
type Viewport struct {
	FirstLine int
	LastLine  int
}

// DecorationProvider produces decorations for a viewport.
// Called once per frame for the visible range. Implementations
// must be fast; heavy work should be done asynchronously and
// cached.
type DecorationProvider interface {
	Decorate(vp Viewport) []Decoration
}

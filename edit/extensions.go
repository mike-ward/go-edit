package edit

import "github.com/mike-ward/go-edit/edit/buffer"

// Extension substrate types re-exported from edit/buffer.
// These are the stable public API; edit/buffer has no
// independent version contract.
type (
	// DecorationProvider produces decorations for a viewport.
	DecorationProvider = buffer.DecorationProvider
	// Decoration is a single visual annotation on the document.
	Decoration = buffer.Decoration
	// DecorationKind classifies a decoration for rendering.
	DecorationKind = buffer.DecorationKind
	// Viewport describes the visible line range for decoration queries.
	Viewport = buffer.Viewport
	// EditFilter observes, transforms, or vetoes an edit before Apply.
	EditFilter = buffer.EditFilter //nolint:revive // stutter is intentional; matches buffer.EditFilter
	// PostEditFunc is called after a successful Apply.
	PostEditFunc = buffer.PostEditFunc
	// FilterResult tells Apply what to do after a filter runs.
	FilterResult = buffer.FilterResult
	// Mark is a tracked position that auto-adjusts across edits.
	Mark = buffer.Mark
	// MarkSet holds all marks for a buffer.
	MarkSet = buffer.MarkSet
	// TrackedRange is a pair of marks bounding an auto-expanding range.
	TrackedRange = buffer.TrackedRange
	// Gravity determines mark behavior on insert at its position.
	Gravity = buffer.Gravity
)

// Filter result constants.
const (
	// FilterAccept proceeds with the (possibly modified) edit.
	FilterAccept = buffer.FilterAccept
	// FilterReject vetoes the edit.
	FilterReject = buffer.FilterReject
)

// Gravity constants.
const (
	// GravityLeft keeps the mark; insert goes to its right.
	GravityLeft = buffer.GravityLeft
	// GravityRight pushes the mark right on insert.
	GravityRight = buffer.GravityRight
)

// Decoration kind constants.
const (
	// DecoToken is inline text coloring (syntax tokens).
	DecoToken = buffer.DecoToken
	// DecoBackground is a full-line background color.
	DecoBackground = buffer.DecoBackground
	// DecoSquiggle is a wavy underline (diagnostics).
	DecoSquiggle = buffer.DecoSquiggle
	// DecoGutter is a gutter icon/marker.
	DecoGutter = buffer.DecoGutter
	// DecoInlineText is virtual inline text (ghost text, AI).
	DecoInlineText = buffer.DecoInlineText
	// DecoBlockText is virtual block text (above/below line).
	DecoBlockText = buffer.DecoBlockText
)

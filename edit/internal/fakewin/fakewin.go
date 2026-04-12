// Package fakewin provides a headless *gui.Window with a
// deterministic TextMeasurer, plus event builders for driving
// editor callbacks in unit tests.
//
// It lives under edit/internal so external projects cannot import
// it; it is a test fixture, not a public API.
package fakewin

import (
	"unicode/utf8"

	"github.com/mike-ward/go-glyph"
	"github.com/mike-ward/go-gui/gui"
)

// Fake measurer constants (pixels).
const (
	Advance    float32 = 8
	LineHeight float32 = 16
)

// New returns a headless window with the fake measurer and an
// in-memory clipboard installed.
func New() *gui.Window {
	w := &gui.Window{}
	w.SetTextMeasurer(&fakeMeasurer{})
	var clip string
	w.SetClipboardFn(func(s string) { clip = s })
	w.SetClipboardGetFn(func() string { return clip })
	return w
}

// NewKeyEvent builds a key-down event.
func NewKeyEvent(code gui.KeyCode, mods gui.Modifier) *gui.Event {
	return &gui.Event{
		Type:      gui.EventKeyDown,
		KeyCode:   code,
		Modifiers: mods,
	}
}

// NewCharEvent builds a character-input event.
func NewCharEvent(r rune) *gui.Event {
	return &gui.Event{
		Type:     gui.EventChar,
		CharCode: uint32(r),
	}
}

// NewScrollEvent builds a mouse-scroll event.
func NewScrollEvent(deltaY float32) *gui.Event {
	return &gui.Event{
		Type:    gui.EventMouseScroll,
		ScrollY: deltaY,
	}
}

// NewIMECharEvent builds an IME commit event.
func NewIMECharEvent(text string) *gui.Event {
	r, _ := utf8.DecodeRuneInString(text)
	return &gui.Event{
		Type:     gui.EventChar,
		CharCode: uint32(r),
		IMEText:  text,
	}
}

// NewClickEvent builds a mouse-down event.
func NewClickEvent(x, y float32, mods gui.Modifier) *gui.Event {
	return &gui.Event{
		Type:      gui.EventMouseDown,
		MouseX:    x,
		MouseY:    y,
		Modifiers: mods,
	}
}

// fakeMeasurer is a deterministic monospace measurer. The editor's
// ASCII fast path bypasses LayoutText, so most driver tests never
// exercise the returned Layout.
type fakeMeasurer struct{}

func (fakeMeasurer) TextWidth(text string, _ gui.TextStyle) float32 {
	return float32(len(text)) * Advance
}

func (fakeMeasurer) TextHeight(_ string, _ gui.TextStyle) float32 {
	return LineHeight
}

func (fakeMeasurer) FontHeight(_ gui.TextStyle) float32 { return LineHeight }

func (fakeMeasurer) FontAscent(_ gui.TextStyle) float32 { return LineHeight * 0.8 }

// LayoutText assumes ASCII input (one CharRect per byte).
func (fakeMeasurer) LayoutText(text string, _ gui.TextStyle, _ float32) (glyph.Layout, error) {
	rects := make([]glyph.CharRect, len(text))
	idx := make(map[int]int, len(text))
	for i := range text {
		rects[i] = glyph.CharRect{
			Rect: glyph.Rect{
				X:      float32(i) * Advance,
				Y:      0,
				Width:  Advance,
				Height: LineHeight,
			},
			Index: i,
		}
		idx[i] = i
	}
	return glyph.Layout{
		Text:            text,
		CharRects:       rects,
		CharRectByIndex: idx,
		Width:           float32(len(text)) * Advance,
		Height:          LineHeight,
	}, nil
}

package edit

import (
	"path/filepath"
	"strconv"
	"time"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// EditorCfg configures an Editor widget instance.
//
// IDFocus is the focus/state key. Width and Height define the fixed
// viewport size — the Editor manages scrolling inside this rectangle
// and never virtualizes through go-gui's Column-scroll mechanism
// (DrawCanvas caches the full draw output, which defeats line
// virtualization).
type EditorCfg struct {
	IDFocus          uint32
	Buffer           *buffer.Buffer
	Width            float32
	Height           float32
	Padding          gui.Opt[gui.Padding]
	SizeBorder       gui.Opt[float32]
	ShowLineNumbers  bool
	ShowBracketMatch bool
	ShowWhitespace   WhitespaceMode
	AutoClosePairs   []AutoClosePair // nil = use DefaultAutoClosePairs
	EnableFolding    bool
	LineWrap         bool
	StickyScroll     bool
	StickyScrollMax  int // 0 = use default (5)
	ReadOnly         bool
	Scrollbar        ScrollbarMode
	LangConfigs      map[string]LangConfig // keyed by ".ext" or filename
	Theme            EditorTheme
	Decorations      []DecorationProvider
	Keymaps          []*Keymap         // pushed on top of DefaultKeymap
	Actions          map[string]Action // additional/override actions
	// OnInvalidate is called once with a RequestRedraw thunk.
	// Decoration providers that do background work should store
	// the thunk and call it when new data is ready.
	OnInvalidate func(func())

	// CursorBlinkPeriod is the half-period of the cursor blink
	// cycle (visible for one period, hidden for one period).
	// Zero uses the default (500 ms). Negative disables blink.
	CursorBlinkPeriod time.Duration

	// Now is an injectable clock used by the blink cycle. Nil
	// defaults to time.Now. Tests pass a fake clock to make blink
	// state deterministic.
	Now func() time.Time
}

// defaultBlinkPeriod is the cursor blink half-period used when
// EditorCfg.CursorBlinkPeriod is zero.
const defaultBlinkPeriod = 500 * time.Millisecond

// minBlinkPeriod is the smallest blink half-period accepted.
// Anything smaller would fire the redraw timer in a tight loop.
const minBlinkPeriod = 50 * time.Millisecond

// blinkPeriod resolves the configured blink period. Zero →
// default; negative → 0 (disabled); too-small positive →
// clamped to minBlinkPeriod.
func blinkPeriod(cfg EditorCfg) time.Duration {
	if cfg.CursorBlinkPeriod == 0 {
		return defaultBlinkPeriod
	}
	if cfg.CursorBlinkPeriod < 0 {
		return 0
	}
	return max(cfg.CursorBlinkPeriod, minBlinkPeriod)
}

// nowOf returns the current time using cfg.Now if set, else
// time.Now. Returns time.Time directly to avoid a closure
// allocation on the hot path.
func nowOf(cfg EditorCfg) time.Time {
	if cfg.Now != nil {
		return cfg.Now()
	}
	return time.Now()
}

// editorMonoStyle returns the TextStyle used for all editor text
// rendering. Both the draw path and the Measurer must use this same
// style so the cached monospace advance matches rendered glyph width;
// drift between the two sites causes visible per-character gaps.
func editorMonoStyle(theme gui.Theme) gui.TextStyle {
	return theme.M5
}

// minDimension is the smallest viewport width/height the editor will
// accept. Smaller values (including NaN/negative) are clamped up.
const minDimension float32 = 1

// maxDimension is the largest viewport width/height the editor will
// accept. Larger values (including +Inf) are clamped down.
const maxDimension float32 = 1 << 20

// sanitizeDim clamps NaN / Inf / negative / over-sized dimensions
// to a safe range. NaN is detected via `x != x`.
func sanitizeDim(x float32) float32 {
	if x != x || x < minDimension {
		return minDimension
	}
	if x > maxDimension {
		return maxDimension
	}
	return x
}

// digitCount returns the number of decimal digits in n.
// Zero-alloc alternative to len(strconv.Itoa(n)).
func digitCount(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// Editor returns a go-gui View rendering a scrollable monospace
// code editor backed by cfg.Buffer. If cfg.Buffer is nil, an empty
// buffer is installed so the widget never nil-derefs. Width and
// Height are clamped to a safe range.
func Editor(cfg EditorCfg) gui.View {
	if cfg.Buffer == nil {
		cfg.Buffer = buffer.New()
	}
	cfg.Width = sanitizeDim(cfg.Width)
	cfg.Height = sanitizeDim(cfg.Height)
	frame := &editorFrameData{}

	a11yLabel := editorA11YLabel(cfg)
	a11yDesc := strconv.Itoa(cfg.Buffer.LineCount()) + " lines"
	var a11yState gui.AccessState
	if cfg.ReadOnly {
		a11yState = gui.AccessStateReadOnly
	}

	// Stable per-IDFocus canvas ID lets go-gui's DrawCanvas cache
	// reuse tessellated output across frames whose Version hasn't
	// changed. Distinct editors get distinct cache slots. The
	// Version is mutated in-place on the canvas's shape at the end
	// of editorAmendLayout so it reflects the current frame state.
	canvasID := "edit.canvas." + strconv.FormatUint(uint64(cfg.IDFocus), 10)
	canvas := gui.DrawCanvas(gui.DrawCanvasCfg{
		ID:              canvasID,
		Width:           cfg.Width,
		Height:          cfg.Height,
		Clip:            true,
		A11YLabel:       a11yLabel,
		A11YDescription: a11yDesc,
		OnDraw:          editorOnDraw(cfg, frame),
		OnClick:         editorOnClick(cfg, frame),
		OnMouseScroll:   editorOnMouseScroll(cfg, frame),
	})

	// Cursor overlay: a separate DrawCanvas with empty ID
	// (cache-bypass) so its contents re-tessellate every frame.
	// Wrapped in a floating Column anchored to the editor root's
	// top-left so it sits exactly over the main canvas. The main
	// canvas keeps its tessellation cache untouched across blink
	// transitions, while this tiny overlay (a few rects) repaints
	// freely. The wrapper has IDFocus=0 and no event handlers so
	// mouse clicks fall through to the main canvas underneath.
	cursorCanvas := gui.DrawCanvas(gui.DrawCanvasCfg{
		ID:     "",
		Width:  cfg.Width,
		Height: cfg.Height,
		Clip:   true,
		OnDraw: editorOnDrawCursor(cfg, frame),
	})
	cursorOverlay := gui.Column(gui.ContainerCfg{
		Width:       cfg.Width,
		Height:      cfg.Height,
		Sizing:      gui.FixedFixed,
		Padding:     cfg.Padding,
		SizeBorder:  cfg.SizeBorder,
		Float:       true,
		FloatAnchor: gui.FloatTopLeft,
		FloatTieOff: gui.FloatTopLeft,
		Content:     []gui.View{cursorCanvas},
	})

	return gui.Column(gui.ContainerCfg{
		IDFocus:     cfg.IDFocus,
		Width:       cfg.Width,
		Height:      cfg.Height,
		Sizing:      gui.FixedFixed,
		Padding:     cfg.Padding,
		SizeBorder:  cfg.SizeBorder,
		Clip:        true,
		A11YRole:    gui.AccessRoleTextArea,
		A11YLabel:   a11yLabel,
		A11YState:   a11yState,
		OnKeyDown:   editorOnKeyDown(cfg, frame),
		OnChar:      editorOnChar(cfg, frame),
		AmendLayout: editorAmendLayout(cfg, frame),
		Content:     []gui.View{canvas, cursorOverlay},
	})
}

// editorA11YLabel returns an accessibility label from the
// buffer's file path, or "Untitled" if empty.
func editorA11YLabel(cfg EditorCfg) string {
	if fp := cfg.Buffer.Props.FilePath; fp != "" {
		return filepath.Base(fp)
	}
	return "Untitled"
}

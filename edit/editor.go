package edit

import (
	"path/filepath"
	"strconv"

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
	ShowLineNumbers  bool
	ShowBracketMatch bool
	ShowWhitespace   WhitespaceMode
	AutoClosePairs   []AutoClosePair // nil = use DefaultAutoClosePairs
	EnableFolding    bool
	LineWrap         bool
	StickyScroll     bool
	StickyScrollMax  int // 0 = use default (5)
	ReadOnly         bool
	LangConfigs      map[string]LangConfig // keyed by ".ext" or filename
	Theme            EditorTheme
	Decorations      []buffer.DecorationProvider
	Keymaps          []*Keymap         // pushed on top of DefaultKeymap
	Actions          map[string]Action // additional/override actions
	// OnInvalidate is called once with a RequestRedraw thunk.
	// Decoration providers that do background work should store
	// the thunk and call it when new data is ready.
	OnInvalidate func(func())
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

	canvas := gui.DrawCanvas(gui.DrawCanvasCfg{
		// ID empty → skip draw cache; OnDraw runs every frame.
		Width:           cfg.Width,
		Height:          cfg.Height,
		Clip:            true,
		A11YLabel:       a11yLabel,
		A11YDescription: a11yDesc,
		OnDraw:          editorOnDraw(cfg, frame),
		OnClick:         editorOnClick(cfg, frame),
		OnMouseScroll:   editorOnMouseScroll(cfg, frame),
	})

	return gui.Column(gui.ContainerCfg{
		IDFocus:     cfg.IDFocus,
		Width:       cfg.Width,
		Height:      cfg.Height,
		Clip:        true,
		A11YRole:    gui.AccessRoleTextArea,
		A11YLabel:   a11yLabel,
		A11YState:   a11yState,
		OnKeyDown:   editorOnKeyDown(cfg, frame),
		OnChar:      editorOnChar(cfg, frame),
		AmendLayout: editorAmendLayout(cfg, frame),
		Content:     []gui.View{canvas},
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

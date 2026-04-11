package edit

import "github.com/mike-ward/go-gui/gui"

// EditorTheme maps editor visual elements to colors. Zero-value
// fields mean "use hardcoded default" (backward compat).
type EditorTheme struct {
	// Background is the editor canvas fill color. Zero = transparent
	// (inherits from the surrounding layout).
	Background gui.Color

	// UI element colors (gui.Color; zero/unset = use default).
	SelectionBg    gui.Color
	BracketMatchBg gui.Color
	SearchMatchBg  gui.Color
	CurrentMatchBg gui.Color
	FindBarBg      gui.Color
	FindBarBorder  gui.Color
	StickyBg       gui.Color
	StickyBorder   gui.Color
	CursorColor    gui.Color
	GutterFg       gui.Color
	ScrollbarThumb gui.Color
	ScrollbarTrack gui.Color
}

// resolvedTheme holds per-frame resolved colors with fallbacks
// applied. Built once in editorOnDraw, passed to draw helpers.
type resolvedTheme struct {
	background     gui.Color
	selectionBg    gui.Color
	bracketMatchBg gui.Color
	searchMatchBg  gui.Color
	currentMatchBg gui.Color
	findBarBg      gui.Color
	findBarBorder  gui.Color
	stickyBg       gui.Color
	stickyBorder   gui.Color
	cursorColor    gui.Color
	gutterFg       gui.Color
	scrollbarThumb gui.Color
	scrollbarTrack gui.Color
}

// resolveEditorTheme applies fallback defaults for any unset
// colors in the EditorTheme.
func resolveEditorTheme(et EditorTheme) resolvedTheme {
	return resolvedTheme{
		background:     et.Background,
		selectionBg:    resolveColor(et.SelectionBg, defaultSelectionBg),
		bracketMatchBg: resolveColor(et.BracketMatchBg, defaultBracketMatchBg),
		searchMatchBg:  resolveColor(et.SearchMatchBg, defaultSearchMatchBg),
		currentMatchBg: resolveColor(et.CurrentMatchBg, defaultCurrentMatchBg),
		findBarBg:      resolveColor(et.FindBarBg, defaultFindBarBg),
		findBarBorder:  resolveColor(et.FindBarBorder, defaultFindBarBorder),
		stickyBg:       resolveColor(et.StickyBg, defaultStickyBg),
		stickyBorder:   resolveColor(et.StickyBorder, defaultStickyBorder),
		cursorColor:    resolveColor(et.CursorColor, gui.Color{}),
		gutterFg:       resolveColor(et.GutterFg, gui.Color{}),
		scrollbarThumb: resolveColor(et.ScrollbarThumb, defaultScrollbarThumb),
		scrollbarTrack: resolveColor(et.ScrollbarTrack, defaultScrollbarTrack),
	}
}

func resolveColor(configured, fallback gui.Color) gui.Color {
	if configured.IsSet() {
		return configured
	}
	return fallback
}

// Default UI colors.
var (
	defaultSelectionBg    = gui.RGBA(51, 144, 255, 96)
	defaultBracketMatchBg = gui.RGBA(255, 255, 0, 40)
	defaultSearchMatchBg  = gui.RGBA(255, 200, 0, 60)
	defaultCurrentMatchBg = gui.RGBA(255, 150, 0, 120)
	defaultFindBarBg      = gui.RGBA(40, 40, 40, 230)
	defaultFindBarBorder  = gui.RGBA(80, 80, 80, 255)
	defaultStickyBg       = gui.RGBA(30, 30, 30, 240)
	defaultStickyBorder   = gui.RGBA(60, 60, 60, 255)
	defaultScrollbarThumb = gui.RGBA(150, 150, 150, 120)
	defaultScrollbarTrack = gui.RGBA(30, 30, 30, 80)
)

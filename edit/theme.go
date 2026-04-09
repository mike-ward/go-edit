package edit

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/mike-ward/go-gui/gui"
)

// EditorTheme maps editor visual elements to colors. Zero-value
// fields mean "use hardcoded default" (backward compat).
type EditorTheme struct {
	// Syntax token colors (0xRRGGBBAA; 0 = use chroma default).
	Keyword  uint32
	String   uint32
	Number   uint32
	Comment  uint32
	Operator uint32
	Type     uint32
	Function uint32
	Builtin  uint32

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
}

// resolvedTheme holds per-frame resolved colors with fallbacks
// applied. Built once in editorOnDraw, passed to draw helpers.
type resolvedTheme struct {
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
}

// resolveEditorTheme applies fallback defaults for any unset
// colors in the EditorTheme.
func resolveEditorTheme(et EditorTheme) resolvedTheme {
	return resolvedTheme{
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
	}
}

func resolveColor(configured, fallback gui.Color) gui.Color {
	if configured.IsSet() {
		return configured
	}
	return fallback
}

// guiColorToUint32 converts gui.Color to 0xRRGGBBAA. Returns 0
// if unset.
func guiColorToUint32(c gui.Color) uint32 {
	if !c.IsSet() {
		return 0
	}
	return uint32(c.R)<<24 | uint32(c.G)<<16 |
		uint32(c.B)<<8 | uint32(c.A)
}

// ThemeFromGUI derives an EditorTheme from the current go-gui
// theme, using MarkdownStyle code colors.
func ThemeFromGUI() EditorTheme {
	ms := gui.DefaultMarkdownStyle()
	theme := gui.CurrentTheme()
	return EditorTheme{
		Keyword:  guiColorToUint32(ms.CodeKeywordColor),
		String:   guiColorToUint32(ms.CodeStringColor),
		Number:   guiColorToUint32(ms.CodeNumberColor),
		Comment:  guiColorToUint32(ms.CodeCommentColor),
		Operator: guiColorToUint32(ms.CodeOperatorColor),
		Type:     guiColorToUint32(ms.CodeTypeColor),
		Function: guiColorToUint32(ms.CodeFunctionColor),
		Builtin:  guiColorToUint32(ms.CodeBuiltinColor),

		SelectionBg: theme.ColorSelect,
	}
}

// TokenOverridesFromTheme builds a chroma token-type override
// map from an EditorTheme. Only non-zero colors are included.
func TokenOverridesFromTheme(et EditorTheme) map[chroma.TokenType]uint32 {
	pairs := []struct {
		tt chroma.TokenType
		c  uint32
	}{
		{chroma.Keyword, et.Keyword},
		{chroma.KeywordConstant, et.Keyword},
		{chroma.KeywordDeclaration, et.Keyword},
		{chroma.KeywordNamespace, et.Keyword},
		{chroma.KeywordReserved, et.Keyword},
		{chroma.KeywordType, et.Type},
		{chroma.LiteralString, et.String},
		{chroma.LiteralStringAffix, et.String},
		{chroma.LiteralStringBacktick, et.String},
		{chroma.LiteralStringChar, et.String},
		{chroma.LiteralStringDouble, et.String},
		{chroma.LiteralStringSingle, et.String},
		{chroma.LiteralNumber, et.Number},
		{chroma.LiteralNumberFloat, et.Number},
		{chroma.LiteralNumberHex, et.Number},
		{chroma.LiteralNumberInteger, et.Number},
		{chroma.LiteralNumberOct, et.Number},
		{chroma.Comment, et.Comment},
		{chroma.CommentSingle, et.Comment},
		{chroma.CommentMultiline, et.Comment},
		{chroma.Operator, et.Operator},
		{chroma.OperatorWord, et.Operator},
		{chroma.NameFunction, et.Function},
		{chroma.NameBuiltin, et.Builtin},
		{chroma.NameClass, et.Type},
		{chroma.NameOther, et.Type},
	}
	m := make(map[chroma.TokenType]uint32, len(pairs))
	for _, p := range pairs {
		if p.c != 0 {
			m[p.tt] = p.c
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// Default UI colors (backward compat with hardcoded values).
var (
	defaultSelectionBg    = gui.RGBA(51, 144, 255, 96)
	defaultBracketMatchBg = gui.RGBA(255, 255, 0, 40)
	defaultSearchMatchBg  = gui.RGBA(255, 200, 0, 60)
	defaultCurrentMatchBg = gui.RGBA(255, 150, 0, 120)
	defaultFindBarBg      = gui.RGBA(40, 40, 40, 230)
	defaultFindBarBorder  = gui.RGBA(80, 80, 80, 255)
	defaultStickyBg       = gui.RGBA(30, 30, 30, 240)
	defaultStickyBorder   = gui.RGBA(60, 60, 60, 255)
)

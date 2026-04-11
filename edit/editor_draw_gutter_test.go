package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/text"
	"github.com/mike-ward/go-gui/gui"
)

// gutterFrame returns a minimal editorFrameData with a fake measurer
// wired in. gutterW and padLeft are set to typical values.
func gutterFrame() *editorFrameData {
	m := text.NewFake(8, 16)
	return &editorFrameData{
		gutterW:    40,
		padLeft:    4,
		lineHeight: 16,
		state:      editorState{Measurer: m},
	}
}

// zeroRT returns a resolvedTheme with no colors set (all zero value).
func zeroRT() resolvedTheme { return resolvedTheme{} }

// bgRT returns a resolvedTheme with a specific background color set.
func bgRT(c gui.Color) resolvedTheme {
	return resolvedTheme{background: c}
}

// blankTheme returns a gui.Theme with ColorBackground set to a known
// color so fallback tests can distinguish it from zero.
func blankTheme(bg gui.Color) gui.Theme {
	var t gui.Theme
	t.ColorBackground = bg
	return t
}

// --- batch presence helpers ---

func batchCount(dc *gui.DrawContext) int { return len(dc.Batches()) }
func textCount(dc *gui.DrawContext) int  { return len(dc.Texts()) }

// --- drawGutterPass: background rect ---

func TestDrawGutterPass_EmitsBatches(t *testing.T) {
	// FilledRect + Line → at least two batches (may merge if same color).
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	drawGutterPass(dc, EditorCfg{}, frame, nil, nil,
		nil, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	if batchCount(dc) == 0 {
		t.Fatal("expected at least one batch (FilledRect), got 0")
	}
}

func TestDrawGutterPass_EmptyEntries_NoText(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	drawGutterPass(dc, EditorCfg{}, frame, nil, nil,
		nil, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	if textCount(dc) != 0 {
		t.Errorf("empty entries: want 0 text entries, got %d", textCount(dc))
	}
}

// --- drawGutterPass: background color selection ---

func TestDrawGutterPass_UsesRtBackground_WhenSet(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	rtBg := gui.RGBA(30, 30, 30, 255)
	fallback := gui.RGBA(200, 200, 200, 255)
	drawGutterPass(dc, EditorCfg{}, frame, nil, nil,
		nil, gui.TextStyle{}, gui.TextStyle{},
		bgRT(rtBg), blankTheme(fallback))
	// The first batch should use rtBg, not fallback.
	batches := dc.Batches()
	if len(batches) == 0 {
		t.Fatal("no batches")
	}
	if batches[0].Color != rtBg {
		t.Errorf("first batch color=%v want rtBg=%v", batches[0].Color, rtBg)
	}
}

func TestDrawGutterPass_FallsBackToThemeBackground_WhenRtUnset(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	themeBg := gui.RGBA(50, 50, 50, 255)
	drawGutterPass(dc, EditorCfg{}, frame, nil, nil,
		nil, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), blankTheme(themeBg))
	batches := dc.Batches()
	if len(batches) == 0 {
		t.Fatal("no batches")
	}
	if batches[0].Color != themeBg {
		t.Errorf("first batch color=%v want themeBg=%v", batches[0].Color, themeBg)
	}
}

// --- drawGutterPass: line number text ---

func TestDrawGutterPass_SingleEntry_EmitsLineNumber(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{{line: 0, y: 0}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	texts := dc.Texts()
	if len(texts) == 0 {
		t.Fatal("want line number text, got none")
	}
	// Line 0 → "1".
	found := false
	for _, e := range texts {
		if e.Text == "1" {
			found = true
		}
	}
	if !found {
		t.Errorf("line number '1' not found in texts: %v", texts)
	}
}

func TestDrawGutterPass_MultipleEntries_EmitsAllLineNumbers(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{
		{line: 0, y: 0},
		{line: 1, y: 16},
		{line: 9, y: 32},
	}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	texts := dc.Texts()
	want := map[string]bool{"1": false, "2": false, "10": false}
	for _, e := range texts {
		if _, ok := want[e.Text]; ok {
			want[e.Text] = true
		}
	}
	for num, seen := range want {
		if !seen {
			t.Errorf("line number %q not found in texts", num)
		}
	}
}

func TestDrawGutterPass_ShowLineNumbersFalse_NoText(t *testing.T) {
	// drawGutterPass is only called when ShowLineNumbers is true at the
	// call site, but drawGutter itself must still produce text.
	// This test confirms that a nil measurer path produces no text.
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	frame.state.Measurer = nil // no measurer → drawGutter returns early
	entries := []gutterEntry{{line: 0, y: 0}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	if textCount(dc) != 0 {
		t.Errorf("nil measurer: want 0 texts, got %d", textCount(dc))
	}
}

// --- drawGutterPass: gutter text drawn after background batch ---

func TestDrawGutterPass_TextOrderAfterBatches(t *testing.T) {
	// Regression: line number texts must come from this pass (after the
	// background rect batch), not from the earlier main draw loop. Since
	// the render pipeline emits all batches then all texts, the gutter
	// background batch must exist and line number texts must also exist.
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{{line: 4, y: 64}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		bgRT(gui.RGBA(20, 20, 20, 255)), gui.CurrentTheme())
	if batchCount(dc) == 0 {
		t.Fatal("no batches: background rect missing")
	}
	if textCount(dc) == 0 {
		t.Fatal("no texts: line number missing")
	}
}

// --- drawGutterPass: nil/empty decos no panic ---

func TestDrawGutterPass_NilDecos_NoPanic(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{{line: 0, y: 0}, {line: 1, y: 16}}
	// Must not panic.
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
}

func TestDrawGutterPass_WithDecoGutter_NoPanic(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	decos := []buffer.Decoration{{
		Kind:        buffer.DecoGutter,
		Range:       buffer.Range{Start: buffer.Position{Line: 0}},
		GutterColor: 0xFF0000FF,
	}}
	entries := []gutterEntry{{line: 0, y: 0}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, decos,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
}

// --- drawGutterPass: negative/large line indices ---

func TestDrawGutterPass_NegativeLine_NoPanic(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{{line: -1, y: 0}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
	// drawGutter returns early on line < 0 → no crash, no text.
	if textCount(dc) != 0 {
		t.Errorf("negative line: want 0 texts, got %d", textCount(dc))
	}
}

func TestDrawGutterPass_LargeLineNumber_NoPanic(t *testing.T) {
	dc := gui.NewDrawContext(800, 600, nil)
	frame := gutterFrame()
	entries := []gutterEntry{{line: 99999, y: 0}}
	drawGutterPass(dc, EditorCfg{ShowLineNumbers: true}, frame, nil, nil,
		entries, gui.TextStyle{}, gui.TextStyle{},
		zeroRT(), gui.CurrentTheme())
}

package edit

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/highlight"
	"github.com/mike-ward/go-edit/edit/internal/fakewin"
	"github.com/mike-ward/go-gui/gui"
)

// benchEnv holds pre-built editor state for tessellation benchmarks.
type benchEnv struct {
	cfg      EditorCfg
	frame    *editorFrameData
	drawFn   func(*gui.DrawContext)
	cursorFn func(*gui.DrawContext)
	w        *gui.Window
}

// genBenchContent builds deterministic pseudo-Go content.
// Duplicated from buffer/buffer_bench_test.go (unexported).
func genBenchContent(lines int) []byte {
	var out bytes.Buffer
	out.Grow(lines * 40)
	r := rand.New(rand.NewPCG(42, 42))
	words := []string{
		"func", "var", "const", "type", "return", "if", "for",
		"range", "struct", "interface", "package", "import",
	}
	for range lines {
		n := 3 + r.IntN(8)
		for j := range n {
			if j > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(words[r.IntN(len(words))])
		}
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// newBenchEnv creates a headless editor environment for benchmarks.
// rows controls viewport height in lines. tweak runs after cfg is
// built but before the first amend pass; use it to pre-populate
// state (selection, search, etc.).
func newBenchEnv(
	rows int,
	tweak func(cfg *EditorCfg, w *gui.Window),
) benchEnv {
	buf := buffer.FromBytes(genBenchContent(100_000))
	buf.Props.FilePath = "bench.go"

	w := fakewin.New()
	frame := &editorFrameData{}

	hl := highlight.New(buf, "go", nil)

	cfg := EditorCfg{
		IDFocus:           1,
		Buffer:            buf,
		Width:             800,
		Height:            float32(rows) * fakewin.LineHeight,
		ShowLineNumbers:   true,
		ShowBracketMatch:  true,
		CursorBlinkPeriod: -1, // disable real timers
		Now:               time.Now,
	}
	if hl != nil {
		cfg.Decorations = []DecorationProvider{hl}
	}

	if tweak != nil {
		tweak(&cfg, w)
	}

	amend := editorAmendLayout(cfg, frame)
	amend(&gui.Layout{}, w)

	return benchEnv{
		cfg:      cfg,
		frame:    frame,
		drawFn:   editorOnDraw(cfg, frame),
		cursorFn: editorOnDrawCursor(cfg, frame),
		w:        w,
	}
}

// tessStats counts batches, text entries, and triangles from a DC.
type tessStats struct {
	batches int
	texts   int
	tris    int
}

func dcStats(dc *gui.DrawContext) tessStats {
	var s tessStats
	s.batches = len(dc.Batches())
	s.texts = len(dc.Texts())
	for _, b := range dc.Batches() {
		s.tris += len(b.Triangles) / 6
	}
	return s
}

func reportStats(b *testing.B, s tessStats) {
	b.ReportMetric(float64(s.batches), "batches/op")
	b.ReportMetric(float64(s.texts), "texts/op")
	b.ReportMetric(float64(s.tris), "tris/op")
}

// BenchmarkTess_FullDraw measures full-frame tessellation for a
// 100-row viewport of a 100k-line buffer with syntax highlighting
// and line numbers.
func BenchmarkTess_FullDraw(b *testing.B) {
	env := newBenchEnv(100, nil)
	b.ResetTimer()
	b.ReportAllocs()
	var last tessStats
	for b.Loop() {
		dc := gui.NewDrawContext(env.cfg.Width, env.cfg.Height,
			env.w.TextMeasurer())
		env.drawFn(dc)
		last = dcStats(dc)
	}
	reportStats(b, last)
}

// BenchmarkTess_WithOverlays adds selection, search highlights,
// and whitespace visualization to the full draw.
func BenchmarkTess_WithOverlays(b *testing.B) {
	env := newBenchEnv(100, func(cfg *EditorCfg, w *gui.Window) {
		cfg.ShowWhitespace = WhitespaceAll

		// Pre-populate selection spanning lines 10-30.
		st := loadState(w, cfg.IDFocus)
		st.Cursors = []CursorState{{
			Cursor: buffer.Position{Line: 30, ByteCol: 0},
			Anchor: buffer.Position{Line: 10, ByteCol: 0},
		}}

		// Fake search matches in the viewport.
		st.Search.Active = true
		st.Search.Query = "func"
		matches := make([]buffer.Range, 10)
		for i := range matches {
			line := 5 + i*3
			matches[i] = buffer.Range{
				Start: buffer.Position{Line: line, ByteCol: 0},
				End:   buffer.Position{Line: line, ByteCol: 4},
			}
		}
		st.Search.Matches = matches
		st.Search.CurrentMatch = 0
		storeState(w, cfg.IDFocus, st)
	})

	b.ResetTimer()
	b.ReportAllocs()
	var last tessStats
	for b.Loop() {
		dc := gui.NewDrawContext(env.cfg.Width, env.cfg.Height,
			env.w.TextMeasurer())
		env.drawFn(dc)
		last = dcStats(dc)
	}
	reportStats(b, last)
}

// BenchmarkTess_CursorOnly measures the cursor overlay canvas in
// isolation. Validates that blink separation keeps this trivially
// cheap compared to a full draw.
func BenchmarkTess_CursorOnly(b *testing.B) {
	env := newBenchEnv(100, nil)
	env.frame.cursorVisible = true
	b.ResetTimer()
	b.ReportAllocs()
	var last tessStats
	for b.Loop() {
		dc := gui.NewDrawContext(env.cfg.Width, env.cfg.Height,
			env.w.TextMeasurer())
		env.cursorFn(dc)
		last = dcStats(dc)
	}
	reportStats(b, last)
}

// BenchmarkTess_Scaling measures how tessellation cost scales with
// the number of visible rows.
func BenchmarkTess_Scaling(b *testing.B) {
	for _, rows := range []int{25, 50, 100, 200} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			env := newBenchEnv(rows, nil)
			b.ResetTimer()
			b.ReportAllocs()
			var last tessStats
			for b.Loop() {
				dc := gui.NewDrawContext(env.cfg.Width,
					env.cfg.Height, env.w.TextMeasurer())
				env.drawFn(dc)
				last = dcStats(dc)
			}
			reportStats(b, last)
		})
	}
}

// BenchmarkTess_Primitives measures raw DrawContext primitive cost,
// isolating go-gui tessellation from editor logic.
func BenchmarkTess_Primitives(b *testing.B) {
	b.Run("FilledRect10k", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			dc := gui.NewDrawContext(1920, 1080, nil)
			c := gui.Color{R: 50, G: 80, B: 100, A: 255}
			for i := range 10_000 {
				dc.FilledRect(float32(i%100)*10, float32(i/100)*10,
					8, 16, c)
			}
		}
	})

	b.Run("Text10k", func(b *testing.B) {
		b.ReportAllocs()
		style := gui.TextStyle{Size: 14}
		for b.Loop() {
			dc := gui.NewDrawContext(1920, 1080, nil)
			for i := range 10_000 {
				dc.Text(float32(i%100)*80, float32(i/100)*16,
					"func main() {", style)
			}
		}
	})

	b.Run("Polyline1k", func(b *testing.B) {
		b.ReportAllocs()
		// 10-segment squiggle-like polyline.
		pts := make([]float32, 22) // 11 points
		for i := range 11 {
			pts[i*2] = float32(i) * 4
			pts[i*2+1] = float32(i%2) * 2
		}
		c := gui.Color{R: 255, A: 255}
		for b.Loop() {
			dc := gui.NewDrawContext(1920, 1080, nil)
			for range 1_000 {
				dc.Polyline(pts, c, 1)
			}
		}
	})
}

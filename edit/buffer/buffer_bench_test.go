package buffer

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

// genBench builds deterministic pseudo-Go content of the requested
// line count. Lines vary in length so edit benches don't hit a
// trivial uniform path. No file is committed; regenerated per run.
func genBench(lines int) []byte {
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

func BenchmarkFromBytes100k(b *testing.B) {
	raw := genBench(100_000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = FromBytes(raw)
	}
}

func BenchmarkLoad100k(b *testing.B) {
	raw := genBench(100_000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Load(bytes.NewReader(raw))
	}
}

func BenchmarkLineIter100k(b *testing.B) {
	buf := FromBytes(genBench(100_000))
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		total := 0
		for i := 0; i < buf.LineCount(); i++ {
			total += len(buf.Line(i))
		}
		_ = total
	}
}

func BenchmarkRandomEdits10k(b *testing.B) {
	raw := genBench(100_000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		buf := FromBytes(raw)
		r := rand.New(rand.NewPCG(1, 1))
		b.StartTimer()
		for range 10_000 {
			line := r.IntN(buf.LineCount())
			col := 0
			if ll := len(buf.Line(line)); ll > 0 {
				col = r.IntN(ll + 1)
			}
			p := Position{Line: line, ByteCol: col}
			switch r.IntN(3) {
			case 0:
				buf.Apply(Edit{
					Range:    Range{Start: p, End: p},
					NewBytes: []byte{'x'},
				})
			case 1:
				buf.Apply(Edit{
					Range:    Range{Start: p, End: p},
					NewBytes: []byte("\n"),
				})
			case 2:
				end := p
				if end.ByteCol < len(buf.Line(end.Line)) {
					end.ByteCol++
					buf.Apply(Edit{Range: Range{Start: p, End: end}})
				}
			}
		}
	}
}

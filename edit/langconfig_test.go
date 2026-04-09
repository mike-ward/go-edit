package edit

import (
	"strings"
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestResolveLangConfigByExtension(t *testing.T) {
	cfg := EditorCfg{
		Buffer: buffer.New(),
		LangConfigs: map[string]LangConfig{
			".go": {TabWidth: 4, UseTabs: true, CommentLine: "//"},
			".py": {TabWidth: 4, CommentLine: "#"},
		},
	}
	cfg.Buffer.Props.FilePath = "main.go"
	lc := resolveLangConfig(cfg)
	if lc.TabWidth != 4 || !lc.UseTabs || lc.CommentLine != "//" {
		t.Fatalf("unexpected config for .go: %+v", lc)
	}

	cfg.Buffer.Props.FilePath = "script.py"
	lc = resolveLangConfig(cfg)
	if lc.CommentLine != "#" {
		t.Fatalf("expected # comment for .py, got %q", lc.CommentLine)
	}
}

func TestResolveLangConfigByFilename(t *testing.T) {
	cfg := EditorCfg{
		Buffer: buffer.New(),
		LangConfigs: map[string]LangConfig{
			"Makefile": {TabWidth: 8, UseTabs: true, CommentLine: "#"},
		},
	}
	cfg.Buffer.Props.FilePath = "/path/to/Makefile"
	lc := resolveLangConfig(cfg)
	if lc.TabWidth != 8 {
		t.Fatalf("expected TabWidth 8, got %d", lc.TabWidth)
	}
}

func TestResolveLangConfigNoMatch(t *testing.T) {
	cfg := EditorCfg{
		Buffer: buffer.New(),
		LangConfigs: map[string]LangConfig{
			".go": {TabWidth: 4},
		},
	}
	cfg.Buffer.Props.FilePath = "file.rs"
	lc := resolveLangConfig(cfg)
	if lc.TabWidth != 0 {
		t.Fatalf("expected zero LangConfig, got %+v", lc)
	}
}

func TestResolveLangConfigNilMap(t *testing.T) {
	cfg := EditorCfg{Buffer: buffer.New()}
	lc := resolveLangConfig(cfg)
	if lc.TabWidth != 0 {
		t.Fatalf("expected zero LangConfig, got %+v", lc)
	}
}

func TestResolveLangConfigEmptyFilePath(t *testing.T) {
	cfg := EditorCfg{
		Buffer: buffer.New(),
		LangConfigs: map[string]LangConfig{
			".go": {TabWidth: 4},
		},
	}
	// Empty FilePath with non-nil map → zero.
	lc := resolveLangConfig(cfg)
	if lc.TabWidth != 0 {
		t.Fatalf("expected zero LangConfig, got %+v", lc)
	}
}

func TestToggleCommentOutOfBoundsCursor(t *testing.T) {
	buf := loadBuf(t, "line1\nline2")
	cfg := EditorCfg{
		Buffer: buf,
		LangConfigs: map[string]LangConfig{
			".go": {CommentLine: "//"},
		},
	}
	cfg.Buffer.Props.FilePath = "test.go"

	// Cursor beyond buffer bounds.
	cs := &CursorState{
		Anchor: buffer.Position{Line: -1, ByteCol: 0},
		Cursor: buffer.Position{Line: 99, ByteCol: 0},
	}
	// Should not panic.
	toggleComment(cfg, cs, buf)

	// startLine > endLine after clamp.
	cs2 := &CursorState{
		Anchor: buffer.Position{Line: 5, ByteCol: 0},
		Cursor: buffer.Position{Line: 3, ByteCol: 0},
	}
	toggleComment(cfg, cs2, buf)
}

func TestToggleCommentSingleLine(t *testing.T) {
	buf := loadBuf(t, "hello")
	cfg := EditorCfg{
		Buffer: buf,
		LangConfigs: map[string]LangConfig{
			".go": {CommentLine: "//"},
		},
	}
	cfg.Buffer.Props.FilePath = "test.go"

	cs := &CursorState{
		Cursor: buffer.Position{Line: 0, ByteCol: 3},
	}
	toggleComment(cfg, cs, buf)
	if string(buf.Line(0)) != "// hello" {
		t.Fatalf("expected '// hello', got %q",
			string(buf.Line(0)))
	}
	// Toggle back.
	toggleComment(cfg, cs, buf)
	if string(buf.Line(0)) != "hello" {
		t.Fatalf("expected 'hello', got %q",
			string(buf.Line(0)))
	}
}

func loadBuf(t *testing.T, s string) *buffer.Buffer {
	t.Helper()
	buf, err := buffer.Load(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	buf.EnableUndo(nil)
	return buf
}

func TestToggleCommentAdd(t *testing.T) {
	buf := loadBuf(t, "func main() {\n\tfmt.Println()\n}")
	cfg := EditorCfg{
		Buffer: buf,
		LangConfigs: map[string]LangConfig{
			".go": {CommentLine: "//"},
		},
	}
	cfg.Buffer.Props.FilePath = "main.go"

	cs := &CursorState{
		Anchor: buffer.Position{Line: 0, ByteCol: 0},
		Cursor: buffer.Position{Line: 2, ByteCol: 1},
	}
	toggleComment(cfg, cs, buf)

	for li := range 3 {
		line := string(buf.Line(li))
		if len(line) < 2 || line[:2] != "//" {
			t.Errorf("line %d not commented: %q", li, line)
		}
	}
}

func TestToggleCommentRemove(t *testing.T) {
	buf := loadBuf(t, "// func main() {\n// \tfmt.Println()\n// }")
	cfg := EditorCfg{
		Buffer: buf,
		LangConfigs: map[string]LangConfig{
			".go": {CommentLine: "//"},
		},
	}
	cfg.Buffer.Props.FilePath = "main.go"

	cs := &CursorState{
		Anchor: buffer.Position{Line: 0, ByteCol: 0},
		Cursor: buffer.Position{Line: 2, ByteCol: 4},
	}
	toggleComment(cfg, cs, buf)

	for li := range 3 {
		line := string(buf.Line(li))
		if len(line) >= 2 && line[:2] == "//" {
			t.Errorf("line %d still commented: %q", li, line)
		}
	}
}

func TestToggleCommentNoOp(t *testing.T) {
	buf := loadBuf(t, "hello")
	cfg := EditorCfg{Buffer: buf}

	cs := &CursorState{
		Cursor: buffer.Position{Line: 0, ByteCol: 0},
	}
	toggleComment(cfg, cs, buf)
	if string(buf.Line(0)) != "hello" {
		t.Fatalf("expected no change, got %q", string(buf.Line(0)))
	}
}

func TestToggleCommentSkipsBlankLines(t *testing.T) {
	buf := loadBuf(t, "a\n\nb")
	cfg := EditorCfg{
		Buffer: buf,
		LangConfigs: map[string]LangConfig{
			".py": {CommentLine: "#"},
		},
	}
	cfg.Buffer.Props.FilePath = "test.py"

	cs := &CursorState{
		Anchor: buffer.Position{Line: 0, ByteCol: 0},
		Cursor: buffer.Position{Line: 2, ByteCol: 1},
	}
	toggleComment(cfg, cs, buf)

	if string(buf.Line(0)) != "# a" {
		t.Errorf("line 0: %q", string(buf.Line(0)))
	}
	if string(buf.Line(1)) != "" {
		t.Errorf("blank line modified: %q", string(buf.Line(1)))
	}
	if string(buf.Line(2)) != "# b" {
		t.Errorf("line 2: %q", string(buf.Line(2)))
	}
}

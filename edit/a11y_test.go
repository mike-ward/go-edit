package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
)

func TestEditorA11YLabelFromFilePath(t *testing.T) {
	buf := buffer.New()
	buf.Props.FilePath = "/home/user/main.go"
	cfg := EditorCfg{Buffer: buf}
	if got := editorA11YLabel(cfg); got != "main.go" {
		t.Fatalf("expected main.go, got %q", got)
	}
}

func TestEditorA11YLabelUntitled(t *testing.T) {
	cfg := EditorCfg{Buffer: buffer.New()}
	if got := editorA11YLabel(cfg); got != "Untitled" {
		t.Fatalf("expected Untitled, got %q", got)
	}
}

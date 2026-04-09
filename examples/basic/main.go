// Command basic mounts an edit.Editor in a go-gui window, loading
// a file from argv[1] (or a built-in sample if no arg is given).
//
// Requires the CGO-linked go-gui backend. Build/run locally:
//
//	go run ./examples/basic /path/to/somefile.go
package main

import (
	"fmt"
	"os"

	"github.com/mike-ward/go-edit/edit"
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-gui/gui/backend"
)

const sample = `package main

import "fmt"

func main() {
	fmt.Println("hello, go-edit")
}
`

const (
	winWidth           = 900
	winHeight          = 600
	focusEditor uint32 = 1
)

func loadBuffer() (*buffer.Buffer, string, error) {
	if len(os.Args) < 2 {
		return buffer.FromBytes([]byte(sample)), "<sample>", nil
	}
	buf, err := buffer.LoadFile(os.Args[1])
	if err != nil {
		return nil, "", err
	}
	return buf, os.Args[1], nil
}

func main() {
	buf, title, err := loadBuffer()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	buf.EnableUndo(nil)

	view := func(w *gui.Window) gui.View {
		ww, wh := w.WindowSize()
		return edit.Editor(edit.EditorCfg{
			IDFocus:         focusEditor,
			Buffer:          buf,
			Width:           float32(ww),
			Height:          float32(wh),
			ShowLineNumbers: true,
		})
	}

	w := gui.NewWindow(gui.WindowCfg{
		Title:  "go-edit: " + title,
		Width:  winWidth,
		Height: winHeight,
		OnInit: func(w *gui.Window) {
			w.UpdateView(view)
			w.SetIDFocus(focusEditor)
		},
	})

	backend.Run(w)
}

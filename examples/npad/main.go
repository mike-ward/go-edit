// Command npad is a kitchen-sink editor example showcasing all
// go-edit widget features: native menus, file I/O, dirty state,
// syntax highlighting, theme switching, and a status bar.
//
//	go run ./examples/npad [file]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/mike-ward/go-edit/edit"
	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-edit/edit/highlight"
	"github.com/mike-ward/go-gui/gui"
	"github.com/mike-ward/go-gui/gui/backend"
)

const (
	winWidth           = 1000
	winHeight          = 700
	focusEditor uint32 = 1
	statusBarH         = 32
	maxRecent          = 10
)

// appState holds per-window state accessed via gui.State[appState](w).
type appState struct {
	Buf      *buffer.Buffer
	HL       *highlight.Highlighter
	FilePath string

	ShowLineNumbers  bool
	ShowBracketMatch bool
	EnableFolding    bool
	LineWrap         bool
	StickyScroll     bool
	ShowWhitespace   edit.WhitespaceMode

	ChromaStyleIdx int
	RecentFiles    []string
}

// chromaStyleNames is populated at init from chroma's registry.
var chromaStyleNames []string

// langConfigs provides per-extension editor settings.
var langConfigs = map[string]edit.LangConfig{
	".go":        {TabWidth: 4, UseTabs: true, CommentLine: "//"},
	".py":        {TabWidth: 4, CommentLine: "#"},
	".js":        {TabWidth: 2, CommentLine: "//"},
	".jsx":       {TabWidth: 2, CommentLine: "//"},
	".ts":        {TabWidth: 2, CommentLine: "//"},
	".tsx":       {TabWidth: 2, CommentLine: "//"},
	".rs":        {TabWidth: 4, CommentLine: "//"},
	".c":         {TabWidth: 4, CommentLine: "//", CommentBlockStart: "/*", CommentBlockEnd: "*/"},
	".h":         {TabWidth: 4, CommentLine: "//", CommentBlockStart: "/*", CommentBlockEnd: "*/"},
	".cpp":       {TabWidth: 4, CommentLine: "//", CommentBlockStart: "/*", CommentBlockEnd: "*/"},
	".java":      {TabWidth: 4, CommentLine: "//", CommentBlockStart: "/*", CommentBlockEnd: "*/"},
	".rb":        {TabWidth: 2, CommentLine: "#"},
	".sh":        {TabWidth: 4, CommentLine: "#"},
	".bash":      {TabWidth: 4, CommentLine: "#"},
	".zsh":       {TabWidth: 4, CommentLine: "#"},
	".lua":       {TabWidth: 4, CommentLine: "--"},
	".sql":       {TabWidth: 4, CommentLine: "--"},
	".html":      {TabWidth: 2, CommentBlockStart: "<!--", CommentBlockEnd: "-->"},
	".css":       {TabWidth: 2, CommentBlockStart: "/*", CommentBlockEnd: "*/"},
	".json":      {TabWidth: 2},
	".yaml":      {TabWidth: 2, CommentLine: "#"},
	".yml":       {TabWidth: 2, CommentLine: "#"},
	".toml":      {TabWidth: 2, CommentLine: "#"},
	".xml":       {TabWidth: 2, CommentBlockStart: "<!--", CommentBlockEnd: "-->"},
	".md":        {TabWidth: 4},
	"Makefile":   {TabWidth: 8, UseTabs: true, CommentLine: "#"},
	"Dockerfile": {TabWidth: 4, CommentLine: "#"},
}

// Reference to the gui.App so native menus can be rebuilt.
var gApp *gui.App

func init() {
	chromaStyleNames = styles.Names()
	slices.Sort(chromaStyleNames)
}

func main() {
	gui.SetTheme(gui.ThemeDarkBordered)

	st := &appState{
		ShowLineNumbers:  true,
		ShowBracketMatch: true,
		EnableFolding:    true,
		ChromaStyleIdx:   slices.Index(chromaStyleNames, "monokai"),
	}
	if st.ChromaStyleIdx < 0 {
		st.ChromaStyleIdx = 0
	}
	loadConfig(st)

	// Load file from argv or create empty buffer.
	if len(os.Args) > 1 {
		buf, err := buffer.LoadFile(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		st.Buf = buf
		st.FilePath = os.Args[1]
	} else {
		st.Buf = buffer.New()
	}
	st.Buf.EnableUndo(nil)

	title := windowTitle(st)

	gApp = gui.NewApp()
	w := gui.NewWindow(gui.WindowCfg{
		State:  st,
		Title:  title,
		Width:  winWidth,
		Height: winHeight,
		OnInit: func(w *gui.Window) {
			s := gui.State[appState](w)
			s.HL = createHighlighter(s)
			registerCommands(w)
			rebuildMenu(s, w)
			w.UpdateView(mainView)
			w.SetIDFocus(focusEditor)
		},
	})

	backend.RunApp(gApp, w)
}

// syncTitle updates the OS window title if it differs from the
// computed one. Cheap no-op when unchanged.
func syncTitle(w *gui.Window, s *appState) {
	if t := windowTitle(s); t != w.Config.Title {
		w.SetTitle(t)
	}
}

// windowTitle returns the title bar string.
func windowTitle(s *appState) string {
	name := "Untitled"
	if s.FilePath != "" {
		name = filepath.Base(s.FilePath)
	}
	if s.Buf.Dirty() {
		return fmt.Sprintf("npad — %s [modified]", name)
	}
	return fmt.Sprintf("npad — %s", name)
}

// createHighlighter builds a Highlighter for the current buffer
// and chroma style. Returns nil if no lexer matches.
func createHighlighter(s *appState) *highlight.Highlighter {
	styleName := chromaStyleNames[s.ChromaStyleIdx]
	style := styles.Get(styleName)
	return highlight.New(s.Buf, "", style)
}

// editorTheme builds an EditorTheme from the active chroma style.
func editorTheme(s *appState) edit.EditorTheme {
	style := styles.Get(chromaStyleNames[s.ChromaStyleIdx])
	if style == nil {
		return edit.EditorTheme{}
	}
	bg := style.Get(chroma.Background).Background
	if !bg.IsSet() {
		return edit.EditorTheme{}
	}
	return edit.EditorTheme{
		Background: gui.RGBA(bg.Red(), bg.Green(), bg.Blue(), 255),
	}
}

// ---- Main View ----

func mainView(w *gui.Window) gui.View {
	ww, wh := w.WindowSize()
	s := gui.State[appState](w)
	theme := gui.CurrentTheme()

	// Sync OS title so dirty-flag transitions from typing update
	// "[modified]". Explicit syncTitle calls cover file ops.
	syncTitle(w, s)

	editorW := float32(ww)
	editorH := float32(wh) - statusBarH

	var decos []buffer.DecorationProvider
	if s.HL != nil {
		decos = []buffer.DecorationProvider{s.HL}
	}

	editorView := edit.Editor(edit.EditorCfg{
		IDFocus:          focusEditor,
		Buffer:           s.Buf,
		Width:            editorW,
		Height:           editorH,
		Padding:          gui.NoPadding,
		SizeBorder:       gui.Some[float32](0),
		ShowLineNumbers:  s.ShowLineNumbers,
		ShowBracketMatch: s.ShowBracketMatch,
		ShowWhitespace:   s.ShowWhitespace,
		EnableFolding:    s.EnableFolding,
		LineWrap:         s.LineWrap,
		StickyScroll:     s.StickyScroll,
		LangConfigs:      langConfigs,
		Theme:            editorTheme(s),
		Decorations:      decos,
		OnInvalidate: func(redraw func()) {
			if s.HL != nil {
				s.HL.SetInvalidateFunc(redraw)
			}
		},
	})

	return gui.Column(gui.ContainerCfg{
		Width:      float32(ww),
		Height:     float32(wh),
		Sizing:     gui.FixedFixed,
		Padding:    gui.NoPadding,
		SizeBorder: gui.NoBorder,
		Spacing:    gui.Some[float32](0),
		Content: []gui.View{
			editorView,
			statusBar(w, s, theme),
		},
	})
}

// ---- Status Bar ----

func statusBar(
	w *gui.Window,
	s *appState,
	theme gui.Theme,
) gui.View {
	line, col, _ := edit.CursorPos(w, focusEditor)
	posText := fmt.Sprintf("  Ln %d, Col %d", line+1, col+1)
	langText := langLabel(s.FilePath)
	eol := eolLabel(s.Buf.Props.EOL)
	enc := encodingLabel(s.Buf.Props.Encoding)

	dirty := ""
	if s.Buf.Dirty() {
		dirty = "  [modified]"
	}

	ts := theme.M4
	ts.Color = gui.RGBA(180, 180, 180, 255)

	return gui.Row(gui.ContainerCfg{
		Height:    statusBarH,
		MinHeight: statusBarH,
		Sizing:    gui.FillFixed,
		Color:     gui.RGBA(30, 30, 30, 255),
		Padding:   gui.NoPadding,
		VAlign:    gui.VAlignMiddle,
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: posText + dirty, TextStyle: ts}),
			// spacer
			gui.Row(gui.ContainerCfg{
				Sizing:  gui.FillFill,
				Padding: gui.NoPadding,
			}),
			gui.Text(gui.TextCfg{Text: langText + "  ", TextStyle: ts}),
			gui.Text(gui.TextCfg{Text: eol + "  ", TextStyle: ts}),
			gui.Text(gui.TextCfg{Text: enc + "  ", TextStyle: ts}),
		},
	})
}

func langLabel(fp string) string {
	if fp == "" {
		return "Plain Text"
	}
	ext := strings.ToLower(filepath.Ext(fp))
	switch ext {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js":
		return "JavaScript"
	case ".jsx":
		return "JSX"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TSX"
	case ".rs":
		return "Rust"
	case ".c":
		return "C"
	case ".h":
		return "C/C++ Header"
	case ".cpp", ".cc", ".cxx":
		return "C++"
	case ".java":
		return "Java"
	case ".rb":
		return "Ruby"
	case ".sh", ".bash", ".zsh":
		return "Shell"
	case ".lua":
		return "Lua"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".xml":
		return "XML"
	case ".md":
		return "Markdown"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func eolLabel(eol buffer.EOL) string {
	switch eol {
	case buffer.EOLLF:
		return "LF"
	case buffer.EOLCRLF:
		return "CRLF"
	case buffer.EOLCR:
		return "CR"
	case buffer.EOLMixed:
		return "Mixed"
	default:
		return "LF"
	}
}

func encodingLabel(enc buffer.Encoding) string {
	switch enc {
	case buffer.EncodingUTF8:
		return "UTF-8"
	case buffer.EncodingUTF8BOM:
		return "UTF-8 BOM"
	case buffer.EncodingUTF16LE:
		return "UTF-16 LE"
	case buffer.EncodingUTF16BE:
		return "UTF-16 BE"
	case buffer.EncodingLatin1:
		return "Latin-1"
	case buffer.EncodingCP1252:
		return "Windows-1252"
	case buffer.EncodingRaw:
		return "Raw"
	default:
		return "UTF-8"
	}
}

// ---- Commands ----

func registerCommands(w *gui.Window) {
	_ = w.RegisterCommands(
		gui.Command{
			ID: "file.new", Label: "New",
			Execute: func(_ *gui.Event, w *gui.Window) { cmdNew(w) },
		},
		gui.Command{
			ID: "file.open", Label: "Open…",
			Execute: func(_ *gui.Event, w *gui.Window) { cmdOpen(w) },
		},
		gui.Command{
			ID: "file.save", Label: "Save",
			Execute: func(_ *gui.Event, w *gui.Window) { cmdSave(w) },
		},
		gui.Command{
			ID: "file.saveAs", Label: "Save As…",
			Execute: func(_ *gui.Event, w *gui.Window) { cmdSaveAs(w) },
		},
		gui.Command{
			ID: "file.close", Label: "Close",
			Execute: func(_ *gui.Event, w *gui.Window) { cmdClose(w) },
		},
	)
}

// ---- File Commands ----

func cmdNew(w *gui.Window) {
	s := gui.State[appState](w)
	if s.Buf.Dirty() {
		confirmSave(w, "Save changes before creating new file?", func(w *gui.Window) {
			doNew(w)
		})
		return
	}
	doNew(w)
}

func doNew(w *gui.Window) {
	s := gui.State[appState](w)
	if s.HL != nil {
		s.HL.Close()
		s.HL = nil
	}
	s.Buf = buffer.New()
	s.Buf.EnableUndo(nil)
	s.FilePath = ""
	s.HL = createHighlighter(s)
	syncTitle(w, s)
	w.UpdateWindow()
}

func cmdOpen(w *gui.Window) {
	s := gui.State[appState](w)
	if s.Buf.Dirty() {
		confirmSave(w, "Save changes before opening?", func(w *gui.Window) {
			doOpenDialog(w)
		})
		return
	}
	doOpenDialog(w)
}

func doOpenDialog(w *gui.Window) {
	w.NativeOpenDialog(gui.NativeOpenDialogCfg{
		Title: "Open File",
		OnDone: func(result gui.NativeDialogResult, w *gui.Window) {
			if result.Status != gui.DialogOK || len(result.Paths) == 0 {
				return
			}
			openFile(w, result.Paths[0].Path)
		},
	})
}

func openFile(w *gui.Window, path string) {
	buf, err := buffer.LoadFile(path)
	if err != nil {
		w.NativeMessageDialog(gui.NativeMessageDialogCfg{
			Title: "Error",
			Body:  fmt.Sprintf("Failed to open file:\n%v", err),
			Level: gui.AlertCritical,
		})
		return
	}

	s := gui.State[appState](w)
	if s.HL != nil {
		s.HL.Close()
	}
	buf.EnableUndo(nil)
	s.Buf = buf
	s.FilePath = path
	s.HL = createHighlighter(s)
	addRecentFile(s, path)
	saveConfig(s)
	rebuildMenu(s, w)
	syncTitle(w, s)
	w.UpdateWindow()
}

func cmdSave(w *gui.Window) {
	s := gui.State[appState](w)
	if s.FilePath == "" {
		cmdSaveAs(w)
		return
	}
	doSave(w, s.FilePath)
}

func cmdSaveAs(w *gui.Window) {
	s := gui.State[appState](w)
	defName := ""
	if s.FilePath != "" {
		defName = filepath.Base(s.FilePath)
	}
	w.NativeSaveDialog(gui.NativeSaveDialogCfg{
		Title:            "Save As",
		DefaultName:      defName,
		ConfirmOverwrite: true,
		OnDone: func(result gui.NativeDialogResult, w *gui.Window) {
			if result.Status != gui.DialogOK || len(result.Paths) == 0 {
				return
			}
			doSave(w, result.Paths[0].Path)
		},
	})
}

func doSave(w *gui.Window, path string) {
	s := gui.State[appState](w)

	// Update file path before saving so SaveFile uses it.
	s.Buf.Props.FilePath = path
	if err := s.Buf.SaveFile(path); err != nil {
		w.NativeMessageDialog(gui.NativeMessageDialogCfg{
			Title: "Error",
			Body:  fmt.Sprintf("Failed to save:\n%v", err),
			Level: gui.AlertCritical,
		})
		return
	}
	s.Buf.MarkClean()

	oldPath := s.FilePath
	s.FilePath = path
	addRecentFile(s, path)
	saveConfig(s)

	// Recreate highlighter if extension changed (new lexer).
	newExt := filepath.Ext(path)
	oldExt := filepath.Ext(oldPath)
	if newExt != oldExt || s.HL == nil {
		if s.HL != nil {
			s.HL.Close()
		}
		s.HL = createHighlighter(s)
	}
	rebuildMenu(s, w)
	syncTitle(w, s)
	w.UpdateWindow()
}

func cmdClose(w *gui.Window) {
	s := gui.State[appState](w)
	if s.Buf.Dirty() {
		confirmSave(w, "Save changes before closing?", func(w *gui.Window) {
			doNew(w) // close = reset to empty
		})
		return
	}
	doNew(w)
}

// confirmSave prompts Save/Don't Save/Cancel when there are unsaved changes.
// onConfirm is called after a successful save or after discard.
func confirmSave(w *gui.Window, msg string, onConfirm func(*gui.Window)) {
	if onConfirm == nil {
		return
	}
	s := gui.State[appState](w)
	w.NativeSaveDiscardDialog(gui.NativeSaveDiscardDialogCfg{
		Title: "Unsaved Changes",
		Body:  msg,
		Level: gui.AlertWarning,
		OnDone: func(result gui.NativeAlertResult, w *gui.Window) {
			switch result.Status {
			case gui.DialogOK:
				// Save then proceed.
				if s.FilePath != "" {
					doSave(w, s.FilePath)
					onConfirm(w)
				} else {
					// Open SaveAs dialog; call onConfirm after save.
					w.NativeSaveDialog(gui.NativeSaveDialogCfg{
						Title:            "Save As",
						ConfirmOverwrite: true,
						OnDone: func(r gui.NativeDialogResult, w *gui.Window) {
							if r.Status != gui.DialogOK || len(r.Paths) == 0 {
								return
							}
							doSave(w, r.Paths[0].Path)
							onConfirm(w)
						},
					})
				}
			case gui.DialogDiscard:
				onConfirm(w)
				// DialogCancel / DialogError: do nothing.
			}
		},
	})
}

// ---- Recent Files ----

func addRecentFile(s *appState, path string) {
	// Remove duplicate if present.
	s.RecentFiles = slices.DeleteFunc(s.RecentFiles, func(p string) bool {
		return p == path
	})
	// Prepend.
	s.RecentFiles = slices.Insert(s.RecentFiles, 0, path)
	if len(s.RecentFiles) > maxRecent {
		s.RecentFiles = s.RecentFiles[:maxRecent]
	}
}

// ---- Native Menu ----

func rebuildMenu(s *appState, w *gui.Window) {
	// File > Recent Files submenu.
	var recentItems []gui.NativeMenuItemCfg
	for i, path := range s.RecentFiles {
		recentItems = append(recentItems, gui.NativeMenuItemCfg{
			ID:   fmt.Sprintf("recent.%d", i),
			Text: filepath.Base(path),
		})
	}
	if len(recentItems) > 0 {
		recentItems = append(recentItems,
			gui.NativeMenuItemCfg{Separator: true},
			gui.NativeMenuItemCfg{
				ID: "recent.clear", Text: "Clear Recent",
			},
		)
	}

	// View > Syntax Theme submenu.
	var syntaxItems []gui.NativeMenuItemCfg
	for i, name := range chromaStyleNames {
		syntaxItems = append(syntaxItems, gui.NativeMenuItemCfg{
			ID:      fmt.Sprintf("theme.syntax.%d", i),
			Text:    name,
			Checked: i == s.ChromaStyleIdx,
		})
	}

	gApp.SetNativeMenubar(gui.NativeMenubarCfg{
		AppName:                 "npad",
		SuppressSystemEditItems: true,
		AboutActionID:           "help.about",
		Menus: []gui.NativeMenuCfg{
			{
				Title: "File",
				Items: []gui.NativeMenuItemCfg{
					{ID: "file.new", Text: "New", CommandID: "file.new",
						Shortcut: gui.Shortcut{Key: gui.KeyN, Modifiers: gui.ModSuper}},
					{ID: "file.open", Text: "Open…", CommandID: "file.open",
						Shortcut: gui.Shortcut{Key: gui.KeyO, Modifiers: gui.ModSuper}},
					{Separator: true},
					{ID: "file.save", Text: "Save", CommandID: "file.save",
						Shortcut: gui.Shortcut{Key: gui.KeyS, Modifiers: gui.ModSuper}},
					{ID: "file.saveAs", Text: "Save As…", CommandID: "file.saveAs",
						Shortcut: gui.Shortcut{Key: gui.KeyS, Modifiers: gui.ModSuper | gui.ModShift}},
					{Separator: true},
					{ID: "recent", Text: "Recent Files", Submenu: recentItems,
						Disabled: len(recentItems) == 0},
					{Separator: true},
					{ID: "file.close", Text: "Close", CommandID: "file.close",
						Shortcut: gui.Shortcut{Key: gui.KeyW, Modifiers: gui.ModSuper}},
				},
			},
			{
				Title: "Edit",
				Items: []gui.NativeMenuItemCfg{
					{ID: "edit.undo", Text: "Undo",
						Shortcut: gui.Shortcut{Key: gui.KeyZ, Modifiers: gui.ModSuper}},
					{ID: "edit.redo", Text: "Redo",
						Shortcut: gui.Shortcut{Key: gui.KeyZ, Modifiers: gui.ModSuper | gui.ModShift}},
					{Separator: true},
					{ID: "find.open", Text: "Find",
						Shortcut: gui.Shortcut{Key: gui.KeyF, Modifiers: gui.ModSuper}},
					{ID: "find.openReplace", Text: "Replace",
						Shortcut: gui.Shortcut{Key: gui.KeyH, Modifiers: gui.ModSuper}},
					{Separator: true},
					{ID: "edit.toggleComment", Text: "Toggle Comment",
						Shortcut: gui.Shortcut{Key: gui.KeySlash, Modifiers: gui.ModSuper}},
				},
			},
			{
				Title: "View",
				Items: []gui.NativeMenuItemCfg{
					{ID: "view.lineNumbers", Text: "Line Numbers",
						Checked: s.ShowLineNumbers},
					{ID: "view.wordWrap", Text: "Word Wrap",
						Checked: s.LineWrap},
					{ID: "view.bracketMatch", Text: "Bracket Match",
						Checked: s.ShowBracketMatch},
					{ID: "view.folding", Text: "Code Folding",
						Checked: s.EnableFolding},
					{ID: "view.stickyScroll", Text: "Sticky Scroll",
						Checked: s.StickyScroll},
					{ID: "view.whitespace", Text: "Whitespace",
						Checked: s.ShowWhitespace != edit.WhitespaceNone},
					{Separator: true},
					{ID: "theme.syntax", Text: "Syntax Theme",
						Submenu: syntaxItems},
				},
			},
			{
				Title: "Help",
				Items: []gui.NativeMenuItemCfg{
					{ID: "help.keys", Text: "Keyboard Shortcuts"},
					{Separator: true},
					{ID: "help.about", Text: "About npad"},
				},
			},
		},
		OnAction: func(id string) {
			w.QueueCommand(func(w *gui.Window) {
				handleMenuAction(id, w)
			})
		},
	})
}

func handleMenuAction(id string, w *gui.Window) {
	s := gui.State[appState](w)

	switch {
	// Edit actions delegated to the editor widget.
	case id == "edit.undo":
		edit.TriggerAction(w, focusEditor, "edit.undo")
	case id == "edit.redo":
		edit.TriggerAction(w, focusEditor, "edit.redo")
	case id == "find.open":
		edit.TriggerAction(w, focusEditor, "find.open")
	case id == "find.openReplace":
		edit.TriggerAction(w, focusEditor, "find.openReplace")
	case id == "edit.toggleComment":
		edit.TriggerAction(w, focusEditor, "edit.toggleComment")

	// View toggles.
	case id == "view.lineNumbers":
		s.ShowLineNumbers = !s.ShowLineNumbers
		rebuildMenu(s, w)
	case id == "view.wordWrap":
		s.LineWrap = !s.LineWrap
		rebuildMenu(s, w)
	case id == "view.bracketMatch":
		s.ShowBracketMatch = !s.ShowBracketMatch
		rebuildMenu(s, w)
	case id == "view.folding":
		s.EnableFolding = !s.EnableFolding
		rebuildMenu(s, w)
	case id == "view.stickyScroll":
		s.StickyScroll = !s.StickyScroll
		rebuildMenu(s, w)
	case id == "view.whitespace":
		if s.ShowWhitespace == edit.WhitespaceNone {
			s.ShowWhitespace = edit.WhitespaceAll
		} else {
			s.ShowWhitespace = edit.WhitespaceNone
		}
		rebuildMenu(s, w)

	// Syntax theme.
	case strings.HasPrefix(id, "theme.syntax."):
		var idx int
		if _, err := fmt.Sscanf(id, "theme.syntax.%d", &idx); err == nil &&
			idx >= 0 && idx < len(chromaStyleNames) {
			s.ChromaStyleIdx = idx
			if s.HL != nil {
				s.HL.Close()
			}
			s.HL = createHighlighter(s)
			saveConfig(s)
			rebuildMenu(s, w)
			w.UpdateView(mainView)
		}

	// Recent files.
	case strings.HasPrefix(id, "recent."):
		if id == "recent.clear" {
			s.RecentFiles = nil
			saveConfig(s)
			rebuildMenu(s, w)
			return
		}
		var idx int
		if _, err := fmt.Sscanf(id, "recent.%d", &idx); err == nil &&
			idx >= 0 && idx < len(s.RecentFiles) {
			openFile(w, s.RecentFiles[idx])
		}

	// Help.
	case id == "help.keys":
		w.NativeMessageDialog(gui.NativeMessageDialogCfg{
			Title: "Keyboard Shortcuts",
			Body: "Ctrl/Cmd+N  New\n" +
				"Ctrl/Cmd+O  Open\n" +
				"Ctrl/Cmd+S  Save\n" +
				"Ctrl/Cmd+Shift+S  Save As\n" +
				"Ctrl/Cmd+W  Close\n" +
				"Ctrl/Cmd+Z  Undo\n" +
				"Ctrl/Cmd+Shift+Z  Redo\n" +
				"Ctrl/Cmd+F  Find\n" +
				"Ctrl/Cmd+H  Replace\n" +
				"Ctrl/Cmd+/  Toggle Comment\n" +
				"Ctrl/Cmd+D  Add Next Occurrence\n" +
				"Alt+Z  Toggle Word Wrap\n" +
				"F1  Help Overlay",
			Level: gui.AlertInfo,
		})

	case id == "help.about":
		w.NativeMessageDialog(gui.NativeMessageDialogCfg{
			Title: "About npad",
			Body: "npad — a go-edit showcase editor\n" +
				"Built with go-gui\n\n" +
				"https://github.com/mike-ward/go-edit",
			Level: gui.AlertInfo,
		})
	}
}

// ---- Config persistence ----

// npadConfig is persisted to disk across sessions.
type npadConfig struct {
	ChromaStyleIdx int      `json:"chromaStyleIdx"`
	RecentFiles    []string `json:"recentFiles,omitempty"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "npad", "state.json")
}

// maxConfigBytes caps the config file read to prevent OOM on a
// corrupt or malicious file (a few KB is ample for valid state).
const maxConfigBytes = 64 * 1024

// maxPathBytes caps individual recent-file path length.
const maxPathBytes = 4096

func loadConfig(s *appState) {
	if s == nil {
		return
	}
	p := configPath()
	if p == "" {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	data := make([]byte, maxConfigBytes+1)
	n, _ := f.Read(data)
	if n > maxConfigBytes {
		return // file too large; ignore
	}
	var cfg npadConfig
	if err := json.Unmarshal(data[:n], &cfg); err != nil {
		return
	}
	if cfg.ChromaStyleIdx >= 0 && cfg.ChromaStyleIdx < len(chromaStyleNames) {
		s.ChromaStyleIdx = cfg.ChromaStyleIdx
	}
	recent := cfg.RecentFiles[:min(len(cfg.RecentFiles), maxRecent)]
	s.RecentFiles = s.RecentFiles[:0]
	for _, p := range recent {
		if p != "" && len(p) <= maxPathBytes {
			s.RecentFiles = append(s.RecentFiles, p)
		}
	}
}

func saveConfig(s *appState) {
	if s == nil {
		return
	}
	p := configPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(npadConfig{
		ChromaStyleIdx: s.ChromaStyleIdx,
		RecentFiles:    s.RecentFiles,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o644)
}

package edit

import (
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// setCursorEnd places the primary cursor at the end of line 0.
func setCursorEnd(d *driver, focusID uint32, line []byte) {
	col := len(line)
	st := loadState(d.w, focusID)
	st.Cursors = []CursorState{mkCursor(0, col)}
	storeState(d.w, focusID, st)
}

// ---------- TriggerAction behaviour ----------

func TestTriggerAction_ExecutedOnNextAmendLayout(t *testing.T) {
	buf := mkBuf("hello")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 400, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	setCursorEnd(d, 400, buf.Line(0))
	d.sendChar('!')
	if string(buf.Line(0)) != "hello!" {
		t.Fatalf("setup: want %q got %q", "hello!", string(buf.Line(0)))
	}

	TriggerAction(d.w, 400, "edit.undo")
	d.tick() // AmendLayout executes the pending action.

	if string(buf.Line(0)) != "hello" {
		t.Errorf("after undo: want %q got %q", "hello", string(buf.Line(0)))
	}
}

func TestTriggerAction_ClearedAfterTick(t *testing.T) {
	buf := mkBuf("hello")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 401, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()
	setCursorEnd(d, 401, buf.Line(0))
	d.sendChar('!')

	TriggerAction(d.w, 401, "edit.undo")
	d.tick()

	st := loadState(d.w, 401)
	if st.PendingAction != "" {
		t.Errorf("PendingAction=%q after tick, want empty", st.PendingAction)
	}
}

func TestTriggerAction_NotExecutedTwice(t *testing.T) {
	// Use a custom action with a counter to confirm it fires exactly
	// once regardless of how many ticks follow.
	buf := mkBuf("hello")
	count := 0
	d := newDriver(EditorCfg{
		IDFocus: 402,
		Buffer:  buf,
		Width:   400,
		Height:  200,
		Actions: map[string]Action{
			"test.counter": {
				ID: "test.counter",
				Execute: ActionFunc(func(_ EditorCfg, _ *editorState,
					_ *buffer.Buffer, _ *gui.Window,
				) {
					count++
				}),
			},
		},
	})
	d.tick()

	TriggerAction(d.w, 402, "test.counter")
	d.tick()
	d.tick()
	d.tick()

	if count != 1 {
		t.Errorf("action fired %d times, want 1", count)
	}
}

func TestTriggerAction_CfgActionsOverrideDefault(t *testing.T) {
	buf := mkBuf("hello")
	buf.EnableUndo(nil)
	fired := false

	d := newDriver(EditorCfg{
		IDFocus: 403,
		Buffer:  buf,
		Width:   400,
		Height:  200,
		Actions: map[string]Action{
			"edit.undo": {
				ID: "edit.undo",
				Execute: ActionFunc(func(_ EditorCfg, _ *editorState,
					_ *buffer.Buffer, _ *gui.Window,
				) {
					fired = true
				}),
			},
		},
	})
	d.tick()
	setCursorEnd(d, 403, buf.Line(0))
	d.sendChar('!')
	before := string(buf.Line(0)) // "hello!"

	TriggerAction(d.w, 403, "edit.undo")
	d.tick()

	if !fired {
		t.Error("cfg.Actions override was not called")
	}
	// Custom action doesn't call buf.Undo(), so buffer is unchanged.
	if string(buf.Line(0)) != before {
		t.Errorf("custom action mutated buffer; want %q got %q",
			before, string(buf.Line(0)))
	}
}

func TestTriggerAction_PerCursorDispatchMultiCursor(t *testing.T) {
	// edit.toggleComment is PerCursor:true; with two cursors both
	// lines should be commented.
	buf := mkBuf("hello\nworld")
	buf.Props.FilePath = "test.go"
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{
		IDFocus: 404,
		Buffer:  buf,
		Width:   400,
		Height:  200,
		LangConfigs: map[string]LangConfig{
			".go": {CommentLine: "//"},
		},
	})
	d.tick()
	// Place cursors on both lines.
	st := loadState(d.w, 404)
	st.Cursors = []CursorState{mkCursor(0, 0), mkCursor(1, 0)}
	storeState(d.w, 404, st)

	TriggerAction(d.w, 404, "edit.toggleComment")
	d.tick()

	line0 := string(buf.Line(0))
	line1 := string(buf.Line(1))
	if line0 == "hello" {
		t.Errorf("line 0 not commented; got %q", line0)
	}
	if line1 == "world" {
		t.Errorf("line 1 not commented; got %q", line1)
	}
}

func TestTriggerAction_FindOpenActivatesSearch(t *testing.T) {
	buf := mkBuf("hello world")
	d := newDriver(EditorCfg{
		IDFocus: 405, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()

	TriggerAction(d.w, 405, "find.open")
	d.tick()

	st := d.state()
	if !st.Search.Active {
		t.Error("find.open: Search.Active=false, want true")
	}
	if st.Search.ShowReplace {
		t.Error("find.open: ShowReplace=true, want false")
	}
}

func TestTriggerAction_FindOpenReplaceActivatesReplace(t *testing.T) {
	buf := mkBuf("hello world")
	d := newDriver(EditorCfg{
		IDFocus: 406, Buffer: buf, Width: 400, Height: 200,
	})
	d.tick()

	TriggerAction(d.w, 406, "find.openReplace")
	d.tick()

	st := d.state()
	if !st.Search.Active {
		t.Error("find.openReplace: Search.Active=false, want true")
	}
	if !st.Search.ShowReplace {
		t.Error("find.openReplace: ShowReplace=false, want true")
	}
}

package edit

import (
	"io"
	"strings"
	"testing"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// ---------- findAllMatches ----------

func TestFindAllMatches_Literal(t *testing.T) {
	buf := mkBuf("hello world\nhello again\nfoo")

	matches, _ := findAllMatches(buf, "hello", true, false, buffer.Range{})
	if len(matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(matches))
	}
	if matches[0].Start.Line != 0 || matches[0].Start.ByteCol != 0 {
		t.Errorf("match 0: %+v", matches[0])
	}
	if matches[1].Start.Line != 1 || matches[1].Start.ByteCol != 0 {
		t.Errorf("match 1: %+v", matches[1])
	}
}

func TestFindAllMatches_NoMatch(t *testing.T) {
	buf := mkBuf("hello world")
	matches, _ := findAllMatches(buf, "xyz", true, false, buffer.Range{})
	if len(matches) != 0 {
		t.Fatalf("want 0 matches, got %d", len(matches))
	}
}

func TestFindAllMatches_Empty(t *testing.T) {
	buf := mkBuf("hello")
	matches, _ := findAllMatches(buf, "", true, false, buffer.Range{})
	if matches != nil {
		t.Fatalf("want nil, got %v", matches)
	}
}

func TestFindAllMatches_CaseInsensitive(t *testing.T) {
	buf := mkBuf("Hello HELLO hello")
	matches, _ := findAllMatches(buf, "hello", false, false, buffer.Range{})
	if len(matches) != 3 {
		t.Fatalf("want 3 matches, got %d", len(matches))
	}
}

func TestFindAllMatches_CaseSensitive(t *testing.T) {
	buf := mkBuf("Hello HELLO hello")
	matches, _ := findAllMatches(buf, "hello", true, false, buffer.Range{})
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].Start.ByteCol != 12 {
		t.Errorf("want col 12, got %d", matches[0].Start.ByteCol)
	}
}

func TestFindAllMatches_Regex(t *testing.T) {
	buf := mkBuf("foo123 bar456 baz")
	matches, re := findAllMatches(buf, `\d+`, true, true, buffer.Range{})
	if len(matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(matches))
	}
	if re == nil {
		t.Fatal("want non-nil compiled regexp")
	}
}

func TestFindAllMatches_InvalidRegex(t *testing.T) {
	buf := mkBuf("hello")
	matches, _ := findAllMatches(buf, "[invalid", true, true, buffer.Range{})
	if matches != nil {
		t.Fatalf("want nil for invalid regex, got %v", matches)
	}
}

func TestFindAllMatches_RegexCaseInsensitive(t *testing.T) {
	buf := mkBuf("Foo foo FOO")
	matches, _ := findAllMatches(buf, "foo", false, true, buffer.Range{})
	if len(matches) != 3 {
		t.Fatalf("want 3 matches, got %d", len(matches))
	}
}

func TestFindAllMatches_MultiplePerLine(t *testing.T) {
	buf := mkBuf("abab")
	matches, _ := findAllMatches(buf, "ab", true, false, buffer.Range{})
	if len(matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(matches))
	}
	if matches[0].Start.ByteCol != 0 {
		t.Errorf("match 0 col: %d", matches[0].Start.ByteCol)
	}
	if matches[1].Start.ByteCol != 2 {
		t.Errorf("match 1 col: %d", matches[1].Start.ByteCol)
	}
}

func TestFindAllMatches_Cap(t *testing.T) {
	// Build a buffer that would produce >maxMatches hits.
	var lines []byte
	for range maxMatches + 100 {
		lines = append(lines, 'a', '\n')
	}
	buf, err := buffer.Load(newReader(lines))
	if err != nil {
		t.Fatal(err)
	}

	matches, _ := findAllMatches(buf, "a", true, false, buffer.Range{})
	if len(matches) != maxMatches {
		t.Fatalf("want %d matches, got %d", maxMatches, len(matches))
	}
}

func TestFindAllMatches_InSelection(t *testing.T) {
	buf := mkBuf("aaa bbb aaa bbb aaa")
	scope := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 4},
		End:   buffer.Position{Line: 0, ByteCol: 15},
	}
	// "aaa bbb aaa bbb aaa" — scope covers "bbb aaa bbb"
	matches, _ := findAllMatches(buf, "aaa", true, false, scope)
	if len(matches) != 1 {
		t.Fatalf("want 1 match in scope, got %d", len(matches))
	}
	if matches[0].Start.ByteCol != 8 {
		t.Errorf("want col 8, got %d", matches[0].Start.ByteCol)
	}
}

func TestFindAllMatches_InSelection_MultiLine(t *testing.T) {
	buf := mkBuf("aaa\nbbb\naaa\nbbb\naaa")
	scope := buffer.Range{
		Start: buffer.Position{Line: 1, ByteCol: 0},
		End:   buffer.Position{Line: 3, ByteCol: 3},
	}
	matches, _ := findAllMatches(buf, "aaa", true, false, scope)
	if len(matches) != 1 {
		t.Fatalf("want 1 match in scope, got %d", len(matches))
	}
	if matches[0].Start.Line != 2 {
		t.Errorf("want line 2, got %d", matches[0].Start.Line)
	}
}

func TestDriver_FindInSelection(t *testing.T) {
	buf := mkBuf("aaa bbb aaa bbb aaa")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	// Select middle portion: "bbb aaa bbb" (col 4..15).
	st := d.state()
	st.Cursors[0].Anchor = buffer.Position{Line: 0, ByteCol: 4}
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 15}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyF, gui.ModCtrl)

	// Type query — multi-line selection auto-enables InSelection,
	// but single-line populates query instead. Toggle manually.
	d.sendKeyMod(gui.KeyS, gui.ModAlt) // won't work: no selection now

	// Restart: select, open, then toggle.
	d.sendKey(gui.KeyEscape)
	st = d.state()
	st.Cursors[0].Anchor = buffer.Position{Line: 0, ByteCol: 4}
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 15}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	// Single-line selection populates query with "bbb aaa bbb".
	// Clear query and type "aaa", then enable InSelection.
	// First, clear query via backspace.
	st = d.state()
	for range st.Search.Query {
		d.sendKey(gui.KeyBackspace)
	}
	// Re-set scope manually via Alt+S (need selection in editor).
	// Since the selection was consumed by openFindBar, set it back.
	st = d.state()
	st.Cursors[0].Anchor = buffer.Position{Line: 0, ByteCol: 4}
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 15}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyS, gui.ModAlt)
	for _, r := range "aaa" {
		d.sendChar(r)
	}

	d.tick()
	st = d.state()
	if !st.Search.InSelection {
		t.Fatal("should be in-selection mode")
	}
	if len(st.Search.Matches) != 1 {
		t.Fatalf("want 1 match in selection, got %d",
			len(st.Search.Matches))
	}
}

// ---------- matchesForLine ----------

func TestMatchesForLine(t *testing.T) {
	matches := []buffer.Range{
		{Start: buffer.Position{Line: 0, ByteCol: 0}, End: buffer.Position{Line: 0, ByteCol: 3}},
		{Start: buffer.Position{Line: 2, ByteCol: 0}, End: buffer.Position{Line: 2, ByteCol: 3}},
		{Start: buffer.Position{Line: 2, ByteCol: 5}, End: buffer.Position{Line: 2, ByteCol: 8}},
		{Start: buffer.Position{Line: 5, ByteCol: 0}, End: buffer.Position{Line: 5, ByteCol: 3}},
	}
	got := matchesForLine(matches, 2)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	got = matchesForLine(matches, 1)
	if len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
	got = matchesForLine(matches, 0)
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
}

func TestMatchesForLine_Empty(t *testing.T) {
	got := matchesForLine(nil, 0)
	if got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

// ---------- replaceAllMatches ----------

func TestReplaceAll_SingleUndo(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)

	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	st.Search.Query = "aaa"
	recomputeMatches(st, buf)

	if len(st.Search.Matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(st.Search.Matches))
	}

	st.Search.ReplaceText = "xxx"
	replaceAllMatches(EditorCfg{Buffer: buf}, st, buf)

	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb xxx" {
		t.Fatalf("want 'xxx bbb xxx', got %q", got)
	}

	// Single undo should revert everything.
	r := buf.Undo()
	if !r.OK {
		t.Fatal("undo failed")
	}
	got = buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "aaa bbb aaa" {
		t.Fatalf("after undo want 'aaa bbb aaa', got %q", got)
	}
}

func TestReplaceAll_Regex(t *testing.T) {
	buf := mkBuf("foo123 bar456")
	buf.EnableUndo(nil)

	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	st.Search.Query = `([a-z]+)(\d+)`
	st.Search.IsRegex = true
	st.Search.ReplaceText = "${2}_${1}"
	recomputeMatches(st, buf)

	if len(st.Search.Matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(st.Search.Matches))
	}

	replaceAllMatches(EditorCfg{Buffer: buf}, st, buf)

	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "123_foo 456_bar" {
		t.Fatalf("want '123_foo 456_bar', got %q", got)
	}
}

func TestReplaceNext(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)

	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	st.Search.Query = "aaa"
	st.Search.ReplaceText = "xxx"
	recomputeMatches(st, buf)

	replaceCurrentMatch(EditorCfg{Buffer: buf}, st, buf)

	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb aaa" {
		t.Fatalf("want 'xxx bbb aaa', got %q", got)
	}

	// CurrentMatch should advance to the next match.
	if st.Search.CurrentMatch != 0 {
		t.Fatalf("want currentMatch 0 (next match), got %d",
			st.Search.CurrentMatch)
	}
}

// ---------- driver integration tests ----------

func TestDriver_CtrlFOpensFindBar(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello world"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	st := d.state()
	if !st.Search.Active {
		t.Fatal("find bar should be active")
	}
}

func TestDriver_FindBarTyping(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello world hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	d.sendChar('h')
	d.sendChar('e')
	d.sendChar('l')
	d.sendChar('l')
	d.sendChar('o')

	d.tick()
	st := d.state()
	if st.Search.Query != "hello" {
		t.Fatalf("query = %q, want 'hello'", st.Search.Query)
	}
	if len(st.Search.Matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(st.Search.Matches))
	}
}

func TestDriver_FindNext(t *testing.T) {
	buf := mkBuf("aaa\nbbb\naaa")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)

	for _, r := range "aaa" {
		d.sendChar(r)
	}

	// Enter = find next.
	d.sendKey(gui.KeyEnter)
	st := d.state()
	// Should select the next match (index 1 after advancing from 0).
	cur := st.Cursors[0]
	if !cur.HasSelection() {
		t.Fatal("cursor should have selection")
	}
	sel := cur.SelectionRange()
	// After one Enter, CurrentMatch advanced from 0 to 1.
	if sel.Start.Line != 2 {
		t.Errorf("want match on line 2, got line %d", sel.Start.Line)
	}
}

func TestDriver_FindPrev(t *testing.T) {
	buf := mkBuf("aaa\nbbb\naaa")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)

	for _, r := range "aaa" {
		d.sendChar(r)
	}

	// Shift+Enter = find prev (wraps to last match).
	d.sendKeyMod(gui.KeyEnter, gui.ModShift)
	st := d.state()
	cur := st.Cursors[0]
	sel := cur.SelectionRange()
	if sel.Start.Line != 2 {
		t.Errorf("want match on line 2, got line %d", sel.Start.Line)
	}
}

func TestDriver_EscapeClosesFindBar(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	st := d.state()
	if !st.Search.Active {
		t.Fatal("find bar should be active")
	}

	d.sendKey(gui.KeyEscape)
	st = d.state()
	if st.Search.Active {
		t.Fatal("find bar should be closed")
	}
}

func TestDriver_SelectionPopulatesQuery(t *testing.T) {
	buf := mkBuf("hello world")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	// Select "hello" (pos 0..5).
	st := d.state()
	st.Cursors[0].Anchor = buffer.Position{Line: 0, ByteCol: 0}
	st.Cursors[0].Cursor = buffer.Position{Line: 0, ByteCol: 5}
	storeState(d.w, d.cfg.IDFocus, st)

	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	st = d.state()
	if st.Search.Query != "hello" {
		t.Fatalf("query = %q, want 'hello'", st.Search.Query)
	}
}

func TestDriver_ReplaceAll(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	// Open replace bar.
	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	st := d.state()
	if !st.Search.Active || !st.Search.ShowReplace {
		t.Fatal("replace bar should be open")
	}

	// Type search query.
	for _, r := range "aaa" {
		d.sendChar(r)
	}

	// Tab to replace field.
	d.sendKey(gui.KeyTab)

	// Type replacement.
	for _, r := range "xxx" {
		d.sendChar(r)
	}

	// Ctrl+Enter = replace all.
	d.sendKeyMod(gui.KeyEnter, gui.ModCtrl)

	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb xxx" {
		t.Fatalf("want 'xxx bbb xxx', got %q", got)
	}

	// Single undo reverts.
	r := buf.Undo()
	if !r.OK {
		t.Fatal("undo failed")
	}
	got = buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "aaa bbb aaa" {
		t.Fatalf("after undo: %q", got)
	}
}

func TestDriver_ToggleCase(t *testing.T) {
	buf := mkBuf("Hello hello")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)

	for _, r := range "hello" {
		d.sendChar(r)
	}

	d.tick()
	st := d.state()
	// Default: case insensitive → 2 matches.
	if len(st.Search.Matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(st.Search.Matches))
	}

	// Alt+C → toggle case sensitive.
	d.sendKeyMod(gui.KeyC, gui.ModAlt)
	d.tick()
	st = d.state()
	if !st.Search.CaseSensitive {
		t.Fatal("should be case sensitive")
	}
	if len(st.Search.Matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(st.Search.Matches))
	}
}

func TestDriver_ToggleRegex(t *testing.T) {
	buf := mkBuf("foo123 bar456")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)

	for _, r := range `\d+` {
		d.sendChar(r)
	}

	d.tick()
	st := d.state()
	// Literal mode: no matches for "\d+".
	if len(st.Search.Matches) != 0 {
		t.Fatalf("literal mode: want 0 matches, got %d",
			len(st.Search.Matches))
	}

	// Alt+R → toggle regex.
	d.sendKeyMod(gui.KeyR, gui.ModAlt)
	d.tick()
	st = d.state()
	if !st.Search.IsRegex {
		t.Fatal("should be regex mode")
	}
	if len(st.Search.Matches) != 2 {
		t.Fatalf("regex mode: want 2 matches, got %d",
			len(st.Search.Matches))
	}
}

func TestDriver_ReadOnlyBlocksReplace(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1, ReadOnly: true})

	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	for _, r := range "aaa" {
		d.sendChar(r)
	}
	d.sendKey(gui.KeyTab)
	for _, r := range "xxx" {
		d.sendChar(r)
	}

	d.sendKeyMod(gui.KeyEnter, gui.ModCtrl)

	// Buffer unchanged.
	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "aaa bbb aaa" {
		t.Fatalf("buffer should be unchanged, got %q", got)
	}
}

func TestDriver_FindBarBackspace(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	d.sendChar('a')
	d.sendChar('b')
	d.sendKey(gui.KeyBackspace)
	st := d.state()
	if st.Search.Query != "a" {
		t.Fatalf("query = %q, want 'a'", st.Search.Query)
	}
}

// ---------- hardening ----------

func TestSpliceField_OutOfBounds(t *testing.T) {
	// Negative lo.
	got := spliceField("abc", -1, 1, "X")
	if got != "Xbc" {
		t.Errorf("negative lo: %q", got)
	}
	// hi > len.
	got = spliceField("abc", 1, 100, "X")
	if got != "aX" {
		t.Errorf("hi > len: %q", got)
	}
	// lo > len.
	got = spliceField("abc", 100, 200, "X")
	if got != "abcX" {
		t.Errorf("lo > len: %q", got)
	}
	// Empty string.
	got = spliceField("", 0, 0, "X")
	if got != "X" {
		t.Errorf("empty: %q", got)
	}
}

func TestFieldCursorClamp(t *testing.T) {
	ss := &searchState{Query: "abc", FieldCursor: 100}
	ss.clampFieldCursor()
	if ss.FieldCursor != 3 {
		t.Errorf("want 3, got %d", ss.FieldCursor)
	}
	ss.FieldCursor = -5
	ss.clampFieldCursor()
	if ss.FieldCursor != 0 {
		t.Errorf("want 0, got %d", ss.FieldCursor)
	}
}

func TestMatchesForLine_NegativeLine(t *testing.T) {
	matches := []buffer.Range{
		{Start: buffer.Position{Line: 0}, End: buffer.Position{Line: 0, ByteCol: 3}},
	}
	got := matchesForLine(matches, -1)
	if len(got) != 0 {
		t.Fatalf("negative line: want 0, got %d", len(got))
	}
}

func TestFindAllMatches_NilBuffer(t *testing.T) {
	// Nil buffer should not panic — Buffer.New() substitutes in
	// Editor, but test the search path defensively.
	buf := buffer.New()
	matches, _ := findAllMatches(buf, "x", true, false, buffer.Range{})
	if len(matches) != 0 {
		t.Fatalf("empty buf: want 0, got %d", len(matches))
	}
}

func TestHandleSearchChar_MaxFieldLen(t *testing.T) {
	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	// Fill query to maxFieldLen.
	st.Search.Query = strings.Repeat("a", maxFieldLen)
	st.Search.FieldCursor = maxFieldLen
	buf := mkBuf("hello")
	handleSearchChar(st, buf, 'x')
	if len(st.Search.Query) != maxFieldLen {
		t.Fatalf("should cap at maxFieldLen, got %d",
			len(st.Search.Query))
	}
}

// ---------- toLowerReuse ----------

func TestToLowerReuse_ASCII(t *testing.T) {
	var buf []byte
	got := toLowerReuse([]byte("Hello WORLD"), &buf)
	if string(got) != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestToLowerReuse_NonASCII(t *testing.T) {
	var buf []byte
	got := toLowerReuse([]byte("Héllo"), &buf)
	if string(got) != "héllo" {
		t.Fatalf("got %q", got)
	}
}

func TestToLowerReuse_Empty(t *testing.T) {
	var buf []byte
	got := toLowerReuse(nil, &buf)
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestToLowerReuse_ReusesBuf(t *testing.T) {
	var buf []byte
	toLowerReuse([]byte("AAA"), &buf)
	if cap(buf) < 3 {
		t.Fatal("should have allocated")
	}
	got := toLowerReuse([]byte("BB"), &buf)
	if string(got) != "bb" {
		t.Fatalf("got %q", got)
	}
}

// ---------- navigateMatch ----------

func TestNavigateMatch_WrapForward(t *testing.T) {
	st := &editorState{}
	st.ensureCursors()
	st.Search.Matches = []buffer.Range{
		{Start: buffer.Position{Line: 0}, End: buffer.Position{Line: 0, ByteCol: 1}},
		{Start: buffer.Position{Line: 1}, End: buffer.Position{Line: 1, ByteCol: 1}},
	}
	st.Search.CurrentMatch = 1
	navigateMatch(st, +1)
	if st.Search.CurrentMatch != 0 {
		t.Fatalf("want 0 (wrapped), got %d", st.Search.CurrentMatch)
	}
}

func TestNavigateMatch_WrapBackward(t *testing.T) {
	st := &editorState{}
	st.ensureCursors()
	st.Search.Matches = []buffer.Range{
		{Start: buffer.Position{Line: 0}, End: buffer.Position{Line: 0, ByteCol: 1}},
		{Start: buffer.Position{Line: 1}, End: buffer.Position{Line: 1, ByteCol: 1}},
	}
	st.Search.CurrentMatch = 0
	navigateMatch(st, -1)
	if st.Search.CurrentMatch != 1 {
		t.Fatalf("want 1 (wrapped), got %d", st.Search.CurrentMatch)
	}
}

func TestNavigateMatch_EmptyMatches(t *testing.T) {
	st := &editorState{}
	st.ensureCursors()
	st.Search.CurrentMatch = 0
	navigateMatch(st, +1) // should not panic
}

// ---------- replaceBytes ----------

func TestReplaceBytes_Literal(t *testing.T) {
	buf := mkBuf("hello world")
	ss := &searchState{ReplaceText: "XXX"}
	m := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 0},
		End:   buffer.Position{Line: 0, ByteCol: 5},
	}
	got := replaceBytes(ss, buf, m)
	if string(got) != "XXX" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceBytes_RegexNilCompiled(t *testing.T) {
	buf := mkBuf("hello")
	ss := &searchState{IsRegex: true, compiled: nil, ReplaceText: "X"}
	m := buffer.Range{
		Start: buffer.Position{Line: 0, ByteCol: 0},
		End:   buffer.Position{Line: 0, ByteCol: 5},
	}
	got := replaceBytes(ss, buf, m)
	if string(got) != "X" {
		t.Fatalf("got %q", got)
	}
}

// ---------- filterToScope ----------

func TestFilterToScope_AllInScope(t *testing.T) {
	matches := []buffer.Range{
		{Start: buffer.Position{Line: 1, ByteCol: 0}, End: buffer.Position{Line: 1, ByteCol: 3}},
		{Start: buffer.Position{Line: 2, ByteCol: 0}, End: buffer.Position{Line: 2, ByteCol: 3}},
	}
	scope := buffer.Range{
		Start: buffer.Position{Line: 0},
		End:   buffer.Position{Line: 5},
	}
	got := filterToScope(matches, scope)
	// Should return original slice (no allocation).
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if &got[0] != &matches[0] {
		t.Fatal("should return original slice, not copy")
	}
}

// ---------- needsRecompute ----------

func TestNeedsRecompute_DirtyFlag(t *testing.T) {
	ss := &searchState{Query: "a", lastQuery: "a", lastFlags: 0}
	if needsRecompute(ss) {
		t.Fatal("should not need recompute")
	}
	ss.matchesDirty = true
	if !needsRecompute(ss) {
		t.Fatal("dirty flag should trigger recompute")
	}
}

func TestNeedsRecompute_QueryChanged(t *testing.T) {
	ss := &searchState{Query: "b", lastQuery: "a"}
	if !needsRecompute(ss) {
		t.Fatal("query change should trigger recompute")
	}
}

// ---------- replaceCurrentMatch edge cases ----------

func TestReplaceCurrentMatch_NoMatches(t *testing.T) {
	buf := mkBuf("hello")
	buf.EnableUndo(nil)
	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	st.Search.Query = "zzz"
	st.Search.CurrentMatch = -1
	// Should not panic.
	replaceCurrentMatch(EditorCfg{Buffer: buf}, st, buf)
	if buf.Line(0) != nil && string(buf.Line(0)) != "hello" {
		t.Fatal("buffer should be unchanged")
	}
}

// ---------- replaceAll edge cases ----------

func TestReplaceAll_EmptyReplacement(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)
	st := &editorState{}
	st.ensureCursors()
	st.Search.Active = true
	st.Search.Query = "aaa"
	st.Search.ReplaceText = ""
	recomputeMatches(st, buf)

	replaceAllMatches(EditorCfg{Buffer: buf}, st, buf)
	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != " bbb " {
		t.Fatalf("want ' bbb ', got %q", got)
	}
}

// ---------- driver: delete, cursor movement, keybindings ----------

func TestDriver_FindBarDelete(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	d.sendChar('a')
	d.sendChar('b')
	// Move cursor left, then delete forward.
	d.sendKey(gui.KeyLeft)
	d.sendKey(gui.KeyDelete)
	st := d.state()
	if st.Search.Query != "a" {
		t.Fatalf("query = %q, want 'a'", st.Search.Query)
	}
}

func TestDriver_FindBarCursorMovement(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	d.sendChar('a')
	d.sendChar('b')
	d.sendChar('c')

	// Home → cursor at 0.
	d.sendKey(gui.KeyHome)
	st := d.state()
	if st.Search.FieldCursor != 0 {
		t.Fatalf("Home: cursor = %d, want 0", st.Search.FieldCursor)
	}

	// End → cursor at len.
	d.sendKey(gui.KeyEnd)
	st = d.state()
	if st.Search.FieldCursor != 3 {
		t.Fatalf("End: cursor = %d, want 3", st.Search.FieldCursor)
	}

	// Left → 2.
	d.sendKey(gui.KeyLeft)
	st = d.state()
	if st.Search.FieldCursor != 2 {
		t.Fatalf("Left: cursor = %d, want 2", st.Search.FieldCursor)
	}

	// Right → 3.
	d.sendKey(gui.KeyRight)
	st = d.state()
	if st.Search.FieldCursor != 3 {
		t.Fatalf("Right: cursor = %d, want 3", st.Search.FieldCursor)
	}
}

func TestDriver_CtrlRReplacesNext(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	for _, r := range "aaa" {
		d.sendChar(r)
	}
	d.sendKey(gui.KeyTab)
	for _, r := range "xxx" {
		d.sendChar(r)
	}

	// Ctrl+R = replace next.
	d.sendKeyMod(gui.KeyR, gui.ModCtrl)

	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb aaa" {
		t.Fatalf("want 'xxx bbb aaa', got %q", got)
	}
}

func TestDriver_CtrlHTogglesReplace(t *testing.T) {
	d := newDriver(EditorCfg{Buffer: mkBuf("hello"), IDFocus: 1})
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	st := d.state()
	if st.Search.ShowReplace {
		t.Fatal("replace should be hidden initially")
	}

	// Ctrl+H while find bar open → show replace.
	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	st = d.state()
	if !st.Search.ShowReplace {
		t.Fatal("replace should be visible")
	}

	// Ctrl+H again → hide replace.
	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	st = d.state()
	if st.Search.ShowReplace {
		t.Fatal("replace should be hidden")
	}
}

// ---------- regex edge cases ----------

func TestFindAllMatches_RegexZeroLengthMatch(t *testing.T) {
	buf := mkBuf("abc")
	matches, _ := findAllMatches(buf, "\\b", true, true, buffer.Range{})
	// \b matches at word boundaries: before 'a' and after 'c'.
	// Exact count depends on engine, but should not exceed maxMatches
	// and should not hang.
	if len(matches) > maxMatches {
		t.Fatalf("should be capped, got %d", len(matches))
	}
}

// ---------- undo round-trip regression ----------

func TestDriver_ReplaceAll_UndoThenReplaceAgain(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	buf.EnableUndo(nil)
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	d.sendKeyMod(gui.KeyH, gui.ModCtrl)
	for _, r := range "aaa" {
		d.sendChar(r)
	}
	d.sendKey(gui.KeyTab)
	for _, r := range "xxx" {
		d.sendChar(r)
	}

	// Replace all.
	d.sendKeyMod(gui.KeyEnter, gui.ModCtrl)
	got := buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb xxx" {
		t.Fatalf("first replace: %q", got)
	}

	// Undo.
	d.sendKeyMod(gui.KeyZ, gui.ModCtrl)
	got = buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "aaa bbb aaa" {
		t.Fatalf("after undo: %q", got)
	}

	// Replace all again — this is the regression case.
	d.sendKeyMod(gui.KeyEnter, gui.ModCtrl)
	got = buf.TextInRange(buffer.Range{
		Start: buffer.Position{},
		End:   buffer.Position{Line: 0, ByteCol: len(buf.Line(0))},
	})
	if got != "xxx bbb xxx" {
		t.Fatalf("second replace after undo: %q", got)
	}
}

// ---------- multi-cursor + find bar ----------

func TestDriver_FindBarWithMultiCursor(t *testing.T) {
	buf := mkBuf("aaa bbb aaa")
	d := newDriver(EditorCfg{Buffer: buf, IDFocus: 1})

	// Add a second cursor.
	d.addCursorAt(0, 4)
	st := d.state()
	if len(st.Cursors) < 2 {
		t.Fatal("need multi-cursor")
	}

	// Open find bar — should not panic.
	d.sendKeyMod(gui.KeyF, gui.ModCtrl)
	st = d.state()
	if !st.Search.Active {
		t.Fatal("find bar should be active")
	}

	// Navigate — should collapse to primary.
	for _, r := range "aaa" {
		d.sendChar(r)
	}
	d.sendKey(gui.KeyEnter)
	st = d.state()
	if len(st.Cursors) != 1 {
		t.Fatalf("should collapse to 1 cursor, got %d",
			len(st.Cursors))
	}
}

// ---------- openFindBar ----------

func TestOpenFindBar_LongSelectionCapped(t *testing.T) {
	long := strings.Repeat("x", maxFieldLen+100)
	buf := mkBuf(long)
	st := &editorState{}
	st.ensureCursors()
	st.Cursors[0].Anchor = buffer.Position{Line: 0, ByteCol: 0}
	st.Cursors[0].Cursor = buffer.Position{
		Line: 0, ByteCol: len(buf.Line(0)),
	}
	openFindBar(st, buf, false)
	if len(st.Search.Query) > maxFieldLen {
		t.Fatalf("query len %d exceeds maxFieldLen",
			len(st.Search.Query))
	}
}

// ---------- helpers ----------

// newReader wraps bytes for buffer.Load.
func newReader(data []byte) *readerHelper { return &readerHelper{data: data} }

type readerHelper struct {
	data []byte
	off  int
}

func (r *readerHelper) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

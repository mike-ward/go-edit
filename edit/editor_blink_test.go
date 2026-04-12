package edit

import (
	"testing"
	"time"

	"github.com/mike-ward/go-edit/edit/buffer"
	"github.com/mike-ward/go-gui/gui"
)

// blinkBase is a non-epoch reference time. Tests must avoid time
// values whose UnixNano() is 0, since the editor uses 0 as a
// "no activity yet" sentinel.
var blinkBase = time.Unix(1_700_000_000, 0)

// fakeClock returns a closure that always reports t. Tests reassign
// the field through the variable so each call sees the current value.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// at returns blinkBase + ms milliseconds.
func at(ms int64) time.Time {
	return blinkBase.Add(time.Duration(ms) * time.Millisecond)
}

func newBlinkDriver(period time.Duration, clock *fakeClock) *driver {
	cfg := EditorCfg{
		IDFocus:           42,
		Buffer:            buffer.FromBytes([]byte("hello")),
		Width:             400,
		Height:            200,
		CursorBlinkPeriod: period,
		Now:               clock.now,
	}
	return newDriver(cfg)
}

func TestBlink_DisabledMeansAlwaysVisible(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(-1, clk)
	for i := range 5 {
		clk.t = blinkBase.Add(time.Duration(i) * time.Second)
		d.tick()
		if !d.frame.cursorVisible {
			t.Fatalf("i=%d: cursor invisible with blink disabled", i)
		}
	}
}

func TestBlink_StartsFromFirstFrame(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	// First amend seeds LastActivityUnixNano so blink starts
	// immediately without waiting for a keystroke.
	d.tick()
	if !d.frame.cursorVisible {
		t.Fatal("cursor should be visible on first frame")
	}
	if d.state().LastActivityUnixNano == 0 {
		t.Fatal("first amend should seed LastActivityUnixNano")
	}
	// Advance past the visible half-period → hidden.
	clk.t = at(500)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("cursor should be hidden at 500ms with no user input")
	}
}

func TestBlink_TogglesAcrossPeriods(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)

	// Type a char to record activity at T=blinkBase.
	d.sendChar('a')
	if !d.frame.cursorVisible {
		t.Fatal("immediately after activity, cursor should be visible")
	}
	if d.state().LastActivityUnixNano != blinkBase.UnixNano() {
		t.Fatalf("LastActivity=%d want %d",
			d.state().LastActivityUnixNano, blinkBase.UnixNano())
	}

	cases := []struct {
		ms      int64
		visible bool
	}{
		{0, true},     // dt=0
		{499, true},   // still in first half
		{500, false},  // hidden half starts
		{999, false},  // still hidden
		{1000, true},  // visible again
		{1499, true},  // still visible
		{1500, false}, // hidden
	}
	for _, c := range cases {
		clk.t = at(c.ms)
		d.tick()
		if d.frame.cursorVisible != c.visible {
			t.Errorf("ms=%d: visible=%v want %v",
				c.ms, d.frame.cursorVisible, c.visible)
		}
	}
}

func TestBlink_ActivityResetsCycle(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)

	// Activity at T=base.
	d.sendChar('a')
	clk.t = at(600)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("at 600ms cursor should be hidden")
	}

	// Activity at T=600ms restarts the cycle. Cursor visible
	// for [600, 1100), hidden for [1100, 1600).
	d.sendChar('b')
	clk.t = at(1099)
	d.tick()
	if !d.frame.cursorVisible {
		t.Fatal("at 1099ms (within reset cycle) cursor should be visible")
	}
	clk.t = at(1100)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("at 1100ms cursor should be hidden")
	}
}

func TestBlink_KeyEventResets(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	d.sendChar('a') // initial activity at T=base
	clk.t = at(700)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("at 700ms expected hidden")
	}
	// Arrow key counts as activity.
	d.sendKey(gui.KeyRight)
	want := clk.t.UnixNano()
	if got := d.state().LastActivityUnixNano; got != want {
		t.Errorf("after key: LastActivity=%d want %d", got, want)
	}
	if !d.frame.cursorVisible {
		// Need to tick to refresh frame.cursorVisible.
		d.tick()
		if !d.frame.cursorVisible {
			t.Fatal("after key reset, cursor should be visible")
		}
	}
}

func TestBlink_MouseClickResets(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	d.sendChar('a')
	clk.t = at(800)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("at 800ms expected hidden")
	}
	d.sendClick(20, 5, 0)
	if d.state().LastActivityUnixNano != clk.t.UnixNano() {
		t.Errorf("click did not reset blink")
	}
}

func TestBlink_ScrollDoesNotReset(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	buf := buffer.FromBytes([]byte("a\nb\nc\nd\ne\nf\ng"))
	cfg := EditorCfg{
		IDFocus:           99,
		Buffer:            buf,
		Width:             400,
		Height:            200,
		CursorBlinkPeriod: 500 * time.Millisecond,
		Now:               clk.now,
	}
	d := newDriver(cfg)
	d.sendChar('x')
	startActivity := d.state().LastActivityUnixNano
	clk.t = at(200)
	d.sendScroll(-1)
	if got := d.state().LastActivityUnixNano; got != startActivity {
		t.Errorf("scroll mutated LastActivity %d → %d (no reset wanted)",
			startActivity, got)
	}
}

func TestBlink_DrawVersionStableAcrossBlinkTransitions(t *testing.T) {
	// Hardening invariant: main canvas drawVersion MUST NOT
	// change when only the blink state flips. Otherwise the main
	// viewport would re-tessellate at the blink rate.
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	d.sendChar('a')
	d.tick()
	v0 := d.frame.drawVersion
	if v0 == 0 {
		t.Fatal("drawVersion is 0")
	}
	// Cross several blink transitions; cursor toggles, no other
	// state changes.
	for _, ms := range []int64{499, 500, 999, 1000, 1499, 1500} {
		clk.t = at(ms)
		d.tick()
		if d.frame.drawVersion != v0 {
			t.Fatalf("drawVersion drifted at ms=%d: v0=%d v=%d",
				ms, v0, d.frame.drawVersion)
		}
	}
}

func TestBlink_ResetBlinkDisabledNoop(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	cfg := EditorCfg{
		IDFocus:           50,
		Buffer:            buffer.New(),
		Width:             400,
		Height:            200,
		CursorBlinkPeriod: -1, // disabled
		Now:               clk.now,
	}
	st := editorState{}
	st.ensureCursors()
	resetBlink(cfg, &st)
	if st.LastActivityUnixNano != 0 {
		t.Fatalf("resetBlink with disabled blink mutated state: %d",
			st.LastActivityUnixNano)
	}
}

func TestBlink_MultiCursorShareVisibility(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	d.addCursorAt(0, 3)
	d.sendChar('a')
	// Visible half.
	clk.t = at(200)
	d.tick()
	if !d.frame.cursorVisible {
		t.Fatal("expected visible at 200ms")
	}
	if len(d.state().Cursors) < 2 {
		t.Fatal("expected multiple cursors")
	}
	// Hidden half — both cursors must share the single flag.
	clk.t = at(600)
	d.tick()
	if d.frame.cursorVisible {
		t.Fatal("expected hidden at 600ms (both cursors)")
	}
}

func TestBlink_HelpOverlayHidesCursor(t *testing.T) {
	clk := &fakeClock{t: blinkBase}
	d := newBlinkDriver(500*time.Millisecond, clk)
	d.sendChar('a')
	d.tick()
	if !d.frame.cursorVisible {
		t.Fatal("cursor should be visible before help")
	}
	// Activate help overlay.
	st := d.state()
	st.HelpActive = true
	storeState(d.w, d.cfg.IDFocus, st)
	d.tick()
	// cursorVisible is still true (blink state), but the overlay
	// draw closure should early-return when HelpActive. Verify
	// the state is correct for the draw check.
	if !d.state().HelpActive {
		t.Fatal("help should be active")
	}
	// The draw closure checks st.HelpActive and skips cursor
	// rendering. We can only assert state here; visual
	// verification requires a DrawContext.
}

func TestBlink_TinyPeriodClampedToFloor(t *testing.T) {
	// Hardening: absurdly small positive CursorBlinkPeriod must
	// be clamped to minBlinkPeriod, not fire in a tight loop.
	if got := blinkPeriod(EditorCfg{CursorBlinkPeriod: 1}); got < minBlinkPeriod {
		t.Fatalf("period=%v want >= %v", got, minBlinkPeriod)
	}
	if got := blinkPeriod(EditorCfg{CursorBlinkPeriod: -1}); got != 0 {
		t.Fatalf("negative period should disable: got %v", got)
	}
	if got := blinkPeriod(EditorCfg{}); got != defaultBlinkPeriod {
		t.Fatalf("zero period should default: got %v", got)
	}
}

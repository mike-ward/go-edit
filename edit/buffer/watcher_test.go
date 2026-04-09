package buffer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherModified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.txt")
	os.WriteFile(path, []byte("v1"), 0o644) //nolint:errcheck

	info, _ := os.Stat(path)
	clock := info.ModTime()
	w := NewWatcher(func() time.Time { return clock })
	w.Watch(path, info.ModTime())

	// No change yet — advance clock past throttle.
	clock = clock.Add(2 * time.Second)
	if events := w.Check(); len(events) != 0 {
		t.Fatalf("unexpected events: %v", events)
	}

	// Modify file.
	time.Sleep(10 * time.Millisecond)       // ensure modtime differs
	os.WriteFile(path, []byte("v2"), 0o644) //nolint:errcheck

	clock = clock.Add(2 * time.Second)
	events := w.Check()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Kind != WatchModified {
		t.Errorf("kind = %d, want WatchModified", events[0].Kind)
	}

	// Second check without further modification → no event.
	clock = clock.Add(2 * time.Second)
	if events := w.Check(); len(events) != 0 {
		t.Errorf("spurious events: %v", events)
	}
}

func TestWatcherDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.txt")
	os.WriteFile(path, []byte("x"), 0o644) //nolint:errcheck
	info, _ := os.Stat(path)

	clock := info.ModTime()
	w := NewWatcher(func() time.Time { return clock })
	w.Watch(path, info.ModTime())

	os.Remove(path) //nolint:errcheck

	clock = clock.Add(2 * time.Second)
	events := w.Check()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Kind != WatchDeleted {
		t.Errorf("kind = %d, want WatchDeleted", events[0].Kind)
	}
}

func TestWatcherThrottle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.txt")
	os.WriteFile(path, []byte("x"), 0o644) //nolint:errcheck
	info, _ := os.Stat(path)

	clock := info.ModTime()
	w := NewWatcher(func() time.Time { return clock })
	w.Watch(path, info.ModTime())

	// First check establishes lastCheck.
	clock = clock.Add(2 * time.Second)
	w.Check()

	// Modify file.
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("y"), 0o644) //nolint:errcheck

	// Check without advancing clock past interval → throttled.
	if events := w.Check(); len(events) != 0 {
		t.Fatalf("expected throttle, got %d events", len(events))
	}
}

func TestWatcherUnwatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.txt")
	os.WriteFile(path, []byte("x"), 0o644) //nolint:errcheck
	info, _ := os.Stat(path)

	clock := info.ModTime()
	w := NewWatcher(func() time.Time { return clock })
	w.Watch(path, info.ModTime())
	w.Unwatch(path)

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("y"), 0o644) //nolint:errcheck

	clock = clock.Add(2 * time.Second)
	if events := w.Check(); len(events) != 0 {
		t.Fatalf("expected no events after unwatch, got %d", len(events))
	}
}

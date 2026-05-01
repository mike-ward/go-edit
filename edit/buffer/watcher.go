package buffer

import (
	"os"
	"time"
)

// WatchKind classifies a file-change event.
type WatchKind byte

// WatchKind event types.
const (
	WatchModified WatchKind = iota // file content changed
	WatchDeleted                   // file removed
)

// WatchEvent reports a detected external change.
type WatchEvent struct {
	Path string
	Kind WatchKind
}

type watchEntry struct {
	modTime time.Time
	size    int64
}

// watchInterval is the minimum time between successive Check calls
// that actually stat the filesystem.
const watchInterval = 1 * time.Second

// Watcher polls watched file paths for external modifications.
// Injectable now func for deterministic tests.
type Watcher struct {
	entries   map[string]watchEntry
	now       func() time.Time
	lastCheck time.Time
}

// NewWatcher creates a Watcher. now is the clock source; pass
// time.Now for production, a fake for tests.
func NewWatcher(now func() time.Time) *Watcher {
	if now == nil {
		now = time.Now
	}
	return &Watcher{
		entries: make(map[string]watchEntry),
		now:     now,
	}
}

// Watch registers path for change detection. modTime is the
// known-good modification time (typically from LoadFile).
func (w *Watcher) Watch(path string, modTime time.Time) {
	if path == "" {
		return
	}
	entry := watchEntry{modTime: modTime, size: -1}
	if info, err := os.Stat(path); err == nil {
		entry.size = info.Size()
	}
	w.entries[path] = entry
}

// Unwatch removes path from change detection.
func (w *Watcher) Unwatch(path string) {
	delete(w.entries, path)
}

// Check stats all watched paths and returns events for any that
// changed or were deleted since the last known modTime. Throttled
// to at most one filesystem scan per watchInterval.
func (w *Watcher) Check() []WatchEvent {
	now := w.now()
	if now.Sub(w.lastCheck) < watchInterval {
		return nil
	}
	w.lastCheck = now

	var events []WatchEvent
	for path, entry := range w.entries {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				events = append(events, WatchEvent{
					Path: path, Kind: WatchDeleted,
				})
				// Emit delete once; callers should re-register if recreated.
				delete(w.entries, path)
			}
			continue
		}
		if info.ModTime() != entry.modTime || entry.size != info.Size() {
			events = append(events, WatchEvent{
				Path: path, Kind: WatchModified,
			})
			// Update so the same change isn't reported twice.
			w.entries[path] = watchEntry{
				modTime: info.ModTime(),
				size:    info.Size(),
			}
		}
	}
	return events
}

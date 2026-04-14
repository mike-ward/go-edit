package main

// closeDecision describes what OnCloseRequest should do for a given
// appState. Pure function of state; no side effects.
type closeDecision int

const (
	// closeIgnore means a dialog is already open; drop the event.
	closeIgnore closeDecision = iota
	// closeNow means buffer is clean; close immediately.
	closeNow
	// closePrompt means buffer is dirty; open save/discard dialog.
	closePrompt
)

// decideClose returns what OnCloseRequest should do next.
func decideClose(s *appState) closeDecision {
	if s == nil || s.Buf == nil {
		return closeNow
	}
	if s.closing {
		return closeIgnore
	}
	if !s.Buf.Dirty() {
		return closeNow
	}
	return closePrompt
}

// shouldFinishClose reports whether the window should close after a
// save attempt. False means save failed (buffer still dirty) and the
// window should stay open for retry.
func shouldFinishClose(s *appState) bool {
	if s == nil || s.Buf == nil {
		return true
	}
	return !s.Buf.Dirty()
}

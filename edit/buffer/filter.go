package buffer

// FilterResult tells Apply what to do after a filter runs.
type FilterResult int

const (
	// FilterAccept proceeds with the (possibly modified) edit.
	FilterAccept FilterResult = iota
	// FilterReject vetoes the edit. Apply returns a zero Change
	// and the dirty flag is unchanged.
	FilterReject
)

// EditFilter observes, transforms, or vetoes an edit before it
// mutates the buffer. The filter receives a pointer to the Edit
// so it can modify Range or NewBytes in place. Coordinates are
// already clamped.
//
// Filters run in registration order. A FilterReject from any
// filter stops the chain.
type EditFilter func(b *Buffer, e *Edit) FilterResult

// PostEditFunc is called after a successful Apply with the
// resulting Change.
type PostEditFunc func(Change)

// AddFilter appends a filter to the chain. Returns a remove func.
// The remove func nils the slot to avoid slice shuffling; Apply
// skips nil entries.
func (b *Buffer) AddFilter(f EditFilter) func() {
	if f == nil {
		return func() {}
	}
	b.filters = append(b.filters, f)
	idx := len(b.filters) - 1
	removed := false
	return func() {
		if !removed && idx < len(b.filters) {
			b.filters[idx] = nil
			removed = true
		}
	}
}

// OnEdit registers a post-edit observer. Returns a remove func.
func (b *Buffer) OnEdit(fn PostEditFunc) func() {
	if fn == nil {
		return func() {}
	}
	b.postEdit = append(b.postEdit, fn)
	idx := len(b.postEdit) - 1
	removed := false
	return func() {
		if !removed && idx < len(b.postEdit) {
			b.postEdit[idx] = nil
			removed = true
		}
	}
}

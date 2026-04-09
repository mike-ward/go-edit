# go-edit ROADMAP

Code editor widget for [go-gui](https://github.com/mike-ward/go-gui). Pure Go,
no CGO. Syntax highlighting via [chroma](https://github.com/alecthomas/chroma).

## Design tenets

- Pure Go. No CGO. No tree-sitter.
- Depends on go-gui public API. Missing capability → push upstream.
- May depend directly on go-glyph `v1.6.4` (the version go-gui pins)
  when go-gui's text path is too coarse — shaping, metrics, glyph-run
  batching, fallback chains. Keep the surface narrow and isolated behind
  an internal text package so swapping stays cheap.
- Heap-allocation conscious (per go-gui conventions).
- Headless-testable. No backend required for unit tests.
- Immediate-mode friendly: editor state is a struct on the window State, not
  a global.

## Architecture sketch

```
Buffer (per-line gap buffer) ── Document model, undo/redo, edits
   │
   ├── Cursors []Cursor          Multi-cursor + selections
   ├── Highlighter (chroma)      Lazy, line-range cached tokens
   ├── Folding                   Range tree
   └── View                      Scroll, viewport, gutter, decorations
        │
        └── go-gui Layout        Renders via Shape primitives
```

Key types (proposed):

- `Buffer` — text storage; line index; edit ops return `Change` for undo.
- `Cursor{ Pos, Anchor, ID }` — selection = `Pos != Anchor`.
- `Editor` — top-level widget state; held in window State or namespaced
  StateMap slot.
- `EditorCfg` — zero-init config (Theme, Language, ReadOnly, TabWidth,
  ShowLineNumbers, ShowMinimap, IDFocus, callbacks).
- `Editor(cfg EditorCfg) gui.View` — factory matching go-gui conventions
  (value config, returns `gui.View` interface).

## Phases

### Phase -1 — Decisions (locked)

- Cursor position: `struct{ Line, ByteCol int }`. Grapheme for movement,
  rune never.
- Edit type: single `Edit{ Range, NewBytes []byte }`. No insert/delete
  variants.
- Naming: `Edit` = applied mutation, `Change` = undo record.
- Canonical coordinate space: logical (line, col). Visual (row, x) derived
  per frame for soft wrap / folds.
- Text rendering: direct go-glyph dependency, wrapped in internal
  `edit/text` package. Single choke point.
- Drawing surface: single canvas widget, not column of per-line widgets.
- Min Go version: match go-gui `go.mod`.
- License: same as go-gui.
- Injectable time source (blink, undo coalesce, autosave). Fake clock from
  day one.
- Panic policy: editor never panics on user input; `Buffer` may panic only
  on internal invariant break.
- Scope fence: no file tree, tabs, command palette, settings UI. App
  concerns, not editor.
- No `io/fs` abstraction until a second consumer asks. `os` only.

Still open at this layer:
- [ ] Gutter+text one canvas or two sharing scroll Y.

### Phase 0 — Skeleton  ☑

- [x] `go mod init github.com/mike-ward/go-edit`; replace directive to local
      go-gui / go-glyph.
- [x] Package layout: `edit/`, `edit/buffer/`, `edit/highlight/`,
      `edit/text/`, `edit/internal/`, `examples/basic/`.
- [x] CI: `go test ./...`, `go vet`, `golangci-lint` (config copied from
      go-gui).
- [x] License: PolyForm Noncommercial 1.0.0 (match go-gui).
- [x] `go.mod`: `go 1.26.0` (match go-gui).

Moved to Phase 1 (need a Buffer to exist first):

- Headless test harness mirroring go-gui's `backend/test`.
- Golden-frame harness: snapshot draw call list, not pixels.
- Property test: random edit sequences, assert Buffer invariants.
- Fuzz `Buffer.Apply` with arbitrary bytes incl. invalid UTF-8 + NULs.
- Bench baseline file checked in (100k-line generated Go) for Phase 3.

### Phase 1 — Buffer + minimal view  ☑ (save deferred)

- [x] `Buffer` as per-line byte-slice store (gap buffer deferred until
      bench pressure — baseline recorded in `buffer_bench_test.go`).
- [x] Line index; UTF-8 aware. Grapheme-cluster movement deferred to
      Phase 2; Phase 1 moves by byte.
- [x] Internal `edit/text` package wrapping `gui.TextMeasurer` (not
      go-glyph directly — the measurer already wraps it, and OnDraw has
      no window access, so the indirection is cheaper).
- [x] `Editor()` factory rendering monospace text.
- [x] Single cursor, arrow keys, Home/End, PgUp/PgDn.
- [x] Insert/delete/Enter/Backspace.
- [x] Line-number gutter (toggleable).
- [x] Example: `examples/basic` — open file, edit (no save).
- [x] Headless driver tests via `edit/internal/fakewin`.
- [x] Buffer benchmarks (`BenchmarkFromBytes100k`, `BenchmarkLoad100k`,
      `BenchmarkLineIter100k`, `BenchmarkRandomEdits10k`).
- [x] File save — completed in Phase 1.2.

Architectural deviations from initial plan:

- Editor does NOT use go-gui's `Column(IDScroll)` virtualization.
  DrawCanvas caches its full draw output keyed by `(ID, Version)`;
  virtualizing 100k lines through that cache is a dead end. The
  Editor instead owns its own `ScrollY` in `editorState` and sizes
  the DrawCanvas to the viewport. `ID: ""` bypasses the cache.
- `edit/text` wraps `gui.TextMeasurer`, not go-glyph directly. The
  go-glyph import is type-only (`glyph.Layout` is the return type
  of `TextMeasurer.LayoutText`).
- Golden-frame tests skipped. `DrawContext`'s draw-op accumulators
  are unexported; an external test can't inspect them. Revisit when
  an upstream `Inspect()` accessor can be pushed.
- Upstream change: pushed `(*Window).TextMeasurer()` getter to
  go-gui — one-line mirror of `SetTextMeasurer`.

### Phase 1.2 — File I/O  ☑

Settle before undo/highlight assume UTF-8 byte offsets.

- [x] EOL detect on load (LF / CRLF / CR / mixed); preserve on save.
      Normalize to LF in buffer, reapply original on write.
- [x] Final-newline-on-save policy (per-buffer flag, default on).
- [x] Trailing-whitespace-on-save policy (off by default).
- [x] Encoding detect: UTF-8, UTF-8-BOM, UTF-16 LE/BE, Latin-1, CP1252.
      Pure-Go chardet-style sniff; explicit override API.
- [x] BOM preserve flag per buffer.
- [x] Invalid UTF-8: lossless byte-passthrough mode so binaries round-trip.
- [x] Transcode on save back to original encoding.
- [x] Atomic save (tmp + rename); preserve mode/owner; symlink-aware.
- [x] External-change watch: reload clean buffers, prompt dirty ones.
- [x] Indent autodetect (tabs vs spaces, width) from file content.

Architectural notes:

- `LoadFile` wraps `Load` with encoding sniff + transcode + EOL detect +
  indent detect. Original encoding/EOL/BOM stored in `FileProps` for
  round-trip on save.
- Save path: `SaveFile` → atomic write (tmp+rename, symlink-aware),
  re-encodes to original encoding, reapplies original EOL, optional
  trailing-WS trim + final-newline append.
- Watcher: poll-based `os.Stat` with injectable clock, throttled to 1/sec.
  Notifies via callback on external change.
- Dep: `golang.org/x/text` for UTF-16/Latin-1/CP1252 codecs.

### Phase 1.5 — Extension substrate  ☑

Lock down before highlighting so chroma is the first consumer, not a special
case. Same machinery must carry diagnostics, folds, AI ghost text,
autocorrect, LSP, collab.

- [x] Single edit choke point: `Buffer.Apply(Edit)`; all mutations route
      through it.
- [x] `EditFilter` chain — observe / transform / veto edits. Use cases:
      autocorrect, AI accept, macro record, collab CRDT.
- [x] Mark + range tracker: positions/ranges auto-updated across edits,
      with gravity and subscribe API. Shared by diagnostics, folds,
      multi-cursor, search highlights, AI anchors.
- [x] Decoration provider interface:
      `Decorate(viewport) []Decoration` — virtual text (inline + block),
      gutter marks, line background, squiggles. Cursor/selection/copy
      math must distinguish document text from virtual text.
- [x] Async invalidation signal into go-gui frame loop (dirty regions).
      Required for LSP, AI, spell check, background tokenize.
- [x] Command registry + layered keymaps. Foundation for vim/emacs modes,
      AI accept/reject bindings, user rebinding.
- [x] Port Phase 4 highlighter onto this substrate as first real consumer;
      if the interface can't express "tokens as decorations + edit-range
      invalidation", it's wrong — find out now.

Architectural notes:

- `EditFilter` chain and `PostEditFunc` observers live on Buffer. Filters
  see post-clamp coordinates and can transform or veto edits. Vetoed
  edits do not dirty the buffer. Post-edit observers fire after Apply
  with the resulting Change.
- `MarkSet` uses a flat `[]*Mark` slice adjusted in O(n) per edit.
  Each mark has `Gravity` (left/right) controlling insert-at-mark
  behavior. `TrackedRange` pairs marks with opposed gravity so inserts
  inside expand the range.
- `DecorationProvider` interface + `Decoration` types live in
  `edit/buffer` so providers (like the highlighter) don't depend on
  `edit`. Only `DecoToken` rendering is implemented; other kinds
  (squiggles, gutter, virtual text) have types defined.
- `KeymapStack` replaces the hardcoded switch in `editorOnKeyDown`.
  `DefaultKeymap` maps keys to action IDs; actions are funcs in
  `defaultActions`. `EditorCfg.Keymaps` pushes user layers on top.
- `Highlighter` in `edit/highlight` is the first `DecorationProvider`.
  Chroma tokenizes the full buffer; per-line token cache invalidated
  on any edit. Synchronous viewport-first; background fill deferred.
  Chroma v2 doesn't expose per-line lexer state, so invalidation
  retokenizes from the start.
- `OnInvalidate` on `EditorCfg` delivers a `w.RequestRedraw` thunk
  to async providers via `editorAmendLayout`.
- Hardening: `AddFilter`/`OnEdit` guard nil funcs and double-remove.
  `MarkSet` caps at `MaxMarks` (1M), guards uint32 ID wrap.
  `Highlighter.Decorate` clamps negative/inverted viewports.
  `KeymapStack.Push` ignores nil. `renderStyledLine` guards nil
  measurer. `decosForLine` guards negative line index.
  `Highlighter.OnEdit` callback reads invalidate func under mutex.

### Phase 2 — Selection + clipboard  ☑

- [x] Shift+arrow selection; mouse drag selection; double-click word; triple
      line.
- [x] Cut/copy/paste via go-gui clipboard.
- [x] Tab / Shift-Tab indent (selection-aware).
- [x] Auto-indent on Enter.

Architectural notes:

- Selection model: `Anchor` field added to `editorState`. Selection exists
  when `Anchor != Cursor`. `PreservesAnchor` flag on `Action` controls
  whether the dispatch auto-collapses selection after action execution.
  Selection-extending actions (`select.*`) set this flag; all others
  auto-set `Anchor = Cursor`.
- Selection rendering: drawn directly in `editorOnDraw` as background
  rects per visible line (not via `DecoBackground` — selection is
  transient UI state, not a document decoration).
- Mouse selection: `OnClick` fires on mouse-down; single-click sets
  cursor, starts `MouseLock` drag. Double-click selects word via
  `wordBoundsAtByte`. Triple-click selects line. Click count tracked
  with 400ms threshold in `editorState`.
- Clipboard: `ActionFunc` extended with `*gui.Window` parameter to
  access `GetClipboard`/`SetClipboard`. Both `ModCtrl` and `ModSuper`
  bindings registered for cross-platform support.
- Indent: uses `buf.Props.IndentStyle` (from Phase 1.2 detect). Multi-line
  indent iterates last→first to avoid position invalidation. Dedent
  removes one tab or up to Width spaces from line start.
- Auto-indent: `insertNewline` copies leading whitespace from current
  line; adds one indent level after `{`.

### Phase 3 — Undo / redo  ☑

- [x] Linear undo stack of `Change` records; coalesce typing runs.
- [x] Redo on undo+edit clears forward stack.
- [x] Bench: 100k-line file, 10k random edits.

Architectural notes:

- Undo stack lives on `Buffer` (`undoStack` in `edit/buffer/undo.go`).
  `EnableUndo(now func() time.Time)` activates tracking; nil defaults
  to `time.Now`. Without `EnableUndo`, all undo API calls are no-ops.
- `applyCore(Edit, record bool) Change` is the internal mutation path.
  `Apply` calls it with `record=true` (runs filters, records to undo).
  `Undo`/`Redo` replay calls it with `record=false` (skips filters and
  undo recording; post-edit observers still fire for highlighter
  invalidation).
- Compound edits: `BeginGroup()`/`EndGroup()` on Buffer. Nestable;
  only the outermost `EndGroup` flushes accumulated changes into one
  undo entry. Used by paste, cut, newline (deleteSelection + insert),
  and multi-line indent/dedent.
- Coalescing: adjacent single-char inserts (typing), backward deletes
  (backspace), and forward deletes coalesce when within 500ms and no
  newline involved. Timeout breaks coalesce chain.
- Cursor restore: editor layer calls `SetUndoCursor(cursor, anchor)`
  before each edit action. `Undo()` returns `cursorBefore`; `Redo()`
  returns `cursorAfter`.
- Dirty tracking: `MarkClean()` records `cleanIdx = len(undo)`.
  `Undo`/`Redo` set `dirty = (len(undo) != cleanIdx)`.
- Benchmarks: undo recording adds ~10k allocs over 10k random edits
  (negligible). Replaying 10k undos takes ~21ms on 100k-line buffer.

### Phase 4 — Syntax highlighting  ☑ (pulled into Phase 1.5)

- [x] Integrate chroma; map chroma token types → theme styles.
- [x] Per-line token cache; invalidate on edit using line ranges.
- [x] Lazy tokenize visible viewport first; background fill.
- [x] Language autodetect from filename + content.

### Phase 5 — Multi-cursor  ☑

- [x] Multiple `Cursor` instances; merge overlapping; sort on edit.
- [x] Alt-click add cursor; Ctrl-D add next match; Esc collapse.
- [x] All edit ops apply per-cursor in reverse position order.

Architectural notes:

- Data model: `editorState.Cursors []CursorState` replaces the single
  `Cursor`/`Anchor`/`DesiredCol` triple. Index 0 is the primary cursor
  (drives scroll via `ensureCursorVisible`). `CursorState` type in
  `edit/cursor.go` with `HasSelection`, `SelectionRange`, `ClearSelection`.
- Sort/merge: `sortCursors` + `mergeCursors` in `edit/multicursor.go`.
  Called after every action dispatch and mouse event. Overlapping or
  touching selection ranges merge into their union. Cap at 1000 cursors.
- Per-cursor dispatch: `PerCursor` flag on `Action`. Per-cursor edit
  actions register a temporary `PostEditFunc` observer that adjusts all
  non-active cursors after each `buf.Apply` call, using the same shift
  logic as `markset.go`. Multi-cursor edits are grouped in one
  `BeginGroup`/`EndGroup` for atomic undo. Single-cursor char insert
  stays ungrouped to preserve typing coalesce.
- Undo: `UndoCursorState` extended with `Extra []CursorPair` to
  capture all cursors. `SetUndoCursorState` records the full set;
  undo/redo restores all cursors.
- Drawing: selection backgrounds and cursor rects iterate all cursors.
- Mouse: Alt-click adds a cursor (no drag). Regular click collapses
  to single. Shift-click extends primary, drops secondaries.
- Ctrl+D (`cursor.addNext`): first press selects word under cursor;
  subsequent presses search forward (wrapping) for next match and add
  a cursor selecting it.
- Escape (`cursor.escape`): collapses to primary cursor; if already
  single, clears selection.
- Clipboard: copy concatenates all selections with `\n`; cut does the
  same then deletes in reverse order. Paste splits by `\n` when line
  count matches cursor count; otherwise pastes full text at each cursor.

### Phase 6 — Search / replace  ☑

- [x] In-editor find bar; literal + regex; case toggle.
- [x] Find next/prev, highlight all matches.
- [x] Replace, replace all (single undo entry).
- [x] Find in selection (Alt+S toggle; auto-enabled for multi-line
      selections).

Architectural notes:

- Find bar is custom-drawn inside the existing DrawCanvas (not a
  `gui.Input` overlay). Avoids IDFocus conflicts. Rendered at
  top-right of viewport with semi-opaque background.
- `searchState` sub-struct on `editorState`, persisted in StateMap.
  Contains query, replace text, flags, compiled regex cache, and
  match list.
- Input routing: `editorOnKeyDown` and `editorOnChar` check
  `st.Search.Active` first and dispatch to `handleSearchKey` /
  `handleSearchChar`. Unhandled keys fall through to normal action
  dispatch (e.g., Ctrl+C still copies).
- Match highlights drawn directly in `editorOnDraw` like selection
  backgrounds. Yellow for all matches, orange for current match.
  `matchesForLine` uses binary search on sorted match slice.
- `findAllMatches` returns all matches via `bytes.Index` (literal)
  or `regexp.FindAllIndex` (regex), capped at 10,000. Compiled
  regex cached in searchState; recompiled only when query/flags
  change.
- Match invalidation: `editorAmendLayout` registers a `buf.OnEdit`
  observer that sets `matchesDirty`; recompute runs on next
  AmendLayout tick.
- Replace-all applies in reverse position order inside one
  `BeginGroup/EndGroup` for single undo. Regex replacement supports
  `$1` group expansion via `regexp.Expand`.
- Keybindings: Ctrl+F/Cmd+F open find, Ctrl+H/Cmd+H open replace,
  Enter/Shift+Enter navigate, Escape closes, Tab switches fields,
  Alt+R toggles regex, Alt+C toggles case sensitivity,
  Alt+S toggles find-in-selection, Ctrl+Enter replaces all.
- Find-in-selection: `InSelection` flag + `SelectionScope` range on
  searchState. Auto-enabled when opening find bar with a multi-line
  selection. `filterToScope` post-filters match results. `[Sel]`
  indicator in find bar toggles.

### Phase 7 — Quality of life  ☑

- [x] Bracket matching + auto-close pairs.
- [x] Line wrap (toggleable).
- [x] Code folding (indent-based first; language-aware later).
- [x] Whitespace + EOL visualization.
- [x] Sticky scroll (pinned scope headers).

Architectural notes:

- Bracket matching: `findMatchingBracket` in `edit/brackets.go`.
  Naive scan (no string/comment awareness) with 10k byte cap.
  Match computed per-frame in AmendLayout, stored in
  `editorFrameData.bracketMatch`. Drawn as background rects.
  `cursor.matchBracket` action jumps to match (Ctrl+Shift+\\).
- Auto-close pairs: `EditFilter` in `edit/autoclose.go`.
  Registered in AmendLayout. Default pairs: `()[]{}""''`\`\``.
  Skip-over in `editorOnChar`; backspace-both in backspace action.
  Quote auto-close suppressed after alphanumeric context.
- Whitespace viz: `edit/whitespace.go`. `WhitespaceMode` enum
  (None/All/Selection). Draws `·`/`→`/`↵` after line text.
  Runtime toggle via `view.toggleWhitespace` action cycling
  `WhitespaceOverride` in editorState.
- Code folding: `edit/fold.go`. Indent-based `foldRangeAt`
  detection. `FoldedRanges []FoldRange` persisted in editorState.
  Draw loop skips folded lines; gutter shows `>` on fold headers
  with ` ...` ellipsis. Cursor movement skips folds (down → past
  fold, up → to header). `PostEditFunc` unfolds ranges overlapping
  edits. Gutter click toggles fold. `fold.toggle` (Ctrl+Shift+[),
  `fold.all`, `fold.unfoldAll` (Ctrl+Shift+]) actions.
- Line wrap: `edit/wrap.go`. `computeBreaks` finds byte-offset
  break points per line, preferring word boundaries. Draw loop
  iterates visual sub-rows. `globalLogicalToVisualRow` /
  `globalVisualRowToLogical` for scroll/cursor mapping.
  `totalVisualRowsForBuffer` for scroll clamping. `WrapOverride`
  in editorState for runtime toggle (Alt+Z). Cursor Y and hit-test
  account for sub-row offsets.
- Sticky scroll: `edit/stickyscroll.go`. `findScopeHeaders` walks
  backward from first visible line collecting lines with
  decreasing indent. Drawn as opaque overlay at viewport top with
  syntax highlighting. Up to `StickyScrollMax` (default 5) headers.

### Phase 8 — Polish  ☐

- [ ] Drag-and-drop file open.
- [ ] Per-language config (tab width, comment string).
- [ ] Diagnostics gutter API (markers, squiggles) — no LSP yet, just an API
      surface for callers to push markers.
- [ ] Theme: derive from go-gui theme; override per-token colors.
      (Currently hardcoded to "monokai". Needs theme bridge.)
- [ ] Accessibility: a11y tree integration via go-gui NativePlatform.
- [ ] Help screen: keybinding to replace buffer with read-only shortcut
      reference. Gathers bindings from DefaultKeymap + user keymaps.
      Dismiss restores original buffer. Invoke via F1 or Ctrl+?.

### Future (post-1.0)

- AI assist (ghost-text completion, inline chat, refactor) — consumer of
  Phase 1.5 substrate; provider-agnostic interface, no provider in core.
- Autocorrect / spell check — `EditFilter` + decoration squiggles.
- LSP client (separate subpackage; opt-in).
- Tree-sitter via WASM (still no CGO) if chroma proves insufficient.
- Collaborative editing (CRDT).
- Inline diff / blame gutter.
- Snippets, completion popup.
- Vim / Emacs keymaps as separate packages.

## Open questions

- Per-line gap buffer: cap on line length before split/fallback for
  pathological long lines (minified JS, logs)?
- ~~Token cache granularity: per-line vs per-chunk.~~ Per-line (Phase 1.5).
- Theme model: extend go-gui Theme or standalone EditorTheme?
- Where does cursor blink animation live — go-gui animation subsystem or
  internal ticker?
- Soft-wrap impact on cursor column math — visual vs logical columns.
- Large-file strategy: memory-map? streaming load? hard cap?
- Undo coalescing rules: time window, char class, or both?
- IME composition: route through go-gui NativePlatform IME hooks?
- AI ghost text: document text or pure decoration? (cursor/selection math)
- Autocorrect: synchronous `EditFilter` or async suggestion like AI?
- ~~EditFilter ordering + conflict resolution when two filters touch same edit?~~
  Registration order; first rejection stops chain (Phase 1.5).
- ~~Decoration providers: render thread or worker?~~ GUI goroutine via
  `Decorate(vp)`. Background work behind provider's own mutex (Phase 1.5).
- ~~Mark gravity: per-mark or per-API default?~~ Per-mark (Phase 1.5).
- ~~CRLF: normalize-in-buffer + reapply on save, or store verbatim?~~
  Normalize in buffer, reapply on save (Phase 1.2).
- ~~Non-UTF-8: transcode-in or byte-passthrough mode?~~
  Both; `EncodingRaw` for byte-passthrough (Phase 1.2).
- ~~BOM: preserve flag per buffer, or global?~~ Per-buffer `PreserveBOM`
  (Phase 1.2).
- ~~Autodetect indent on load, or require explicit config?~~
  Autodetect via `detectIndent` (Phase 1.2).
- ~~External change: prompt, auto-reload, or both (dirty vs clean split)?~~
  Poll-based watcher, callback on external change (Phase 1.2).
- One `Editor` per window, or N? Affects state-slot keying.
- Public API: `edit` only, or split `edit/buffer` as independently importable?
- Gutter+text one canvas or two sharing scroll Y?
- Minimap: same widget second viewport, or sibling reading shared buffer?
- Fallback font chain for CJK / emoji — go-glyph default or custom?
- Subpixel positioning needed, or integer advance fine for monospace?
- Shaping cache lifetime: per-frame, per-line-edit, or LRU?

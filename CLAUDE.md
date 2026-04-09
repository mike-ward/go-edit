# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

go-edit is a code editor widget for [go-gui](https://github.com/mike-ward/go-gui). Sibling repo at `../go-gui`; local `replace` directives in `go.mod` point there and at `../go-glyph`. Syntax highlighting is planned via chroma; text shaping via go-glyph (through go-gui's `TextMeasurer`).

## Common commands

```
go test ./edit/...              # run all unit tests
go test -race ./edit/...        # with race detector
go test ./edit/buffer/ -run TestApplyOnEmptyBuffer   # single test
go test -fuzz=FuzzBufferApply -fuzztime=30s ./edit/buffer/
go test -bench=. -benchmem -run='^$' ./edit/buffer/  # buffer benches
go vet ./...
golangci-lint run               # config in .golangci.yml, copied from go-gui
go build ./examples/basic       # requires CGO backend (SDL2, freetype, etc.)
```

Tests run fully headless. `examples/basic` is the only CGO-linked target.

## Architecture

### Constraints that shape everything

1. **DrawCanvas caches its output** keyed by `(shape.ID, shape.Version)` (see `go-gui/gui/render_draw_canvas.go`). The editor uses `ID: ""` to bypass the cache because buffer/cursor/scroll change every frame.
2. **`OnDraw(*DrawContext)` has no access to `*Window`, `TextMeasurer`, or Theme.** Everything the draw path needs must be closed over at `GenerateLayout` time. The pattern:
   - `Editor(cfg)` allocates a closure-shared `*editorFrameData`.
   - `AmendLayout(layout, w)` (which does have `*Window`) loads persistent state from `StateMap`, builds the `text.Measurer` lazily, populates the frame struct.
   - `OnDraw(dc)` reads the closed-over frame struct. No window access needed.
   - Input callbacks (`OnKeyDown`, `OnChar`, `OnMouseScroll`) receive `*Window` per event and read/write `StateMap` directly.
3. **Editor owns its own `ScrollY`**, not go-gui's `Column(IDScroll)` mechanism. DrawCanvas is sized to the viewport; scroll state lives in `editorState.ScrollY`; mouse wheel via `OnMouseScroll`; keys adjust via `ensureCursorVisible`.

### Package layout

- `edit/` — `EditorCfg`, `Editor(cfg) gui.View` factory, closures for draw/input/amend. State structs in `editor_state.go`.
- `edit/buffer/` — document model. `Buffer` is a slice of lines (plain `[]byte` for now; gap buffer deferred until benchmarks justify the rewrite). The single mutation choke point is `Buffer.Apply(Edit) Change`. `Load(io.Reader)` caps at `MaxLoadBytes` (256 MiB).
- `edit/text/` — wraps `gui.TextMeasurer`. `Measurer` caches monospace advance + line height and exposes `XForColumn` / `ColumnForX` with an ASCII fast path falling back to `glyph.Layout.HitTest`/`GetCursorPos`.
- `edit/internal/fakewin/` — headless test fixture. `fakewin.New()` returns a `*gui.Window` with a deterministic fake `TextMeasurer` (8 px advance, 16 px line height). Event builders (`NewKeyEvent`, `NewCharEvent`, `NewScrollEvent`). Used by driver tests in `edit/editor_driver_test.go`.
- `examples/basic/` — minimal CLI example; links the CGO backend.

### Type decisions (locked — see ROADMAP Phase -1)

- `buffer.Position` = `{Line, ByteCol int}` — byte offsets, not runes or graphemes. Grapheme-cluster movement is a Phase 2 concern.
- `buffer.Edit` = single `{Range, NewBytes []byte}` shape. No tagged union; insert is `Range.Empty()` + NewBytes, delete is non-empty Range + nil NewBytes.
- `buffer.Change` is the undo record returned from `Apply`; Phase 3 will consume it.
- Canonical coordinate space is logical `(line, col)`. Visual `(row, x)` is derived per frame for wrap/folds.

### Public-API hardening invariants

Public entry points are guarded against bad input and should stay that way:

- `buffer.Load(nil)` → empty buffer; over `MaxLoadBytes` → error.
- `text.New(nil, ...)` → nil; nil-receiver `Measurer` methods return zero.
- `Measurer.ColumnForX(..., NaN)` → 0.
- `Editor(cfg)` with nil Buffer substitutes `buffer.New()`. Width/Height run through `sanitizeDim` (clamps NaN/Inf/negative/zero/over-large to `[1, 1<<20]`).
- `editorOnMouseScroll` drops NaN or absurd (`|dy| > 1e6`) scroll deltas.
- `clampScroll` / `ensureCursorVisible` guard NaN and zero line-height.
- `editorOnChar` filters via `acceptChar` (`unicode.IsPrint(r) || r == '\t'`).

Any new public entry point should follow the same pattern; add tripwire tests in `*_hardening_test.go`.

### StateMap namespace

Editor state is stored in `gui.StateMap[uint32, editorState](w, "edit.state", capEdit)` keyed by `cfg.IDFocus`. Follows go-gui's dotted-namespace convention (`gui.input`, `gui.scroll.y`, etc.).

## Working agreements (from user global rules)

- Never modify files in `~/Documents/github/v`.
- Keep commit messages and responses extremely concise; sacrifice grammar for brevity.
- Passive/imperative voice; avoid "we" / first-person.
- Performance work should favor reducing heap allocations.
- **Never stage or commit without explicit user confirmation.**
- Prefer pushing missing capability upstream to go-gui over local workarounds (already done once: `(*Window).TextMeasurer()` getter).

## Status

Phases 0, 1, and 1.2 (File I/O) are committed. Golden-frame tests are deferred until an upstream `DrawContext.Inspect()` accessor exists. The plain-`[]byte` line store will be replaced with a per-line gap buffer once bench pressure from Phase 3 (undo) justifies it. See `ROADMAP.md` for the full phase list and open questions.

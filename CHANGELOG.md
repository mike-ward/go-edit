# Changelog

## v0.10.2 — 2026-05-17

- deps: bump go-gui to v0.19.1 (scroll phase bridge, context menu focus fix).
- deps: bump go-gui to v0.19.0, go-glyph to v1.7.1 (animation heartbeat,
  Metal autorelease fix).
- lint: use `slices.Backward` for reverse loops in undo and fold.

## v0.10.1 — 2026-05-01

- buffer/watcher: emit `WatchDeleted` once and unwatch missing paths.
- buffer/watcher: detect external edits by `(modTime, size)`.
- buffer/save: stream `(*Buffer).WriteTo` via `io.Copy`; surface short
  writes as `io.ErrShortWrite`.
- buffer/save: snapshot + recheck symlink target before atomic commit;
  fail on mid-save target changes.
- buffer: cut multiline insert allocations via in-place splice.

## v0.10.0 — 2026-04-30

- editor: harden `EditorCfg.Font` override. Empty `Family` borrows
  `theme.M5.Family`; NaN / Inf / non-positive `Size` borrows
  `theme.M5.Size`; oversized `Size` clamped to 1024. Prevents
  proportional-font fallback and huge-glyph allocations from
  hostile or partially-populated configs.
- editor: per-widget `Font` override (`EditorCfg.Font`) for
  callers that want a non-theme monospace style (e.g. npad uses
  SF Mono Terminal).
- deps: bump go-gui v0.12.5 → v0.17.0, go-glyph → v1.7.0; drop
  local `replace` directives now that go-gui carries the
  upstream `TextMeasurer` surface.

## v0.9.0

Initial tagged release.

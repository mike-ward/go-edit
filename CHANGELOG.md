# Changelog

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

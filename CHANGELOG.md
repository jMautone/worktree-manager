# Changelog

## [Unreleased] — v1 (Go)

v1 is a from-scratch rewrite in Go, with macOS and Windows as first-class
platforms. No code is ported from v0.9. See
[docs/decisions/0001-de-cero-en-go.md](docs/decisions/0001-de-cero-en-go.md).

### Added

- Project bootstrap: Go module, package layout, CI matrix on macOS, Windows and
  Linux, plus cross-compilation checks for all release targets.
- [`docs/design/product.md`](docs/design/product.md) — the full product design:
  CLI contract, config schema, the 23 capabilities and the M1–M6 execution order.
- OpenSpec project context and per-artifact rules, including the rule that specs
  stay language-agnostic.

### Changed

- Nothing works yet. `main` has no usable commands; the stable line is `v0.9.x`.

---

## [0.9.0] — 2026-09-21

Last PowerShell release. Windows only, PowerShell 5.1+. Frozen: this line
receives no new features.

Its history is preserved on the `v0.9.x` branch and the `v0.9.0` tag. The
changelog for that line is at
[docs/historial/CHANGELOG-v0.9.md](docs/historial/CHANGELOG-v0.9.md).

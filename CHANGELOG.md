# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
SemVer with one minor per milestone, and every merged change ships as a
pre-release of it; see [CONTRIBUTING.md](CONTRIBUTING.md).

## [Unreleased]

v1 is a from-scratch rewrite in Go, with macOS and Windows as first-class
platforms. No code is ported from v0.9. See
[docs/decisions/0001-de-cero-en-go.md](docs/decisions/0001-de-cero-en-go.md).
It ships as `0.x` pre-releases until M5 closes with `1.0.0`.

### Added

- Project bootstrap: Go module, package layout, CI matrix on macOS, Windows and
  Linux, plus cross-compilation checks for all release targets.
- [`docs/design/product.md`](docs/design/product.md) — the full product design:
  CLI contract, config schema, the 23 capabilities and the M1–M6 execution order.
- OpenSpec project context and per-artifact rules, including the rule that specs
  stay language-agnostic.
- Release pipeline: every merged change publishes a GitHub pre-release with
  binaries for macOS, Windows and Linux, plus `checksums.txt`.
- `wt list`: one table with every worktree of the current repository, current
  (`@`) and main (`^`) marks and a `STATE` column; `--json` uses `wt.list.v1`.
- `wt config path|list|get`: layered configuration (defaults < user file <
  `.wt.toml` < `WT_*`), with an allowlist for the repository file.
- The CLI contract: documented exit codes 0–8, strict validation of commands
  and flags, `wt:`/`hint:` errors or `wt.error.v1` with `--json`, and the
  global flags `--json`, `--dry-run`, `-C` and `--no-color`.
- `wt version`, which works even when the configuration is broken.
- `wt cd <target>`: moves the shell to a worktree of the current repository by
  name or branch, to the main worktree (`^`), to the root of the current one
  (`@`), or back to the previous directory (`-`); `--json` uses `wt.cd.v1`.
- `wt shell init zsh|bash|fish|pwsh`: prints the `wt` shell function that runs
  `git-wt` and moves the shell; `--json` uses `wt.shell.init.v1`. In
  PowerShell on Windows the function hides `wt.exe` (Windows Terminal).
- Completions in the four shells for commands, flags, worktree names in
  `wt cd` and shell names in `wt shell init`.
- The protocol between the function and the binary: the environment
  variables `WT_DIRECTIVE_CD_FILE` and `WT_PREVIOUS_DIR`, which `wt` does not
  pass to the processes it starts.

### Changed

- `main` has read-only commands only (`list`, `cd`, `config`, `shell init`,
  `version`); the stable line is still the PowerShell tag `powershell-v0.9.0`.
- The PowerShell line was renamed: the tag `v0.9.0` is now `powershell-v0.9.0`,
  and the `v0.9.x` branch was removed (the tag points at the same commit).

### Fixed

- `wt version` reports the module version when installed with
  `go install …@vX.Y.Z`, instead of `0.0.0-dev`.

---

## PowerShell 0.9.0 — 2026-09-21

Last PowerShell release. Windows only, PowerShell 5.1+. Frozen: this line
receives no new features.

Its history is preserved in the `powershell-v0.9.0` tag. The changelog for
that line is at
[docs/historial/CHANGELOG-v0.9.md](docs/historial/CHANGELOG-v0.9.md).

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
- `wt create <name>`: creates a worktree of the current repository on a new
  branch, at the path `worktree_path` dictates, and moves the shell into it.
  The branch is `branch_prefix` followed by `<name>`, or `-b <branch>`; it
  starts at `--base`, `default_base` or the repository's default branch, and
  has no upstream. A branch or a path that already exists is exit 5. Shell
  completion offers branches for `--base`; `--json` uses `wt.create.v1`, also
  with `--dry-run`.
- `wt create -x <cmd>` runs a command in the new worktree, in the foreground,
  with `sh -c` on macOS and Linux and `cmd.exe` on Windows, and exits with its
  exit code. `--cd` and `--no-cd` override `create_cd` for one run.
- Path templates: `{repo}`, `{repo_parent}`, `{repo_path}`, `{name}` and
  `{branch}`, with the filters `sanitize` (the same valid directory name on
  every OS) and `lower`. An invalid template in `worktree_path` is invalid
  configuration (exit 1).
- Configuration keys `branch_prefix`, `fetch_before_create` (fetch the
  remote of a `<remote>/<branch>` base before creating) and `create_cd`, which
  `.wt.toml` may not set. Boolean keys read `true`, `false`, `1` or `0` from
  `WT_*`.
- `wt remove <target>`: removes a worktree of the current repository, resolved
  as in `wt cd`, and deletes its branch when it is merged: when its tip is in
  the base (`default_base` or the default branch) or in its upstream, judged
  without fetching; a squash merge does not count. `--keep-branch` keeps the
  branch, `-D` (`--force-delete-branch`) deletes it even if not merged, and
  `-f` (`--force`) removes a worktree with modified or untracked files; `-f`
  never deletes an unmerged branch. A locked worktree, one that contains
  another worktree, or one with uncommitted work (without `-f`) is exit 5;
  the main worktree is exit 2. Ignored files are deleted with the worktree.
  Removed from inside, the shell ends in the main worktree. Coming from
  v0.9: the merged branch is deleted by default (it needed `--delete-branch`)
  and `-D` replaces `--force-branch`.
- `wt lock <target> [<reason>]` and `wt unlock <target>`, and `wt prune`,
  which forgets the worktrees whose directory is gone and never deletes
  branches.
- JSON schemas `wt.remove.v1`, `wt.lock.v1`, `wt.unlock.v1` and
  `wt.prune.v1`, the same with `--dry-run`.
- Completions for the `<target>` of `wt remove` and `wt lock` (worktrees that
  are neither main nor locked) and of `wt unlock` (locked worktrees).

### Changed

- `main` covers the minimum cycle of M1: create, move to and remove
  worktrees; the stable line is still the PowerShell tag
  `powershell-v0.9.0`.
- The default `worktree_path` is `{repo_parent}/{repo}.worktrees/{name|sanitize}`
  (it was `{branch|sanitize}`), so that `wt cd <name>` finds what `wt create`
  made whatever `-b` or `branch_prefix` chose. The key is now validated as a
  template.
- Exit codes: once a command given with `-x` starts, its exit code replaces
  `wt`'s own, unchanged.
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

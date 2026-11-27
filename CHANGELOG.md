# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
SemVer with one minor per milestone, and every merged change ships as a
pre-release of it; see [CONTRIBUTING.md](CONTRIBUTING.md).

## [Unreleased]

### Added

- Hooks: a `[hooks]` table in the user configuration file sets, for each of
  the events `pre-create`, `post-create`, `pre-cd`, `post-cd`, `pre-remove`
  and `post-remove`, a list of commands that run in order, with `sh -c` on
  macOS and Linux and `cmd.exe` on Windows, as `-x`; the first that fails
  stops the rest. Each event is fired only by its command: `wt create`,
  `wt cd` or `wt remove`. The keys `hooks.<event>` have no
  environment variable, and `wt config get` prints one command per line.
- `pre-create`, `post-create`, `pre-cd` and `pre-remove` block: `wt` waits
  for them, writes `==> <event>  <command>` before each command, and sends
  their output to stderr, so that stdout stays `wt`'s own. `post-create`
  runs in the new worktree before the shell moves and before `-x`, so that
  `-x` starts in a tree that is ready.
- `post-cd` and `post-remove` run in the background, detached from the
  terminal: they outlive `wt`, Ctrl-C and closing the terminal, on every OS.
  `wt` prints where their log is, `<git common dir>/wt/logs/<name>/<event>.log`,
  which has a header, the output and the exit code of each command, and is
  replaced on every run. Their outcome never changes `wt`'s exit code.
- Hook commands are templates with `{repo}`, `{repo_parent}`, `{repo_path}`,
  `{name}` (the worktree's NAME, as in `wt list`), `{branch}`, `{path}`, and
  `{base}` in the events of `wt create`. Values are inserted as they are;
  the quotes are the user's to write. An invalid template is invalid
  configuration, exit 1.
- `--no-hooks` on `wt create`, `wt cd` and `wt remove` runs no hook.
- A blocking hook that fails ends the command with exit 7, a code that was
  reserved: a `pre-*` leaves nothing done, and a failed `post-create` leaves
  the worktree created and moves the shell into it, but does not run `-x`.
  Ctrl-C while a blocking hook runs reaches the hook; once it ends, `wt`
  stops the operation the same way and exits with 130.
- `--dry-run` prints `would run <event> hook: <command>`, or `would run
  <event> hook in the background: <command>`, with the commands rendered,
  in the order they would run. With `--json`, blocking hooks write to their
  log instead of stderr, and a failed one is a `wt.error.v1` with code 7 or
  130 whose hints name the log.
- Repository hooks: a repository's `.wt.toml` may have a `[hooks]` table,
  with the user's events and form. Its commands run after the user's of the
  same event, never instead of them, and only once approved. An approval is
  the repository (its main worktree's path), the event and the SHA-256 of
  the command as written; changing the command asks for it again. Approvals
  live in `approvals.toml`, next to the user configuration file.
- `wt create`, `wt cd` and `wt remove` exit with 8, a code that was
  reserved, before doing anything, when an event they would fire has a
  repository command that is not approved, also under `--dry-run`. The hints
  give the command that approves it and `--no-hooks`.
- `wt hook list [--json]` shows every hook command that would run in the
  current repository, with its source and, for the repository's, whether it
  is approved (`wt.hook.list.v1`). `wt hook approve [<event>...]` approves
  the ones that are not, with `--dry-run` and `--json`
  (`wt.hook.approve.v1`).
- `wt config path` shows `approvals.toml`; `wt.config.path.v1` gains the
  field `approvals`.
- `wt sync [<name>...] [--all] [--base <ref>] [--no-fetch]` rebases
  worktrees onto their base: the current one, the ones named (as `wt remove`
  takes them, other repositories included), or with `--all` every linked
  worktree of the current repository. The base is `--base`, otherwise the
  repository's `default_base` or default branch; a base on a remote is
  fetched once per repository. It never stashes: a worktree with changes to
  tracked files, a detached HEAD, an operation in progress or no directory
  is skipped. A rebase that stops on a conflict is aborted, leaving the
  worktree as it was, and the next one goes on. One line per worktree, a
  summary on stderr, and exit 1 (a rebase failed), 6 (a conflict) or 5 (a
  worktree skipped) when not every worktree was synced. With `--json`, the
  schema `wt.sync.v1`.
- `wt merge [<target>] [--no-rebase] [--no-remove] [--no-hooks]` closes the
  current worktree: it checks that the worktree has no modified or untracked
  files and that the hooks that would run are approved, rebases the branch
  onto the target (a local branch; by default the repository's base without
  its remote, so `origin/main` gives `main`), runs `pre-merge`,
  fast-forwards the target, updating the worktree that has it checked out,
  removes the worktree and its branch as `wt remove` does, and runs
  `post-merge`. It stops at the first step that fails: a conflict aborts the
  rebase (exit 6), a failed `pre-merge` leaves the target where it was
  (exit 7). It never fetches, pushes or creates a merge commit. With
  `--json`, the schema `wt.merge.v1`.
- The hook events `pre-merge`, which blocks and runs in the worktree, and
  `post-merge`, which runs in the background in the main worktree, with the
  keys `hooks.pre-merge` and `hooks.post-merge` and the variable `{target}`,
  only in those two events.
- `wt step commit|squash|rebase|push [<target>]` runs one step of
  `wt merge` on the current worktree, and nothing else: `commit` commits
  every pending change (modified and untracked files, not the ignored
  ones); `squash` replaces the branch's commits that the target does not
  contain, and the pending changes, by one commit on their merge base;
  `rebase` rebases onto the target as `wt merge` does; `push` fast-forwards
  the local target between `pre-merge` and `post-merge`, and never runs
  `git push`. A step with nothing to do says so and exits 0. With `--json`,
  the schema `wt.step.v1`.
- The commit message of `wt step commit`, `wt step squash` and `wt merge`
  is `-m <msg>`, or what the user writes in git's own editor, which opens
  with the squashed messages for a squash. An empty message commits
  nothing (exit 1); without a terminal, or with `--json`, `-m` is needed
  (exit 2). When a commit fails or is interrupted, the branch and the index
  are left as they were. The repository's own git hooks run.
- `wt merge --squash`, which squashes the branch before rebasing, `-m`, and
  `--no-commit`, which keeps the M3 behavior: pending changes block the
  merge with exit 5. `wt.merge.v1` gains `committed` and `squashed`.
- The hook events `pre-commit`, which blocks and runs before the message is
  asked for, and `post-commit`, which runs in the background, in the main
  worktree when `wt merge` removes the worktree, with the keys
  `hooks.pre-commit` and `hooks.post-commit`.

### Changed

- `wt merge` commits the pending changes of the worktree, between
  `pre-commit` and `post-commit`, instead of stopping with exit 5.
  `--no-commit` stops as before.

- In every template, `worktree_path` included, `{{` is a literal `{` and
  `}}` a literal `}`, as in `awk '{{print $1}}'`. Both were errors before,
  so no valid template changes meaning.
- `wt create` prints its `created worktree` line after `post-create`, and
  after writing the directive: when either fails, stdout stays empty.
- A `[hooks]` table in `.wt.toml` is no longer ignored with a warning: until
  its commands are approved, `wt create`, `wt cd` and `wt remove` fail with
  exit 8. `wt config get` and `wt config list` still show only the user's
  hooks.

## [0.2.0] — 2026-10-19

### Added

- `wt repos`: lists the repositories found under the roots in `repos_root`,
  ordered by name ignoring case, with `@` on the repository of the working
  directory; `--json` uses `wt.repos.v1`. A linked worktree is never a
  repository; one with the `.bare` layout or a separate git directory is. A
  root that does not exist is a warning, not an error.
- Configuration keys `repos_root`, a list of roots that are absolute or start
  with `~`, and `repos_depth`, how many levels below each root to search (1 to
  3, default 1), with the environment variables `WT_REPOS_ROOT` (split at `:`,
  or at `;` on Windows) and `WT_REPOS_DEPTH`. `wt config get` prints a list
  one element per line, and `wt config list` as a TOML array.
- `wt cd <name>` jumps to a repository under the roots from any directory,
  when the current repository has no worktree or branch with that name: by
  exact name, then ignoring letter case, then by a unique prefix ignoring case
  (`wt cd trend` → `TrendFisher`). A name that matches more than one
  repository is exit 4, with a hint per candidate.
- Completions of `wt cd` offer the names of the repositories under the roots,
  also outside a repository.
- The qualified form `<repo>:<name>` names the worktree `<name>` of the
  repository `<repo>` from anywhere, in `wt cd`, `wt path`, `wt remove`,
  `wt lock`, `wt unlock` and `wt create`. `<repo>` matches as in `wt cd
  <repo>`, or, with a `/` (also `\` on Windows), as the end of the
  repository's path, which tells repositories with the same name apart
  (`GIT/api:feat`). `api:` is the repository itself, and `api:^` its main
  worktree. Hints and completions name such repositories by the shortest
  path suffix that is unique.
- `wt cd <name>` also reaches the linked worktrees of the other repositories
  under the roots, by name and then by branch, when neither the current
  repository nor a repository name matched. A name found in more than one
  repository is exit 4, with a hint per candidate in its qualified form.
- `wt path <target>` prints the path `wt cd <target>` would move to, without
  the shell integration; `--json` uses `wt.path.v1`.
- `wt remove`, `wt lock` and `wt unlock` resolve a name in the current
  repository and then among the linked worktrees of the other repositories,
  also outside a repository; `wt create <repo>:<name>` creates the worktree
  in that repository, with its branches, remotes and default branch.
- `wt list --all-repos` lists the worktrees of every repository under the
  roots in one table with a `REPO` column; with `--json`, each element of
  `wt.list.v1` adds `repo` and `repo_path`, and the document an `unreadable`
  array. A repository git cannot read is skipped with a warning.
- Completions of `wt cd`, `wt path`, `wt remove`, `wt lock` and `wt unlock`
  offer the worktrees of every repository under the roots, and `<repo>:` the
  worktrees of that repository; each candidate carries a description of where
  it comes from. `wt create` offers `<repo>:` for `<name>`, and `--base` the
  branches of the repository it names.
- `wt each [--all-repos] [--] <cmd>...` runs a command in every worktree of
  the current repository, main worktree included, or with `--all-repos` in
  every worktree `wt list --all-repos` lists, in that order: one at a time,
  in the root of each worktree, with the terminal. The command starts at the
  first word that is not a flag of `wt`, so `--` is optional, and every word
  after it is the command's (`wt each npm test --json`). A single word is
  the command line as written, run by `sh -c` or `cmd.exe` as `-x` is
  (`wt each 'npm ci && npm test'`); several words reach the program each one
  whole (`wt each git commit -m "fix bug"`). Before each command, a line
  `==> <worktree>  <path>` names the worktree as `wt cd` takes it. When a
  command fails, the next one still runs, and at the end a summary lists the
  failed ones and `wt each` exits with 1. A bare repository is skipped, and
  so is, with a warning, a worktree whose directory does not exist.
  `--dry-run` prints `would run <line> in <path>` for each worktree. Ctrl-C
  reaches the running command; once it ends, `wt each` runs no other and
  exits with 130, a new exit code that means the same on every OS.

### Changed

- When `wt cd <name>` finds nothing, its message says whether it looked for a
  worktree, a repository or both; outside a repository with no roots
  configured, a hint suggests setting `repos_root`.
- With roots configured, `wt cd <name>` that finds nothing outside a
  repository says `no worktree or repository named "<name>"`, and its hints
  name `wt list --all-repos` and `wt repos`. The hints of an exit 4 among
  repositories now give each candidate's qualified form with its path.
- Outside a repository with roots configured, `wt list` and `wt create
  <name>` add a hint that names `wt list --all-repos` and `<repo>:<name>`.
- When a command operates on a worktree of another repository, the
  repository layer of the configuration is that repository's `.wt.toml`, in
  its main worktree, not the working directory's: `wt remove api:feat` judges
  whether the branch is merged with `api`'s `default_base`.
- In PowerShell, a `--` without quotes never reaches `wt`: PowerShell drops
  it before the function gets its arguments, so `wt config get -- --x` reads
  `--x` as a flag. `'--'`, in quotes, does reach it, and no command of `wt`
  needs a `--`.

### Removed

- A worktree or repository whose name contains `:` is no longer reached by
  its bare name, which is now a qualified target; a worktree is still reached
  as `<repo>:<name>`. Only macOS and Linux allow `:` in a directory name.

## [0.1.0] — 2026-10-07

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
  worktrees.
- The default `worktree_path` is `{repo_parent}/{repo}.worktrees/{name|sanitize}`
  (it was `{branch|sanitize}`), so that `wt cd <name>` finds what `wt create`
  made whatever `-b` or `branch_prefix` chose. The key is now validated as a
  template.
- Exit codes: once a command given with `-x` starts, its exit code replaces
  `wt`'s own, unchanged.

### Fixed

- `wt version` reports the module version when installed with
  `go install …@vX.Y.Z`, instead of `0.0.0-dev`.

---

## PowerShell 0.9.0 — 2026-09-21

Last PowerShell release. Windows only, PowerShell 5.1+. Retired: the Go
rewrite replaces it, and its tag no longer exists. Its commits remain in the
history of `main`. The changelog for that line is at
[docs/historial/CHANGELOG-v0.9.md](docs/historial/CHANGELOG-v0.9.md).

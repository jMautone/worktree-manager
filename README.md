# wt

A git worktree manager for multi-repo workspaces, built for the terminal.

> ### ⚠️ v1 is being rewritten. `main` is not usable yet.
>
> **Looking for something that works?** The stable line is PowerShell, Windows-only:
>
> ```
> git checkout powershell-v0.9.0
> ```
>
> `main` is a from-scratch rewrite in Go, targeting macOS and Windows as
> first-class platforms. It has no working commands yet. See
> [docs/design/product.md](docs/design/product.md) for the full design and
> [docs/decisions/0001-de-cero-en-go.md](docs/decisions/0001-de-cero-en-go.md)
> for why.

---

## What it is

Most worktree tools assume you are already inside a repository. `wt` doesn't. It
discovers repositories under configurable roots, resolves a name across all of
them (reporting ambiguity instead of guessing), and operates on many worktrees
at once.

The workflow it is built for:

> You have 10 repositories and 3 AI agents working in parallel in different
> worktrees spread across them. You want to jump to any of them by name, see the
> state of all of them in one table, run a command across them, and close a
> worktree — rebase, test, merge, delete — with a single command.

## Principles

- **Terminal-first.** A command leaves you standing in the worktree, in the same
  terminal. Nothing opens an external application by default.
- **Agents run in the foreground.** `-x <cmd>` runs inside the worktree, in your
  terminal, and propagates its exit code. A new tab is opt-in.
- **Nothing is hardcoded to one tool.** Editor, terminal and agent are
  configurable launchers with per-OS presets.
- **A repository does not run code without permission.** Project config is
  allowlisted; project hooks require explicit approval by command hash.

## Status

| | |
|---|---|
| tag `powershell-v0.9.0` | **Stable.** PowerShell 5.1+, Windows only. Frozen — no new features. |
| `main` | **In development.** Go, macOS + Windows + Linux. Pre-releases on [Releases](https://github.com/jMautone/worktree-manager/releases). See the milestone map below. |

Development is tracked with [OpenSpec](https://openspec.dev): durable
capabilities live in `openspec/specs/`, work in flight in `openspec/changes/`.
`openspec/specs/` grows only as changes land, so it never describes a product
that does not exist. The workflow is in
[CONTRIBUTING.md](CONTRIBUTING.md#working-on-an-openspec-change).

```
  M1  usable skeleton    list, cd, create, remove + shell integration
  M2  the differentiator multi-repo workspace, cross-repo resolution, batch
  M3  closing the loop   hooks, sync, merge
  M4  richness           one rich list view, branch state, picker, statusline, merge steps
  M5  extension          launchers, aliases, project hooks, doctor
  M6  product            dashboard, agent integrations, distribution, generated docs
```

Each milestone ships as one minor version, and every merged change as a
pre-release of it: M1 is `0.1.0`, M2 `0.2.0`, M3 `0.3.0`, M4 `0.4.0`, and M5
is `1.0.0`. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Installing

Download the archive for your platform from
[Releases](https://github.com/jMautone/worktree-manager/releases) and put
`git-wt` on your `PATH`. With Go installed, name the version explicitly:

```sh
go install github.com/jMautone/worktree-manager/cmd/git-wt@v0.1.0-alpha.2
```

`@latest` does not work until `1.0.0`: the Go module proxy still serves the
PowerShell `v0.9.0`, which has no Go code.

## Building

```sh
go build ./cmd/git-wt
```

The binary is named `git-wt`, which also makes it available as `git wt <cmd>`.
The `wt` you type is a shell function installed by `wt shell init`, because a
binary cannot change its parent shell's working directory.

## License

See [LICENSE](LICENSE).

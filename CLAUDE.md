# worktree-manager

`wt` manages git worktrees across a multi-repo workspace. It is a from-scratch
Go rewrite; the old PowerShell line is frozen at the tag `powershell-v0.9.0`.

## Read first

- `openspec/config.yaml`: product context, non-negotiable principles, stack,
  and the rules each OpenSpec artifact follows.
- `docs/design/product.md`: scope and milestone order (M1..M6). It is the
  source of truth for what comes next (§7).
- `CONTRIBUTING.md`: branches, pull request titles, versions, releases, and
  [Working on an OpenSpec change](CONTRIBUTING.md#working-on-an-openspec-change).

## How work flows

Every change to what `wt` does is an OpenSpec change, on its own branch:

```
vX.Y/<change> -> /opsx:propose -> author approves -> /opsx:apply
  -> CHANGELOG + PR -> /opsx:archive (same PR) -> squash merge
```

- Never commit to `main`. Pick the branch prefix from `CONTRIBUTING.md`.
- After `/opsx:propose`, stop and wait for the author to approve the
  artifacts. Do not start `/opsx:apply` on your own.
- Archive in the change's own pull request, before the merge, always syncing
  the specs.
- Before pushing a pull request, `git fetch origin` and check its title with
  `go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<title>"`.

## Checks

The CI runs these on macOS, Windows and Linux; run them on macOS before
closing a task:

```sh
gofmt -l .
go vet ./...
go test -race ./...
openspec validate --all --strict
```

Tasks marked **[Windows]** cannot be closed from macOS alone.

## Language

Code, comments, CLI output, help, specs and `README`/`CONTRIBUTING`/`CHANGELOG`
in English. `proposal.md`, `design.md`, `tasks.md`, `docs/decisions/`,
`docs/design/` and pull request bodies in Spanish.

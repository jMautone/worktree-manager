# Contributing

This repository follows one convention for branches, pull request titles,
versions and releases. The reasoning, and the alternatives that were
discarded, are in
[docs/decisions/0002-versionado-y-releases.md](docs/decisions/0002-versionado-y-releases.md)
(Spanish). The `pr-conventions` check enforces the branch, title and version
rules below.

Changes to what `wt` does go through OpenSpec, step by step, as described in
[Working on an OpenSpec change](#working-on-an-openspec-change).

## Versions

SemVer. Each milestone of [the roadmap](docs/design/product.md) ships as one
minor version:

| Milestone | Version |
|---|---|
| M1 | `0.1.0` |
| M2 | `0.2.0` |
| M3 | `0.3.0` |
| M4 | `0.4.0` |
| M5 | `1.0.0` |
| M6 (optional) | `1.1.0` |

- Every change or fix merged to `main` publishes a pre-release of the **open
  minor**: `v0.1.0-alpha.1`, `v0.1.0-alpha.2`, and so on.
- Closing a milestone publishes its final version, such as `v0.1.0`.
- The open minor is the minor of the latest published alpha. After a final,
  it is the next one in the table.
- Milestones do not overlap: a change of M2 cannot merge until `v0.1.0` is
  published.
- Patch releases (`v0.1.1`) come only from a `v0.1.x` maintenance branch,
  created when needed. They are not automated yet.

## Branches

| Branch | For | Publishes |
|---|---|---|
| `vX.Y/<change>` | An OpenSpec change. `<change>` is its directory in `openspec/changes/`; `X.Y` is the minor of its milestone. | alpha |
| `fix/<slug>` | A bug fix outside a change | alpha |
| `release/vX.Y.0` | Closing a milestone | final |
| `chore/`, `docs/`, `ci/`, `refactor/`, `test/` + `<slug>` | Everything else | nothing |
| `dependabot/**` | Dependabot | nothing |
| `vX.Y.x` | Maintenance lines (protected) | patch |

Change names and slugs are lowercase kebab-case. Work branches are deleted
when their pull request merges.

## Pull request titles

The title becomes the commit on `main`, so it follows
[Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>)[!]: <summary> [<version>]
```

- **type**: `feat`, `fix`, `docs`, `chore`, `ci`, `refactor`, `test` or
  `perf`.
- **scope**: required, kebab-case. On a `vX.Y/<change>` branch it is the
  change name.
- **summary**: imperative, starts in lowercase, no final period. The title
  without the version suffix is at most 72 characters.
- **`!`**: the change breaks the CLI contract (exit codes, `--json` schemas,
  flags).
- **`[<version>]`**: the exact version the merge publishes. The release
  workflow tags whatever this says, so the suffix is required on branches
  that publish and forbidden on the rest.

| Branch | type | scope | Version suffix |
|---|---|---|---|
| `vX.Y/<change>` | any | `<change>` | the next alpha, `vX.Y.0-alpha.N` |
| `fix/<slug>` | `fix` | any | the next alpha, `vX.Y.0-alpha.N` |
| `release/vX.Y.0` | `chore` | `release` | `vX.Y.0` |
| `chore/`, `docs/`, `ci/`, `refactor/`, `test/` | same as the prefix | any | none |
| `dependabot/**` | any | any | none |

Examples:

```
feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]
fix(list): sort branch names case-insensitively [v0.1.0-alpha.3]
chore(release): close M1 [v0.1.0]
chore(ci): bump actions/checkout from 4 to 7
ci(release): add branch, title and release conventions
```

If another pull request publishes first, `main` requires yours to be up to
date: update the branch, and the check tells you the alpha to use instead.

To check a title before pushing, fetch first: the check reads the versions
already published from `origin/main`, and a stale copy suggests an alpha that
is already taken.

```sh
git fetch origin
go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<title>"
```

## Merging

Squash only. The commit on `main` is the title plus ` (#N)`, with no body;
the details stay in the pull request, which follows
[the template](.github/pull_request_template.md). Commits inside a branch are
free-form, since squash discards them; Conventional Commits are recommended.

## Releases

Merging a title with a version runs `.github/workflows/release.yml`. It tags
the squash commit and publishes a GitHub Release with GoReleaser:

- archives for `darwin/arm64`, `darwin/amd64`, `windows/amd64`,
  `linux/amd64` and `linux/arm64`, plus `checksums.txt`, all built with the
  version injected into `wt version`;
- an alpha is marked as a pre-release, with notes grouped from the titles
  merged since the previous version;
- a final is marked as latest, with its `CHANGELOG.md` section as the notes.

The targets live in `.goreleaser.yaml`. The `cross-build` CI job builds them
on every pull request.

### Changelog

Every pull request that publishes adds its lines under `## [Unreleased]` in
`CHANGELOG.md`, following
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

### Closing a milestone

1. Check the milestone's definition of done in `docs/design/product.md` §7.
2. Branch `release/vX.Y.0` from `main`.
3. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.0] — YYYY-MM-DD`
   and add a new, empty `## [Unreleased]` above it.
4. Open the pull request titled `chore(release): close MN [vX.Y.0]`.
5. Squash-merge it. The release notes come from the new changelog section.

## Working on an OpenSpec change

`wt` is developed with [OpenSpec](https://openspec.dev).
`openspec/changes/<change>/` holds a change in flight; `openspec/specs/`
describes what `main` does today, and nothing more. `openspec/config.yaml`
holds the product context and the rules every artifact follows; the
`openspec` CLI and the `/opsx:*` commands in `.claude/` feed both to the
agent that writes the artifacts.

### Choosing the change

Changes come from the milestone breakdown in
[docs/design/product.md](docs/design/product.md) §7, in the order of its
table, and only from the open milestone (see [Versions](#versions)). A
milestone is broken into changes only when it opens: if §7 has no table for
it yet, add one first, in a `docs/` pull request.

### Steps

1. **Branch** from an up-to-date `main`. The branch name is the change name,
   under the minor of its milestone:

   ```sh
   git switch main && git pull
   git switch -c vX.Y/<change>
   ```

2. **Explore** (optional). `/opsx:explore` works through open questions
   before anything is written. Use it when the design has real unknowns.

3. **Propose.** `/opsx:propose <change>` writes `proposal.md`, `design.md`,
   `specs/<capability>/spec.md` and `tasks.md` under
   `openspec/changes/<change>/`. Check them with
   `openspec validate <change> --strict` and commit them on the branch.
   **Stop here** until the artifacts are approved: no code is written before
   that. Revise them with `/opsx:update`.

4. **Apply.** `/opsx:apply <change>` works through `tasks.md` in order:
   - each task brings its test, and is ticked (`- [x]`) only once that test
     passes and `go test ./...` is green on macOS;
   - a task marked **[Windows]** is closed by the `windows-latest` CI job or
     by hand on Windows, never from macOS alone; until then it stays open,
     with a note of what is pending;
   - a new dependency is recorded in `design.md` before it is added;
   - if the plan turns out to be wrong, fix the artifacts with
     `/opsx:update` first, so code and artifacts never disagree.

5. **Open the pull request.** The last task adds the change's lines under
   `## [Unreleased]` in `CHANGELOG.md` and opens the pull request, titled
   `<type>(<change>): <summary> [vX.Y.0-alpha.N]` (see
   [Pull request titles](#pull-request-titles)).

6. **Archive, in the same pull request.** Once every task is ticked and CI
   is green, run `/opsx:archive <change>` and choose to sync the specs. It
   merges the change's delta specs into `openspec/specs/` and moves the change
   to `openspec/changes/archive/YYYY-MM-DD-<change>/`. Read the synced specs
   against the code: they must describe what the code does, not what the
   proposal planned. Run `openspec validate --all --strict`, then commit and
   push.

7. **Merge.** Squash-merge. The release workflow publishes the alpha, and
   the branch is deleted.

Archiving before the merge keeps `openspec/specs/` true at every published
version: no alpha ships code whose specs are still pending, and no follow-up
pull request is needed. `pr-conventions` finds the change in either place.

### Languages

| What | Language |
|---|---|
| `proposal.md`, `design.md`, `tasks.md` | Spanish |
| Specs, both delta and in `openspec/specs/` | English |
| Code, comments, CLI output, help, `README.md`, `CONTRIBUTING.md`, `CHANGELOG.md` | English |
| `docs/decisions/`, `docs/design/` | Spanish |
| Pull request title / body | English / Spanish |

### Work outside a change

- A `fix/<slug>` makes the code do what `openspec/specs/` already says, and
  does not touch the specs. If the specified behavior itself has to change,
  that is a change: open one.
- `chore/`, `docs/`, `ci/`, `refactor/` and `test/` branches do not change
  what `wt` does, and do not touch `openspec/specs/`.

# Contributing

This repository follows one convention for branches, pull request titles,
versions and releases. The reasoning, and the alternatives that were
discarded, are in
[docs/decisions/0002-versionado-y-releases.md](docs/decisions/0002-versionado-y-releases.md)
(Spanish). The `pr-conventions` check enforces everything below.

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

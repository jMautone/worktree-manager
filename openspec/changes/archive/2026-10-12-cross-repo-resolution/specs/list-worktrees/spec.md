## MODIFIED Requirements

### Requirement: Listing the current repository
`wt list` SHALL list every worktree of the repository associated with the working directory (see the `git-worktrees` capability), including the main worktree, and SHALL exit 0. It SHALL accept no positional arguments. Outside a repository it SHALL fail as that capability specifies (exit 3) and, when `repos_root` names at least one root, with a hint naming `wt list --all-repos`.

#### Scenario: Repository with linked worktrees
- **WHEN** a repository has its main worktree and two linked worktrees and the user runs `wt list` inside any of them
- **THEN** all three worktrees are listed
- **AND** the exit code is 0

#### Scenario: Outside a repository with roots configured
- **WHEN** `repos_root` names at least one root and the user runs `wt list` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** a `hint:` line names `wt list --all-repos`
- **AND** the exit code is 3

## ADDED Requirements

### Requirement: Listing every repository
`wt list --all-repos` SHALL list, in a single table, every worktree of every repository found under the roots (see the `workspace-discovery` capability), including each main worktree, from any working directory, and SHALL exit 0. The repositories SHALL be the ones `wt repos` lists: a repository outside the roots SHALL NOT be listed, even when the working directory is inside it. The table SHALL have the columns of `wt list` with a `REPO` column after the markers:

| Column | Content |
|---|---|
| (markers) | `@` if the worktree is current, `^` if it is the main one of its repository; both when both apply; blank otherwise |
| `REPO` | the display name of the worktree's repository (see the `cross-repo-resolution` capability) |
| `NAME`, `BRANCH`, `HEAD`, `STATE`, `PATH` | as in `wt list` |

Columns SHALL be aligned by their visible width, whether or not color is enabled. The repositories SHALL be in the order of `wt repos`, and the worktrees of each repository in the order of `wt list`. A repository that git cannot read SHALL be skipped as the `cross-repo-resolution` capability specifies. When `repos_root` names no root, the command SHALL fail as `wt repos` does (exit 1, `no repository roots configured`). When no repository is found, stdout SHALL be empty and the exit code SHALL be 0.

#### Scenario: Two repositories
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` has the linked worktree `feat`, `<root>/web` has no linked worktree, and the user runs `wt list --all-repos` outside any repository
- **THEN** the rows are the main worktree of `api`, `feat`, and the main worktree of `web`, in that order
- **AND** their `REPO` column reads `api`, `api` and `web`
- **AND** the rows of both main worktrees carry `^`
- **AND** no row carries `@`
- **AND** the exit code is 0

#### Scenario: Current worktree
- **WHEN** the user runs `wt list --all-repos` from inside the linked worktree `feat` of `<root>/api`
- **THEN** the row of `feat` starts with `@`

#### Scenario: Repositories with the same name
- **WHEN** `repos_depth` is `2` and `<root>/work/api` and `<root>/oss/api` are repositories
- **THEN** the `REPO` column of `wt list --all-repos` reads `oss/api` for the first and `work/api` for the second

#### Scenario: Repository outside the roots
- **WHEN** the working directory is inside a repository that is not under any root and the user runs `wt list --all-repos`
- **THEN** none of its worktrees is listed

#### Scenario: Repository that git cannot read
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` is a repository, and the user runs `wt list --all-repos`
- **THEN** the main worktree of `api` is listed
- **AND** no row has `broken` in its `REPO` column
- **AND** stderr contains a warning naming `broken`
- **AND** the exit code is 0

#### Scenario: No roots configured
- **WHEN** no configuration sets `repos_root` and the user runs `wt list --all-repos`
- **THEN** stderr's first line is `wt: no repository roots configured`
- **AND** the exit code is 1

#### Scenario: No repositories found
- **WHEN** `repos_root` names an existing empty directory and the user runs `wt list --all-repos`
- **THEN** stdout is empty
- **AND** the exit code is 0

### Requirement: JSON output of every repository
`wt list --all-repos --json` SHALL print a `wt.list.v1` document whose `worktrees` array is in the order of the table. Each element SHALL have the fields that "JSON output" specifies plus `repo`, the repository's name as `wt repos` lists it, and `repo_path`, the repository's path as `wt repos` lists it. The document SHALL also have an `unreadable` array with one element per repository that git could not read, each with `repo` and `path` (as above) and `message` (git's error message), empty when every repository was read. Without `--all-repos`, `wt list --json` SHALL NOT include `repo`, `repo_path` or `unreadable`.

#### Scenario: Every repository as JSON
- **WHEN** `<root>/api` has the linked worktree `feat` and the user runs `wt list --all-repos --json`
- **THEN** stdout is a JSON object with `schema` `wt.list.v1`
- **AND** the element of `feat` has `repo` `api` and `repo_path` `<root>/api`
- **AND** `unreadable` is an empty array

#### Scenario: Repository that git cannot read, as JSON
- **WHEN** git cannot read the worktrees of `<root>/broken` and the user runs `wt list --all-repos --json`
- **THEN** stderr is empty
- **AND** `unreadable` has one element with `repo` `broken`, `path` `<root>/broken`, and git's message in `message`
- **AND** the exit code is 0

#### Scenario: Current repository as JSON
- **WHEN** the user runs `wt list --json` inside a repository
- **THEN** no element has a `repo` field
- **AND** the document has no `unreadable` field

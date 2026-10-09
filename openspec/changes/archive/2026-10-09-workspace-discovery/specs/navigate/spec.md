## MODIFIED Requirements

### Requirement: Targets
`wt cd` SHALL take exactly one argument, `<target>`:

| Target | Destination |
|---|---|
| `^` | the main worktree |
| `@` | the root of the current worktree |
| `-` | the previous directory of the shell session |
| anything else | the worktree of the current repository with that name or branch or, when there is none, the repository with that name under the configured roots (see "Resolving a name") |

A missing `<target>` or an extra argument SHALL be a usage error (exit 2).

#### Scenario: No target
- **WHEN** the user runs `wt cd`
- **THEN** stderr reports the missing `<target>` argument
- **AND** the exit code is 2

#### Scenario: No target with roots configured
- **WHEN** `repos_root` names at least one root and the user runs `wt cd`
- **THEN** stderr reports the missing `<target>` argument
- **AND** the shell's working directory does not change
- **AND** the exit code is 2

#### Scenario: Two targets
- **WHEN** the user runs `wt cd feat fix`
- **THEN** stderr reports the unexpected argument `fix`
- **AND** the exit code is 2

### Requirement: Resolving a name
Any target other than `^`, `@` and `-` SHALL be resolved first in the current repository and then, only if that finds nothing, in the workspace.

**Current repository.** When the working directory is inside a repository (see the `git-worktrees` capability), the target SHALL be resolved among that repository's worktrees in two steps:

1. the worktrees whose `NAME` (the last component of the path, as in `wt list`) equals the target;
2. only if step 1 matched none, the worktree whose branch equals the target.

These comparisons SHALL be exact and case-sensitive on every operating system. A single match SHALL be the destination. More than one match in the step that matched SHALL fail with exit code 4, a message stating that the target matches more than one worktree, and one `hint:` line per candidate with its path; the workspace SHALL NOT be searched.

**Workspace.** When the current repository matched nothing, or the working directory is inside no repository, and `repos_root` names at least one root, the target SHALL be resolved among the repositories found under the roots (see the `workspace-discovery` capability) in three steps, each only if the previous one matched none:

1. the repositories whose name equals the target;
2. the repositories whose name equals the target, ignoring letter case;
3. the repositories whose name starts with the target, ignoring letter case.

These rules SHALL be the same on every operating system. An empty target SHALL match no repository. A single match SHALL be the destination: the repository's path, as `wt repos` lists it. More than one match in the step that matched SHALL fail with exit code 4, the message `"<target>" matches more than one repository`, and one `hint:` line per candidate with its path.

**Nothing found.** When neither matched, the command SHALL fail with exit code 3 and:

| Working directory | Roots configured | Message | Hints |
|---|---|---|---|
| inside a repository | no | `no worktree named "<target>"` | run `wt list` |
| inside a repository | yes | `no worktree or repository named "<target>"` | run `wt list`; run `wt repos` |
| inside no repository | yes | `no repository named "<target>"` | run `wt repos` |
| inside no repository | no | `not a git repository: <dir>`, as the `git-worktrees` capability specifies | set `repos_root` to jump to repositories from anywhere |

#### Scenario: By name
- **WHEN** a linked worktree lives at `<parent>/repo.worktrees/feat` and the user runs `wt cd feat`
- **THEN** the destination is that worktree

#### Scenario: By branch
- **WHEN** a linked worktree at `<parent>/repo.worktrees/feature-abc1` has the branch `feature/abc1` checked out and the user runs `wt cd feature/abc1`
- **THEN** the destination is that worktree

#### Scenario: Name before branch
- **WHEN** a worktree named `api` has the branch `fix` checked out, another worktree is named `fix`, and the user runs `wt cd fix`
- **THEN** the destination is the worktree named `fix`

#### Scenario: Ambiguous name
- **WHEN** two worktrees of the repository live at `/a/x` and `/b/x` and the user runs `wt cd x`
- **THEN** stderr's first line states that `x` matches more than one worktree
- **AND** stderr has a `hint:` line with `/a/x` and another with `/b/x`
- **AND** the exit code is 4

#### Scenario: No such worktree
- **WHEN** no configuration sets `repos_root` and the user runs `wt cd nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree named "nope"`
- **AND** the exit code is 3

#### Scenario: Case differs
- **WHEN** no configuration sets `repos_root`, a worktree is named `feat`, and the user runs `wt cd Feat`
- **THEN** the exit code is 3, on every operating system

#### Scenario: Outside a repository
- **WHEN** no configuration sets `repos_root` and the user runs `wt cd feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** a `hint:` line names `repos_root`
- **AND** the exit code is 3

#### Scenario: Repository from inside another
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` and `<root>/web` are repositories, the shell is in a subdirectory of `api`, and the user runs `wt cd web`
- **THEN** the shell's working directory is `<root>/web`
- **AND** the exit code is 0

#### Scenario: Repository from outside any repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/web` is a repository, the shell is in a directory that is in no repository, and the user runs `wt cd web`
- **THEN** the shell's working directory is `<root>/web`

#### Scenario: Current repository first
- **WHEN** `<root>/web` is a repository, the shell is inside the repository `api`, `api` has a linked worktree named `web`, and the user runs `wt cd web`
- **THEN** the destination is the linked worktree `web` of `api`, not `<root>/web`

#### Scenario: Local ambiguity is not resolved in the workspace
- **WHEN** `<root>/x` is a repository, the shell is inside a repository with worktrees at `/a/x` and `/b/x`, and the user runs `wt cd x`
- **THEN** stderr's first line states that `x` matches more than one worktree
- **AND** the exit code is 4

#### Scenario: Exact name before prefix
- **WHEN** `<root>/api` and `<root>/api-gateway` are repositories and the user runs `wt cd api` outside any repository
- **THEN** the destination is `<root>/api`

#### Scenario: Exact case before ignoring case
- **WHEN** `repos_root` is `["<one>", "<two>"]`, `<one>/Api` and `<two>/api` are repositories, and the user runs `wt cd api` outside any repository
- **THEN** the destination is `<two>/api`

#### Scenario: Letter case ignored
- **WHEN** `<root>/TrendFisher` is a repository and the user runs `wt cd trendfisher` outside any repository
- **THEN** the destination is `<root>/TrendFisher`

#### Scenario: Unique prefix
- **WHEN** `<root>/TrendFisher` is the only repository whose name starts with `trend`, ignoring case, and the user runs `wt cd trend`
- **THEN** the destination is `<root>/TrendFisher`

#### Scenario: Ambiguous prefix
- **WHEN** `<root>/cv-scorer-backend` and `<root>/cv-scorer-front` are repositories and the user runs `wt cd cv` outside any repository
- **THEN** stderr's first line is `wt: "cv" matches more than one repository`
- **AND** stderr has a `hint:` line with `<root>/cv-scorer-backend` and another with `<root>/cv-scorer-front`
- **AND** the shell's working directory does not change
- **AND** the exit code is 4

#### Scenario: Two repositories with the same name
- **WHEN** `repos_depth` is `2`, `<root>/work/api` and `<root>/oss/api` are repositories, and the user runs `wt cd api` outside any repository
- **THEN** stderr's first line is `wt: "api" matches more than one repository`
- **AND** the exit code is 4

#### Scenario: Ambiguity as JSON
- **WHEN** the user runs `wt cd cv --json` with the repositories of "Ambiguous prefix"
- **THEN** stdout is empty
- **AND** stderr parses as a JSON object with `schema` `wt.error.v1`, `code` 4, and a `hints` array with both paths

#### Scenario: No worktree or repository
- **WHEN** `repos_root` names at least one root, no repository under it starts with `nope`, and the user runs `wt cd nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree or repository named "nope"`
- **AND** stderr has a `hint:` line naming `wt list` and another naming `wt repos`
- **AND** the exit code is 3

#### Scenario: No repository
- **WHEN** `repos_root` names at least one root, no repository under it starts with `nope`, and the user runs `wt cd nope` outside any repository
- **THEN** stderr's first line is `wt: no repository named "nope"`
- **AND** a `hint:` line names `wt repos`
- **AND** the exit code is 3

### Requirement: Main and current worktree
`^` SHALL resolve to the main worktree of the repository associated with the working directory; for a bare repository, that is the bare repository itself. `@` SHALL resolve to the root of the current worktree; when the working directory is inside a repository but no worktree is current, `wt cd @` SHALL fail with exit code 3 and the message `not inside a worktree`. Outside a repository, both SHALL fail as the `git-worktrees` capability specifies (exit 3), whether or not `repos_root` names any root: neither is ever resolved in the workspace.

#### Scenario: Back to main
- **WHEN** the shell is in a subdirectory of a linked worktree and the user runs `wt cd ^`
- **THEN** the shell's working directory is the main worktree's path

#### Scenario: Root of the current worktree
- **WHEN** the shell is in `<feat>/src/pkg`, inside the worktree `feat`, and the user runs `wt cd @`
- **THEN** the shell's working directory is the path of `feat`

#### Scenario: Main worktree outside a repository
- **WHEN** `repos_root` names at least one root and the user runs `wt cd ^` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

# navigate Specification

## Purpose

Defines `wt cd`, the command that moves the shell to a worktree of the current repository by its name or branch, to a repository under the configured roots by its name, to a linked worktree of another of those repositories, to a worktree named in the qualified form `<repo>:<name>`, to the main worktree, to the root of the current worktree, or back to the directory it was in before the last jump; and `wt path`, which prints where `wt cd` would go.

## Requirements

### Requirement: Targets
`wt cd` SHALL take exactly one argument, `<target>`:

| Target | Destination |
|---|---|
| `^` | the main worktree |
| `@` | the root of the current worktree |
| `-` | the previous directory of the shell session |
| `<repo>:<name>` | the worktree `<name>` of the repository `<repo>`, or the repository itself when `<name>` is empty (see the `cross-repo-resolution` capability) |
| anything else | the worktree of the current repository with that name or branch; when there is none, the repository with that name under the configured roots; when there is none, the linked worktree of another repository with that name or branch (see "Resolving a name") |

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

### Requirement: Shell integration required
`wt cd` SHALL fail with exit code 1 when the shell integration is not active (see the `shell-integration` capability), before resolving its target: stdout SHALL be empty, the first line of stderr SHALL be `wt: shell integration is not active`, and a hint SHALL name `wt shell init`. This applies with and without `--json` and `--dry-run`.

#### Scenario: Run without the function
- **WHEN** the user runs `git-wt cd feat` in a shell that did not load the script
- **THEN** stderr's first line is `wt: shell integration is not active`
- **AND** a `hint:` line names `wt shell init`
- **AND** stdout is empty
- **AND** the exit code is 1

#### Scenario: Unknown target without the function
- **WHEN** the user runs `git-wt cd does-not-exist` in a shell that did not load the script
- **THEN** the exit code is 1

### Requirement: Moving the shell
When the target resolves to an existing directory, `wt cd` SHALL write that directory as the directive (see the `shell-integration` capability), SHALL print nothing to stdout, and SHALL exit 0. Through the function, the shell ends in that directory.

#### Scenario: Jump to a worktree
- **WHEN** the shell is in a subdirectory of the main worktree and the user runs `wt cd feat`, where `feat` is a linked worktree
- **THEN** the shell's working directory is the path of `feat`
- **AND** stdout is empty
- **AND** the exit code is 0

### Requirement: Resolving a name
A target that contains `:` SHALL be resolved as a qualified target (see the `cross-repo-resolution` capability), and never as this requirement describes. Any other target than `^`, `@` and `-` SHALL be resolved first in the current repository, then among the repositories under the roots, and then among the linked worktrees of the other repositories, each only if the previous one found nothing.

**Current repository.** When the working directory is inside a repository (see the `git-worktrees` capability), the target SHALL be resolved among that repository's worktrees in two steps:

1. the worktrees whose `NAME` (the last component of the path, as in `wt list`) equals the target;
2. only if step 1 matched none, the worktree whose branch equals the target.

These comparisons SHALL be exact and case-sensitive on every operating system. A single match SHALL be the destination. More than one match in the step that matched SHALL fail with exit code 4, a message stating that the target matches more than one worktree, and one `hint:` line per candidate with its path; neither the repositories nor the worktrees of other repositories SHALL be searched.

**Repositories.** When the current repository matched nothing, or the working directory is inside no repository, and `repos_root` names at least one root, the target SHALL be resolved among the repositories found under the roots (see the `workspace-discovery` capability) in three steps, each only if the previous one matched none:

1. the repositories whose name equals the target;
2. the repositories whose name equals the target, ignoring letter case;
3. the repositories whose name starts with the target, ignoring letter case.

These rules SHALL be the same on every operating system. An empty target SHALL match no repository. A single match SHALL be the destination: the repository's path, as `wt repos` lists it. More than one match in the step that matched SHALL fail with exit code 4, the message `"<target>" matches more than one repository`, and one `hint:` line per candidate containing its display name followed by `:` (see the `cross-repo-resolution` capability) and its path; the worktrees of other repositories SHALL NOT be searched.

**Worktrees of other repositories.** When neither the current repository nor the repositories matched, and `repos_root` names at least one root, the target SHALL be resolved among the linked worktrees of the other repositories, as the `cross-repo-resolution` capability specifies, with its exit 4 when more than one matches.

**Nothing found.** When none matched, the command SHALL fail with exit code 3 and:

| Working directory | Roots configured | Message | Hints |
|---|---|---|---|
| inside a repository | no | `no worktree named "<target>"` | run `wt list` |
| inside a repository | yes | `no worktree or repository named "<target>"` | run `wt list --all-repos`; run `wt repos` |
| inside no repository | yes | `no worktree or repository named "<target>"` | run `wt list --all-repos`; run `wt repos` |
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
- **AND** stderr has a `hint:` line containing `cv-scorer-backend:` and `<root>/cv-scorer-backend`, and another containing `cv-scorer-front:` and `<root>/cv-scorer-front`
- **AND** the shell's working directory does not change
- **AND** the exit code is 4

#### Scenario: Two repositories with the same name
- **WHEN** `repos_depth` is `2`, `<root>/work/api` and `<root>/oss/api` are repositories, and the user runs `wt cd api` outside any repository
- **THEN** stderr's first line is `wt: "api" matches more than one repository`
- **AND** stderr has a `hint:` line containing `oss/api:` and another containing `work/api:`
- **AND** the exit code is 4

#### Scenario: Ambiguity as JSON
- **WHEN** the user runs `wt cd cv --json` with the repositories of "Ambiguous prefix"
- **THEN** stdout is empty
- **AND** stderr parses as a JSON object with `schema` `wt.error.v1`, `code` 4, and a `hints` array with both paths

#### Scenario: Worktree of another repository
- **WHEN** `<root>/api` has the linked worktree `login`, the shell is inside `<root>/web`, which has no worktree or branch named `login`, no repository is named `login` or starts with it, and the user runs `wt cd login`
- **THEN** the shell's working directory is the path of the worktree `login` of `api`
- **AND** the exit code is 0

#### Scenario: Repository before worktrees of other repositories
- **WHEN** `<root>/feature-flags` is a repository, `<root>/api` has the linked worktree `feat`, and from inside `<root>/web` the user runs `wt cd feat`
- **THEN** the destination is `<root>/feature-flags`

#### Scenario: Ambiguous repositories stop the search
- **WHEN** `<root>/cv-scorer-backend` and `<root>/cv-scorer-front` are repositories, `<root>/api` has the linked worktree `cv`, and from outside any repository the user runs `wt cd cv`
- **THEN** stderr's first line is `wt: "cv" matches more than one repository`
- **AND** the exit code is 4

#### Scenario: Qualified target
- **WHEN** `<root>/api` has the linked worktree `feat`, the shell is inside `<root>/web`, and the user runs `wt cd api:feat`
- **THEN** the shell's working directory is the path of the worktree `feat` of `api`

#### Scenario: No worktree or repository
- **WHEN** `repos_root` names at least one root, no repository under it starts with `nope`, no linked worktree of another repository has the name or branch `nope`, and the user runs `wt cd nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree or repository named "nope"`
- **AND** stderr has a `hint:` line naming `wt list --all-repos` and another naming `wt repos`
- **AND** the exit code is 3

#### Scenario: No repository
- **WHEN** `repos_root` names at least one root, no repository under it starts with `nope`, no linked worktree of any repository has the name or branch `nope`, and the user runs `wt cd nope` outside any repository
- **THEN** stderr's first line is `wt: no worktree or repository named "nope"`
- **AND** stderr has a `hint:` line naming `wt list --all-repos` and another naming `wt repos`
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

### Requirement: Previous directory
`-` SHALL resolve to the directory in `WT_PREVIOUS_DIR` (see the `shell-integration` capability) and SHALL NOT require a repository. When that variable is unset or empty, `wt cd -` SHALL fail with exit code 3 and the message `no previous directory`.

#### Scenario: Previous directory outside a repository
- **WHEN** the user ran `wt cd feat` from a directory outside any repository, and then runs `wt cd -`
- **THEN** the shell is back in that directory
- **AND** the exit code is 0

#### Scenario: No previous directory
- **WHEN** the user runs `wt cd -` in a shell session where the function has not changed directory
- **THEN** stderr's first line is `wt: no previous directory`
- **AND** the exit code is 3

### Requirement: Destination must exist
When the destination directory does not exist, such as a prunable worktree or a previous directory that was deleted, `wt cd` SHALL fail with exit code 3, a message naming the path, and SHALL NOT write the directive.

#### Scenario: Prunable worktree
- **WHEN** the directory of the linked worktree `feat` was deleted without `git worktree remove`, and the user runs `wt cd feat`
- **THEN** stderr names the path of `feat`
- **AND** the shell's working directory does not change
- **AND** the exit code is 3

### Requirement: Working directory override
With `-C <dir>`, `wt cd` SHALL resolve names, `^` and `@` as if it had been started in `<dir>`. `-C` SHALL NOT change what `-` resolves to.

#### Scenario: Main worktree of another repository
- **WHEN** the shell is in repository `a` and the user runs `wt -C <path of repository b> cd ^`
- **THEN** the shell's working directory is the main worktree of `b`

### Requirement: JSON output
`wt cd <target> --json` SHALL move the shell like `wt cd <target>` and SHALL print `{"schema":"wt.cd.v1","path":<destination>}`, where `path` is the destination's absolute path in the operating system's native form.

#### Scenario: Destination as JSON (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt cd feat --json` for a worktree at `/src/repo.worktrees/feat`
- **THEN** stdout is a JSON object with `schema` `wt.cd.v1` and `path` `/src/repo.worktrees/feat`

#### Scenario: Destination as JSON (Windows)
- **WHEN** on Windows the user runs `wt cd feat --json` for a worktree at `C:\src\repo.worktrees\feat`
- **THEN** `path` is `C:\src\repo.worktrees\feat`

### Requirement: Dry run
`wt cd --dry-run <target>` SHALL perform every check of `wt cd`, with the same errors and exit codes, SHALL NOT write the directive, and on success SHALL print `would change directory to <destination>` to stdout and exit 0. With `--json` it SHALL print the same `wt.cd.v1` document as without `--dry-run`.

#### Scenario: Preview a jump
- **WHEN** the user runs `wt cd --dry-run feat` through the function
- **THEN** stdout is `would change directory to <path of feat>`
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

### Requirement: Printing a path
`wt path` SHALL take exactly one argument, `<target>`, and SHALL resolve it as `wt cd` does (see "Targets", "Resolving a name", "Main and current worktree", "Destination must exist" and "Working directory override"), with the same errors and exit codes, except that `-` SHALL NOT be special: it SHALL be resolved as a name. On success it SHALL print the destination's absolute path in the operating system's native form, followed by a newline, to stdout, SHALL write no directive, and SHALL exit 0. It SHALL behave the same whether or not the shell integration is active. A missing `<target>` or an extra argument SHALL be a usage error (exit 2).

#### Scenario: Path of a worktree
- **WHEN** the linked worktree `feat` lives at `/src/repo.worktrees/feat` and the user runs `wt path feat` inside the repository
- **THEN** stdout is `/src/repo.worktrees/feat` followed by a newline
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Path of a repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/web` is a repository, and the user runs `wt path web` outside any repository
- **THEN** stdout is `<root>/web` followed by a newline

#### Scenario: Path of a worktree of another repository
- **WHEN** `<root>/api` has the linked worktree `feat` and from inside `<root>/web` the user runs `wt path api:feat`
- **THEN** stdout is the path of the worktree `feat` of `api` followed by a newline

#### Scenario: Native path on Windows
- **WHEN** on Windows the linked worktree `feat` lives at `C:\src\repo.worktrees\feat` and the user runs `wt path feat`
- **THEN** stdout is `C:\src\repo.worktrees\feat` followed by a newline

#### Scenario: Without the function
- **WHEN** the user runs `git-wt path feat` in a shell that did not load the script, inside a repository with the linked worktree `feat`
- **THEN** stdout is the path of `feat`
- **AND** the exit code is 0

#### Scenario: Dash is a name
- **WHEN** no configuration sets `repos_root`, the repository has no worktree or branch named `-`, and the user runs `wt path -`
- **THEN** stderr's first line is `wt: no worktree named "-"`
- **AND** the exit code is 3

#### Scenario: Directory gone
- **WHEN** the directory of the linked worktree `feat` was deleted without `git worktree remove`, and the user runs `wt path feat`
- **THEN** stdout is empty
- **AND** stderr names the path of `feat`
- **AND** the exit code is 3

#### Scenario: No target
- **WHEN** the user runs `wt path`
- **THEN** stderr reports the missing `<target>` argument
- **AND** the exit code is 2

### Requirement: JSON output and dry run of path
`wt path <target> --json` SHALL print `{"schema":"wt.path.v1","path":<destination>}` instead of the path line, where `path` is the destination's absolute path in the operating system's native form. `wt path` never writes, so `--dry-run` SHALL NOT change its output.

#### Scenario: Path as JSON
- **WHEN** the linked worktree `feat` lives at `/src/repo.worktrees/feat` and the user runs `wt path feat --json`
- **THEN** stdout is a JSON object with `schema` `wt.path.v1` and `path` `/src/repo.worktrees/feat`

#### Scenario: Dry run
- **WHEN** the user runs `wt path feat --dry-run` inside a repository with the linked worktree `feat`
- **THEN** stdout is the path of `feat` followed by a newline

# Spec Delta

## Purpose

Defines `wt cd`, the command that moves the shell to a worktree of the current repository by its name or branch, to the main worktree, to the root of the current worktree, or back to the directory it was in before the last jump.

## ADDED Requirements

### Requirement: Targets
`wt cd` SHALL take exactly one argument, `<target>`:

| Target | Destination |
|---|---|
| `^` | the main worktree |
| `@` | the root of the current worktree |
| `-` | the previous directory of the shell session |
| anything else | the worktree with that name or branch |

A missing `<target>` or an extra argument SHALL be a usage error (exit 2).

#### Scenario: No target
- **WHEN** the user runs `wt cd`
- **THEN** stderr reports the missing `<target>` argument
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
Any target other than `^`, `@` and `-` SHALL be resolved among the worktrees of the repository associated with the working directory (see the `git-worktrees` capability), in two steps:

1. the worktrees whose `NAME` (the last component of the path, as in `wt list`) equals the target;
2. only if step 1 matched none, the worktree whose branch equals the target.

Comparisons SHALL be exact and case-sensitive on every operating system. A single match SHALL be the destination. More than one match in the step that matched SHALL fail with exit code 4, a message stating that the target matches more than one worktree, and one `hint:` line per candidate with its path. No match SHALL fail with exit code 3 and the message `no worktree named "<target>"`, with a hint to run `wt list`. Outside a repository, the command SHALL fail as the `git-worktrees` capability specifies (exit 3).

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
- **WHEN** the user runs `wt cd nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree named "nope"`
- **AND** the exit code is 3

#### Scenario: Case differs
- **WHEN** a worktree is named `feat` and the user runs `wt cd Feat`
- **THEN** the exit code is 3, on every operating system

#### Scenario: Outside a repository
- **WHEN** the user runs `wt cd feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

### Requirement: Main and current worktree
`^` SHALL resolve to the main worktree of the repository associated with the working directory; for a bare repository, that is the bare repository itself. `@` SHALL resolve to the root of the current worktree; when the working directory is inside a repository but no worktree is current, `wt cd @` SHALL fail with exit code 3 and the message `not inside a worktree`. Outside a repository, both SHALL fail as the `git-worktrees` capability specifies (exit 3).

#### Scenario: Back to main
- **WHEN** the shell is in a subdirectory of a linked worktree and the user runs `wt cd ^`
- **THEN** the shell's working directory is the main worktree's path

#### Scenario: Root of the current worktree
- **WHEN** the shell is in `<feat>/src/pkg`, inside the worktree `feat`, and the user runs `wt cd @`
- **THEN** the shell's working directory is the path of `feat`

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

# git-worktrees Specification

## Purpose

Defines how `wt` finds the repository that contains a directory, enumerates that repository's worktrees as git records them, and reports each worktree's state and path consistently across operating systems.

## Requirements

### Requirement: Git is required
`wt` SHALL operate git by running the `git` executable found on the PATH. When no `git` executable is found, any command that needs git SHALL fail with exit code 1 and a message stating that git was not found.

#### Scenario: Git missing
- **WHEN** `git` is not on the PATH and the user runs `wt list`
- **THEN** stderr states that git was not found
- **AND** the exit code is 1

### Requirement: Repository discovery
The repository SHALL be the one git associates with the working directory: the directory may be anywhere inside the main worktree, inside a linked worktree, or inside a bare repository. When git associates no repository with the working directory, commands that need one SHALL fail with exit code 3 and the message `not a git repository: <dir>`.

#### Scenario: From a linked worktree
- **WHEN** the working directory is a subdirectory of a linked worktree
- **THEN** the repository is the one that worktree belongs to, and all of its worktrees are visible

#### Scenario: From a bare repository
- **WHEN** the working directory is a bare repository that has linked worktrees
- **THEN** the repository is that bare repository, and its linked worktrees are visible

#### Scenario: Outside any repository
- **WHEN** the working directory is not inside any repository
- **THEN** the command fails with exit code 3 and `not a git repository: <dir>`

### Requirement: Worktree enumeration
`wt` SHALL report every worktree git has registered for the repository, including worktrees whose directory no longer exists. The first worktree git reports SHALL be marked as the main one; for a bare repository, that is the bare repository itself.

#### Scenario: Worktree deleted from disk
- **WHEN** a linked worktree's directory was deleted without `git worktree remove`
- **THEN** that worktree is still reported, marked as prunable

### Requirement: Worktree state
For each worktree `wt` SHALL report:
- its absolute path;
- the full commit id of its HEAD, or none for a bare repository;
- its branch as a short name (for example `feature/abc1`, never `refs/heads/feature/abc1`), or none when HEAD is detached or the entry is a bare repository;
- whether HEAD is detached;
- whether the entry is a bare repository;
- whether it is locked, and the lock reason when one was given;
- whether it is prunable, and git's reason;
- whether it is the main worktree;
- whether it is the current worktree.

#### Scenario: Branch with a slash
- **WHEN** a worktree has `feature/abc1` checked out
- **THEN** its branch is reported as `feature/abc1`

#### Scenario: Locked with a reason
- **WHEN** a worktree was locked with `git worktree lock --reason "on usb drive"`
- **THEN** it is reported as locked with reason `on usb drive`

#### Scenario: Locked without a reason
- **WHEN** a worktree was locked with no reason
- **THEN** it is reported as locked with no reason

#### Scenario: Detached HEAD
- **WHEN** a worktree was created with `--detach`
- **THEN** it is reported as detached, with no branch and with its HEAD commit id

### Requirement: Native paths
Worktree paths SHALL be reported in the operating system's native form: with `/` separators on macOS and Linux, and with `\` separators and a drive letter on Windows, even though git itself reports `/` on every platform.

#### Scenario: Windows separators
- **WHEN** a worktree lives at `C:\src\repo` on Windows
- **THEN** its path is reported as `C:\src\repo`, not `C:/src/repo`

### Requirement: Current worktree
The current worktree SHALL be the one whose path contains the working directory, compared after resolving symbolic links in both. When worktree paths are nested, the deepest match SHALL win. The comparison SHALL be case-insensitive on macOS and Windows and case-sensitive on Linux. When the working directory is inside no worktree path, no worktree SHALL be current.

#### Scenario: Symbolic link in the working directory (macOS)
- **WHEN** on macOS the working directory is given as `/var/folders/.../repo` and git reports the worktree as `/private/var/folders/.../repo`
- **THEN** that worktree is the current one

#### Scenario: Nested worktree
- **WHEN** a linked worktree lives at `<main>/.worktrees/x` and the working directory is inside it
- **THEN** the linked worktree is current, not the main one

#### Scenario: Case differs (macOS, Windows)
- **WHEN** on macOS or Windows the working directory is spelled with different letter case than the worktree path
- **THEN** that worktree is the current one

### Requirement: Git failures
When git fails for a reason other than the absence of a repository, the command SHALL fail with exit code 1 and SHALL include git's own error message.

#### Scenario: Corrupt repository
- **WHEN** git fails to read the worktree list because the repository is corrupt
- **THEN** stderr contains git's error message
- **AND** the exit code is 1

### Requirement: Default branch
Commands that need the repository's default branch, such as `wt create` when neither `--base` nor `default_base` is given, SHALL use:

1. the remote-tracking branch that `origin/HEAD` points to, written `origin/<branch>`, when `origin/HEAD` exists;
2. otherwise, the local branch that the main worktree's HEAD points to (for a bare repository, the bare repository's HEAD);
3. otherwise, when the main worktree's HEAD is detached, the repository has no default branch.

#### Scenario: Clone
- **WHEN** the repository was cloned, `origin/HEAD` points to `origin/main`, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from origin/main`

#### Scenario: No remote
- **WHEN** the repository has no remote, its main worktree has the branch `trunk` checked out, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from trunk`

#### Scenario: Bare repository
- **WHEN** the working directory is a bare repository without `origin/HEAD` whose HEAD points to `main`, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from main`

#### Scenario: Detached main worktree
- **WHEN** the repository has no `origin/HEAD`, its main worktree has a detached HEAD, and the user runs `wt create feat` with `default_base` empty
- **THEN** stderr states that there is no default branch
- **AND** the exit code is 3

### Requirement: Merged branch
Commands that need to know whether a branch is merged, such as `wt remove` when it decides whether to delete a worktree's branch, SHALL consider a branch merged when its tip commit is the commit of, or an ancestor of, either:

1. the base: the value of `default_base` when it is not empty, otherwise the repository's default branch (see "Default branch"); or
2. the branch's upstream, when it has one.

This SHALL be judged against the references already present in the repository: nothing is fetched. A branch whose changes reached the base through other commits, such as a squash merge, a rebase or a cherry-pick, SHALL NOT count as merged. When `default_base` is empty and the repository has no default branch, or when the base does not resolve to a commit, only the upstream SHALL count, and the command SHALL write a warning stating why the base could not be used.

#### Scenario: No commits of its own
- **WHEN** the branch `feat` points to the same commit as `origin/main`, `origin/HEAD` points to `origin/main`, `default_base` is empty, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Merged into the base
- **WHEN** the tip of the branch `feat` is an ancestor of `origin/main`, `origin/HEAD` points to `origin/main`, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Commit not in the base
- **WHEN** the branch `feat` has a commit that `origin/main` does not contain, `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists
- **AND** stdout contains `kept branch feat: not merged into origin/main`

#### Scenario: Pushed to its upstream
- **WHEN** the branch `feat` has a commit that `origin/main` does not contain, its upstream is `origin/feat`, `origin/feat` contains that commit, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Ahead of its upstream
- **WHEN** the branch `feat` has a commit that neither `origin/main` nor its upstream `origin/feat` contains, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists

#### Scenario: Squash merge
- **WHEN** the changes of the branch `feat` reached `origin/main` as one new commit made by a squash merge, `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists

#### Scenario: Base from the configuration
- **WHEN** `default_base` is `develop`, the tip of the branch `feat` is an ancestor of `develop` but not of `origin/main`, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: No base
- **WHEN** the repository has no `origin/HEAD`, its main worktree has a detached HEAD, `default_base` is empty, the branch `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** stderr contains a warning stating that the repository has no default branch
- **AND** the branch `feat` still exists
- **AND** the exit code is 0

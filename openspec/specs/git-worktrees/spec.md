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

# Spec Delta

## Purpose

Defines `wt list`, the command that shows every worktree of the current repository in one table for people and as a versioned JSON document for scripts and agents.

## ADDED Requirements

### Requirement: Listing the current repository
`wt list` SHALL list every worktree of the repository associated with the working directory (see the `git-worktrees` capability), including the main worktree, and SHALL exit 0. It SHALL accept no positional arguments. Outside a repository it SHALL fail as that capability specifies (exit 3).

#### Scenario: Repository with linked worktrees
- **WHEN** a repository has its main worktree and two linked worktrees and the user runs `wt list` inside any of them
- **THEN** all three worktrees are listed
- **AND** the exit code is 0

### Requirement: Table layout
Without `--json`, `wt list` SHALL print a header row followed by one row per worktree, with these columns in this order:

| Column | Content |
|---|---|
| (markers) | `@` if the worktree is current, `^` if it is the main one; both when both apply; blank otherwise |
| `NAME` | the last component of the worktree's path |
| `BRANCH` | the short branch name, `(detached)`, or `(bare)` |
| `HEAD` | the first 7 characters of the HEAD commit id, or `-` for a bare repository |
| `STATE` | `locked`, `prunable`, both separated by a comma, or blank |
| `PATH` | the worktree's native absolute path |

Columns SHALL be aligned by their visible width, whether or not color is enabled.

#### Scenario: Current and main markers
- **WHEN** the user runs `wt list` from inside a linked worktree
- **THEN** that worktree's row starts with `@`
- **AND** the main worktree's row carries `^`

#### Scenario: Locked worktree
- **WHEN** a linked worktree is locked
- **THEN** its `STATE` column reads `locked`

#### Scenario: Alignment with color
- **WHEN** color is enabled
- **THEN** after removing ANSI escape sequences, every column starts at the same character offset in every row

### Requirement: Order
The main worktree SHALL be listed first. The remaining worktrees SHALL be ordered by `NAME`, case-insensitively, with ties broken by path.

#### Scenario: Alphabetical after main
- **WHEN** the repository has linked worktrees named `zeta`, `Alpha` and `beta`
- **THEN** the rows after the main worktree are `Alpha`, `beta`, `zeta`

### Requirement: JSON output
`wt list --json` SHALL print a document with `schema` `wt.list.v1` and a `worktrees` array in the same order as the table. Each element SHALL have the fields `name`, `path`, `branch` (string or null), `head` (string or null), `detached`, `bare`, `main`, `current`, `locked`, `prunable` (booleans), and `locked_reason` and `prunable_reason` (string or null). `head` SHALL be the full commit id.

#### Scenario: Detached worktree in JSON
- **WHEN** a linked worktree has a detached HEAD
- **THEN** its element has `"branch": null`, `"detached": true`, and `head` set to the full commit id

#### Scenario: Lock reason in JSON
- **WHEN** a worktree was locked with reason `on usb drive`
- **THEN** its element has `"locked": true` and `"locked_reason": "on usb drive"`

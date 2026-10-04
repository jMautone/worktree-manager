## MODIFIED Requirements

### Requirement: Exit codes
Exit codes SHALL be part of the public contract and SHALL mean:

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Execution error (including invalid configuration and git failures) |
| 2 | Usage error (unknown command or flag, bad argument, invalid flag value) |
| 3 | Not found (repository, worktree, branch) |
| 4 | Ambiguous name |
| 5 | Blocked by state (dirty worktree, lock) |
| 6 | Git conflict |
| 7 | Hook failed |
| 8 | Repository hook not approved |

A command SHALL NOT use a code for a meaning other than the one listed, with one exception: a command that runs a command the user gave it with `-x` SHALL, once that command has started, exit with that command's exit code, unchanged, whatever its value. Until the command starts, the codes above apply.

#### Scenario: Not found
- **WHEN** the user runs `wt list` in a directory that is not inside a git repository
- **THEN** the exit code is 3

#### Scenario: Usage error
- **WHEN** the user runs `wt -C` without a directory
- **THEN** the exit code is 2

#### Scenario: Exit code of a user command
- **WHEN** the user runs `wt create feat -x <a command that exits with 4>` and the worktree is created
- **THEN** the exit code is 4, although `wt` found no ambiguous name

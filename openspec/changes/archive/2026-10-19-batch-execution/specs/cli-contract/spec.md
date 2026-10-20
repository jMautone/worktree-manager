## MODIFIED Requirements

### Requirement: Global flags
Every command SHALL accept the global flags `--json`, `--dry-run`, `-C <dir>`, `--no-color` and `-h/--help`, placed either before or after the command name. A command that takes the words of another command, `wt each`, SHALL recognize them only before the first of those words: from it on, every argument belongs to that other command, even when it looks like a global flag (see the `batch-execution` capability).

#### Scenario: Global flag before the command
- **WHEN** the user runs `wt --json list` inside a repository
- **THEN** the output is identical to `wt list --json`

#### Scenario: Global flag after the words of another command
- **WHEN** the user runs `wt each git log --no-color -1` inside a repository
- **THEN** `--no-color` reaches `git`
- **AND** the exit code is 0

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
| 130 | Interrupted from the terminal (Ctrl-C) before finishing |

Code 130 SHALL mean the same on every operating system. A command that runs commands the user gave it in several worktrees (`wt each`) SHALL exit with 1 when one of them failed and with 130 when it was interrupted (see the `batch-execution` capability).

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

#### Scenario: Failed command in a batch
- **WHEN** the user runs `wt each <a command that exits with 4>` inside a repository
- **THEN** the exit code is 1

#### Scenario: Interrupted batch (macOS, Linux)
- **WHEN** on macOS or Linux `wt each` receives SIGINT while one of its commands runs
- **THEN** the exit code is 130

### Requirement: Human-readable error format
On failure without `--json`, `wt` SHALL write nothing to stdout and SHALL write to stderr one line `wt: <message>`, optionally followed by lines beginning with `hint: `. Warnings SHALL be written to stderr as `wt: warning: <message>` and SHALL NOT change the exit code.

`wt each` is the exception to the first rule: when it reports, after its commands ran, that one of them failed or that it was interrupted, it has already written its headers to stdout, and its commands have written to stdout and stderr whatever they wrote. Its report SHALL still be one line `wt: <message>` followed by `hint: ` lines, written to stderr after the last command's output (see the `batch-execution` capability).

#### Scenario: Error with hint
- **WHEN** a command fails because the working directory is not in a git repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** stdout is empty

#### Scenario: Report of a batch
- **WHEN** the user runs `wt each <a command that prints ok and exits with 3>` in a repository with no linked worktree
- **THEN** stdout contains the header of the main worktree and `ok`
- **AND** the last lines of stderr are `wt: 1 of 1 commands failed` and `hint: ^ exited with 3`
- **AND** the exit code is 1

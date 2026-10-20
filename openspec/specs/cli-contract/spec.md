# cli-contract Specification

## Purpose

Defines the command-line contract every `wt` command obeys: how commands and flags are parsed and validated, which global flags exist, how errors and machine-readable output are emitted, and which exit codes callers can rely on.

## Requirements

### Requirement: Program identity
The executable SHALL be named `git-wt` (`git-wt.exe` on Windows) so that git exposes it as `git wt`. All diagnostics SHALL identify the program as `wt`, regardless of how it was invoked.

#### Scenario: Invoked through git
- **WHEN** the user runs `git wt version` with `git-wt` on the PATH
- **THEN** the version is printed exactly as with `git-wt version`

#### Scenario: Diagnostic prefix
- **WHEN** any command fails
- **THEN** the first line written to stderr starts with `wt: `

### Requirement: Help
Running `wt` with no arguments, `wt help`, `wt -h` or `wt --help` SHALL print the list of commands to stdout and exit 0. `wt <command> -h`, `wt <command> --help` and `wt help <command>` SHALL print that command's usage to stdout and exit 0.

#### Scenario: No arguments
- **WHEN** the user runs `wt` with no arguments
- **THEN** stdout lists the available commands, including `list`, `config` and `version`
- **AND** the exit code is 0

#### Scenario: Command help
- **WHEN** the user runs `wt list --help`
- **THEN** stdout describes the usage of `wt list`
- **AND** the exit code is 0

### Requirement: Strict validation of commands, flags and arguments
An unknown command, an unknown flag, a flag missing its value, or a wrong number of positional arguments SHALL be a usage error: nothing is executed, nothing is written to stdout, a diagnostic naming the offending token is written to stderr, and the exit code is 2. When an unknown command is close to a known one, the diagnostic SHALL suggest it.

#### Scenario: Unknown command
- **WHEN** the user runs `wt lsit`
- **THEN** stderr contains `unknown command "lsit"` and suggests `list`
- **AND** stdout is empty
- **AND** the exit code is 2

#### Scenario: Unknown flag
- **WHEN** the user runs `wt list --bogus`
- **THEN** stderr names `--bogus` as an unknown flag
- **AND** the exit code is 2

#### Scenario: Unexpected positional argument
- **WHEN** the user runs `wt list extra`
- **THEN** stderr reports the unexpected argument
- **AND** the exit code is 2

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

### Requirement: Machine-readable output
With `--json`, a successful command SHALL write exactly one JSON document to stdout and nothing else. The document SHALL be an object whose `schema` field names its shape as `wt.<kind>.v<version>`, and all field names SHALL be `snake_case`. A change that removes or renames a field, or changes a field's type or meaning, SHALL increment the version. On failure with `--json`, stdout SHALL be empty and stderr SHALL contain exactly one JSON document `{"schema":"wt.error.v1","code":<exit code>,"message":<text>}` (plus an optional `hints` array of strings). Warnings SHALL NOT be written to stderr when `--json` is given.

#### Scenario: JSON success
- **WHEN** the user runs `wt list --json` inside a repository
- **THEN** stdout parses as a single JSON object whose `schema` is `wt.list.v1`

#### Scenario: JSON failure
- **WHEN** the user runs `wt list --json` outside any git repository
- **THEN** stdout is empty
- **AND** stderr parses as a JSON object with `schema` `wt.error.v1` and `code` 3
- **AND** the exit code is 3

### Requirement: Dry run
Every command SHALL accept `--dry-run`. A command given `--dry-run` SHALL perform all of its reads against the real state and SHALL NOT create, modify or delete any file, git ref, worktree or configuration. Commands that never write SHALL behave identically with and without `--dry-run`.

#### Scenario: Read-only command under dry run
- **WHEN** the user runs `wt list --dry-run` inside a repository
- **THEN** the output is identical to `wt list`

### Requirement: Working directory override
`-C <dir>` SHALL make the command behave as if it had been started in `<dir>`. A `<dir>` that does not exist or is not a directory SHALL be a usage error (exit 2). Relative paths SHALL be resolved against the actual working directory.

#### Scenario: Listing another repository
- **WHEN** the user runs `wt -C /path/to/repo list` from outside any repository
- **THEN** the worktrees of `/path/to/repo` are listed
- **AND** the exit code is 0

#### Scenario: Missing directory
- **WHEN** the user runs `wt -C /does/not/exist list`
- **THEN** stderr names the directory
- **AND** the exit code is 2

### Requirement: Color
Output SHALL contain ANSI color sequences only when stdout is a terminal, the `NO_COLOR` environment variable is unset or empty, and `--no-color` is not given. JSON output SHALL never contain color sequences. On Windows, color SHALL be emitted only if the console accepts ANSI sequences.

#### Scenario: Piped output
- **WHEN** the output of `wt list` is piped to another program
- **THEN** it contains no ANSI escape sequences

#### Scenario: NO_COLOR
- **WHEN** `NO_COLOR=1` is set and `wt list` runs in a terminal
- **THEN** the output contains no ANSI escape sequences

### Requirement: Version
`wt version` and `wt --version` SHALL print the version string to stdout and exit 0. With `--json`, `wt version` SHALL print `{"schema":"wt.version.v1","version":...,"commit":...,"os":...,"arch":...}`, where `commit` is the source revision or an empty string when unknown. Neither form SHALL read the configuration, so they SHALL succeed even when the configuration is invalid.

#### Scenario: Version with broken configuration
- **WHEN** the user config file contains invalid TOML and the user runs `wt version`
- **THEN** the version is printed
- **AND** the exit code is 0

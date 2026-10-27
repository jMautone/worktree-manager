## MODIFIED Requirements

### Requirement: Human-readable error format
On failure without `--json`, `wt` SHALL write nothing to stdout and SHALL write to stderr one line `wt: <message>`, optionally followed by lines beginning with `hint: `. Warnings SHALL be written to stderr as `wt: warning: <message>` and SHALL NOT change the exit code.

`wt each` is the exception to the first rule: when it reports, after its commands ran, that one of them failed or that it was interrupted, it has already written its headers to stdout, and its commands have written to stdout and stderr whatever they wrote. Its report SHALL still be one line `wt: <message>` followed by `hint: ` lines, written to stderr after the last command's output (see the `batch-execution` capability).

A command that ran blocking hooks before it failed (see the `hooks` capability) has already written to stderr the hooks' `==>` lines and whatever their commands wrote. Its stdout SHALL still be empty, and its report SHALL still be one line `wt: <message>` followed by `hint: ` lines, written to stderr after the last hook's output.

#### Scenario: Error with hint
- **WHEN** a command fails because the working directory is not in a git repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** stdout is empty

#### Scenario: Report of a batch
- **WHEN** the user runs `wt each <a command that prints ok and exits with 3>` in a repository with no linked worktree
- **THEN** stdout contains the header of the main worktree and `ok`
- **AND** the last lines of stderr are `wt: 1 of 1 commands failed` and `hint: ^ exited with 3`
- **AND** the exit code is 1

#### Scenario: Report after a hook
- **WHEN** `hooks.pre-create` is `["echo checking", "exit 3"]` and the user runs `wt create feat`
- **THEN** stderr's first lines are `==> pre-create  echo checking`, `checking` and `==> pre-create  exit 3`
- **AND** the last lines of stderr are `wt: pre-create hook failed: exit 3 exited with 3` and a `hint:` line naming `--no-hooks`
- **AND** stdout is empty
- **AND** the exit code is 7

# Spec Delta

## Purpose

Defines how a repository declares hooks in its `.wt.toml` without being trusted: which of its commands need the user's approval, what identifies an approval and where it is kept, what a command does when a hook is not approved, and the `wt hook list` and `wt hook approve` commands.

## ADDED Requirements

### Requirement: Repository hooks
A repository's hooks SHALL be the `[hooks]` table of the `.wt.toml` of the command's repository layer (see the `config-layers` capability), with the events and the form of the user's. They SHALL be validated as the user's, and an invalid one SHALL fail the command with exit code 1 naming the key and `.wt.toml`. Without a repository layer, a repository has no hooks.

#### Scenario: Hooks read from the repository
- **WHEN** `.wt.toml` at the root of the current worktree contains `[hooks]` with `post-create = ["echo {name} > repo.txt"]`, that command is approved, and the user runs `wt create feat`
- **THEN** `repo.txt` in the new worktree contains `feat`
- **AND** stderr contains no warning naming `hooks.post-create`

#### Scenario: Invalid repository hook
- **WHEN** `.wt.toml` contains `[hooks]` with `pre-remove = ["echo {base}"]` and the user runs `wt list`
- **THEN** stderr names `hooks.pre-remove`, `.wt.toml` and `base`
- **AND** the exit code is 1

#### Scenario: Hooks of the other repository
- **WHEN** the shell is inside `<root>/web`, whose `.wt.toml` sets `post-create = ["echo web > from.txt"]`, the main worktree of `<root>/api` has a `.wt.toml` with `post-create = ["echo api > from.txt"]`, approved for `api`, and the user runs `wt create api:feat`
- **THEN** `from.txt` in the new worktree of `api` contains `api`

### Requirement: Approval of repository hook commands
A command of a repository's hooks SHALL run only when it is approved. An approval SHALL be identified by the repository, the event, and the SHA-256 of the command's text as written in `.wt.toml`, before it is rendered. The repository SHALL be the main worktree's path of the repository whose `.wt.toml` declares the command, native and absolute. A change of any character of a command SHALL require a new approval. Hooks of the user's file SHALL need no approval.

#### Scenario: Approved in one repository only
- **WHEN** `npm ci` is approved for `post-create` in `/src/a`, and `/src/b`'s `.wt.toml` sets `post-create = ["npm ci"]`, and the user runs `wt create feat` inside `/src/b`
- **THEN** the exit code is 8

#### Scenario: Approved for one event only
- **WHEN** `make` is approved for `post-create` in the repository, its `.wt.toml` sets `pre-remove = ["make"]`, and the user runs `wt remove feat`
- **THEN** the exit code is 8
- **AND** the worktree `feat` still exists

#### Scenario: Command changed after approval
- **WHEN** `npm ci` is approved for `post-create`, `.wt.toml` changes it to `npm ci --force`, and the user runs `wt create feat`
- **THEN** the exit code is 8
- **AND** no worktree `feat` exists

#### Scenario: Approval valid in every worktree
- **WHEN** `echo {name} > n.txt` is approved for `post-create` from inside the main worktree, and from inside the linked worktree `a`, whose `.wt.toml` has the same command, the user runs `wt create b`
- **THEN** `n.txt` in the new worktree contains `b`

### Requirement: Approvals file
Approvals SHALL be kept in the file `approvals.toml` in the directory of the user configuration file (see the `config-layers` capability), on every operating system and also when `WT_CONFIG` names that file. A missing file SHALL mean no approvals. A file that is not valid TOML, or whose entries lack a field, SHALL fail every command that reads it with exit code 1, naming the file. `wt` SHALL write it only from `wt hook approve`.

#### Scenario: Location (macOS, Linux)
- **WHEN** on macOS or Linux `WT_CONFIG` is unset, `XDG_CONFIG_HOME` is `/cfg`, and the user approves a command
- **THEN** the file `/cfg/wt/approvals.toml` exists

#### Scenario: Location (Windows)
- **WHEN** on Windows `WT_CONFIG` is unset, `APPDATA` is `C:\Users\u\AppData\Roaming`, and the user approves a command
- **THEN** the file `C:\Users\u\AppData\Roaming\wt\approvals.toml` exists

#### Scenario: Next to WT_CONFIG
- **WHEN** `WT_CONFIG` is `/tmp/x/wt.toml` and the user approves a command
- **THEN** the file `/tmp/x/approvals.toml` exists

#### Scenario: Broken file
- **WHEN** `approvals.toml` contains `[[approval` and the user runs `wt hook list` inside a repository
- **THEN** stderr names the file
- **AND** the exit code is 1

### Requirement: Format of the approvals file
`approvals.toml` SHALL hold one `[[approval]]` table per approval, with the string keys `repo` (the main worktree's path), `event`, `sha256` (64 lowercase hexadecimal digits) and `command` (the text approved, for the reader). Only `repo`, `event` and `sha256` SHALL decide whether a command is approved. Removing a table SHALL revoke its approval.

#### Scenario: Entry written
- **WHEN** the repository `/src/repo` sets `post-create = ["npm ci"]` and the user runs `wt hook approve` inside it
- **THEN** `approvals.toml` has an `[[approval]]` table with `repo` `/src/repo`, `event` `post-create`, `command` `npm ci`, and `sha256` the SHA-256 of `npm ci` in lowercase hexadecimal

#### Scenario: Revoked by hand
- **WHEN** the user deletes from `approvals.toml` the table that approves `npm ci` for `post-create` and runs `wt create feat` in that repository
- **THEN** the exit code is 8

### Requirement: Hook not approved
When an event that `wt create`, `wt cd` or `wt remove` would fire has a repository command that is not approved, the command SHALL fail with exit code 8 before it changes anything, including when the event runs after the operation, such as `post-create`. The message SHALL be `<event> hook of <path of .wt.toml> is not approved: <command>`, for the first such command in event order, with the command as written.

#### Scenario: post-create not approved
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and through the function the user runs `wt create feat -x "echo ran"`
- **THEN** no branch `feat`, no directory at its path and no worktree exist afterwards
- **AND** the shell does not move
- **AND** stdout is empty
- **AND** stderr contains `wt: post-create hook of <worktree root>/.wt.toml is not approved: npm ci`
- **AND** the exit code is 8

#### Scenario: Background event not approved
- **WHEN** `.wt.toml` sets `post-cd = ["git fetch"]`, not approved, and the user runs `wt cd feat` through the function
- **THEN** the shell's working directory does not change
- **AND** the exit code is 8

#### Scenario: Not approved with JSON
- **WHEN** `.wt.toml` sets `pre-create = ["make check"]`, not approved, and the user runs `wt create feat --json`
- **THEN** stdout is empty
- **AND** stderr is a single `wt.error.v1` document with `code` 8
- **AND** the exit code is 8

### Requirement: Hints of a hook not approved
The error of "Hook not approved" SHALL have a hint with the command that approves the repository's hooks, and a hint naming `--no-hooks`. The approving command SHALL be `wt hook approve` when the repository is the one of the working directory, and `wt -C <main worktree path> hook approve` otherwise.

#### Scenario: Hints in the current repository
- **WHEN** `.wt.toml` sets `pre-create = ["make check"]`, not approved, and the user runs `wt create feat`
- **THEN** stderr has a `hint:` line containing `wt hook approve` and a `hint:` line naming `--no-hooks`

#### Scenario: Hint for another repository
- **WHEN** the main worktree of `<root>/api` has a `.wt.toml` with `post-create = ["npm ci"]`, not approved, and from inside `<root>/web` the user runs `wt create api:feat`
- **THEN** stderr has a `hint:` line containing `wt -C <root>/api hook approve`
- **AND** the exit code is 8

### Requirement: Checking approvals
Approvals SHALL be checked for every event the command would fire, and only for those: `pre-create` and `post-create` for `wt create`, `pre-cd` and `post-cd` for `wt cd`, `pre-remove` and `post-remove` for `wt remove`. With `--no-hooks` no approval SHALL be checked. Under `--dry-run` approvals SHALL be checked as without it.

#### Scenario: Other events are not checked
- **WHEN** `.wt.toml` sets `pre-remove = ["make clean"]`, not approved, and no other hook, and the user runs `wt create feat`
- **THEN** the worktree `feat` is created
- **AND** the exit code is 0

#### Scenario: Skipping hooks skips the check
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt create feat --no-hooks`
- **THEN** the worktree `feat` is created
- **AND** the exit code is 0

#### Scenario: Dry run not approved
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt create feat --dry-run`
- **THEN** stdout is empty
- **AND** the exit code is 8

### Requirement: Listing hooks
`wt hook list` SHALL print every hook command that would run for the repository of the working directory and its repository layer: by event, in the order of the `hooks` capability's table, and within an event the user's before the repository's. It SHALL print a header and one row per command with the columns `EVENT`, `SOURCE` (`user` or `repo`), `APPROVED` (`yes` or `no` for `repo`, `-` for `user`) and `COMMAND`, as written.

#### Scenario: User and repository hooks
- **WHEN** the user file sets `post-create = ["cp ~/.npmrc ."]`, `.wt.toml` sets `post-create = ["npm ci"]` and `pre-remove = ["make clean"]`, `npm ci` is approved and `make clean` is not, and the user runs `wt hook list`
- **THEN** stdout has a header line and then three lines, in this order: `post-create`, `user`, `-`, `cp ~/.npmrc .`; `post-create`, `repo`, `yes`, `npm ci`; `pre-remove`, `repo`, `no`, `make clean`
- **AND** the exit code is 0

### Requirement: Listing hooks when there are none
With no hook command to list, `wt hook list` SHALL print nothing and exit with code 0. Outside a repository it SHALL fail with exit code 3.

#### Scenario: No hooks
- **WHEN** no configuration sets hooks and the user runs `wt hook list` inside a repository
- **THEN** stdout is empty
- **AND** the exit code is 0

#### Scenario: Outside a repository
- **WHEN** the user runs `wt hook list` in a directory that is in no repository
- **THEN** the exit code is 3

### Requirement: Hook list as JSON
With `--json`, `wt hook list` SHALL write a `wt.hook.list.v1` document with `repo_path` (the main worktree's path), `repo_file` (the path of the `.wt.toml` read, or null when it does not exist) and `hooks`, an array, in the order of "Listing hooks", of objects with `event`, `source`, `command` and `approved`, which SHALL be true for every command of the user.

#### Scenario: JSON
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt hook list --json`
- **THEN** stdout parses as one JSON object with `schema` `wt.hook.list.v1`
- **AND** its `hooks` is one object with `event` `post-create`, `source` `repo`, `command` `npm ci` and `approved` false

### Requirement: Approving hooks
`wt hook approve [<event>...]` SHALL approve, for the repository of the working directory and its repository layer, every repository command not yet approved, of the events named or of every event when none is named. It SHALL print one line `approved <event> hook: <command>` per approval, in the order of "Listing hooks", and `nothing to approve` when there is none. An unknown event SHALL be a usage error (exit 2). Outside a repository it SHALL fail with exit code 3.

#### Scenario: Approve then create
- **WHEN** `.wt.toml` sets `post-create = ["echo ok > ok.txt"]`, the user runs `wt hook approve` and then `wt create feat`
- **THEN** the first command prints `approved post-create hook: echo ok > ok.txt`
- **AND** the new worktree has the file `ok.txt`
- **AND** both exit codes are 0

#### Scenario: Only the events named
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]` and `pre-remove = ["make clean"]`, and the user runs `wt hook approve post-create`
- **THEN** stdout is `approved post-create hook: npm ci`
- **AND** `wt hook list` shows `make clean` as not approved

#### Scenario: Nothing to approve
- **WHEN** every repository command is approved and the user runs `wt hook approve`
- **THEN** stdout is `nothing to approve`
- **AND** `approvals.toml` is unchanged

#### Scenario: Unknown event
- **WHEN** the user runs `wt hook approve post-merge`
- **THEN** stderr names `post-merge`
- **AND** the exit code is 2

### Requirement: Approving hooks in a dry run or as JSON
Under `--dry-run`, `wt hook approve` SHALL print `would approve <event> hook: <command>` for each command it would approve, and SHALL NOT write `approvals.toml`. With `--json`, it SHALL write a `wt.hook.approve.v1` document whose `approved` is an array of objects with `event` and `command`, empty when there was nothing to approve.

#### Scenario: Preview
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt hook approve --dry-run`
- **THEN** stdout is `would approve post-create hook: npm ci`
- **AND** `approvals.toml` does not exist afterwards

#### Scenario: JSON
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt hook approve --json`
- **THEN** stdout parses as one JSON object with `schema` `wt.hook.approve.v1`, whose `approved` is one object with `event` `post-create` and `command` `npm ci`

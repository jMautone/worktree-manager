## Purpose

Defines the hooks `wt` runs on its own around its commands: which events exist and which command fires each, where and in what order their commands run, which ones the command waits for and which run in the background, where their output goes, and what is left done when one fails or is interrupted.

## ADDED Requirements

### Requirement: Events
`wt` SHALL run hooks on exactly these six events, each fired only by the command named:

| Event | Fired by | When | Runs in | Kind |
|---|---|---|---|---|
| `pre-create` | `wt create` | once every check passes, after the fetch, before the branch is created | the main worktree | blocking |
| `post-create` | `wt create` | once the worktree is created, before the directive is written and before the `-x` command | the new worktree | blocking |
| `pre-cd` | `wt cd` | once the destination is resolved and found to exist, before the directive is written | the destination's worktree | blocking |
| `post-cd` | `wt cd` | once the directive is written | the destination's worktree | background |
| `pre-remove` | `wt remove` | once every check passes, before anything is removed | the worktree to remove, or the main worktree when that worktree's directory does not exist | blocking |
| `post-remove` | `wt remove` | once the worktree is removed, its branch deleted or kept, and the directive written | the main worktree | background |

The main worktree SHALL be the one the `git-worktrees` capability defines: for a bare repository, the bare repository itself. The destination's worktree SHALL be the worktree that contains the destination of `wt cd`, and the hook SHALL run at its root. When that destination is inside no worktree, as the previous directory of `wt cd -` can be, `wt cd` SHALL fire neither `pre-cd` nor `post-cd`. When a command operates on a worktree of another repository (see the `cross-repo-resolution` capability), its events SHALL concern that worktree and run in that repository. An event with no commands SHALL do nothing.

`wt create` and `wt remove` SHALL NOT fire `pre-cd` or `post-cd`, even when they move the shell. No other command SHALL run hooks.

#### Scenario: Order of the create events
- **WHEN** `hooks.pre-create` is `["git branch --list feat > pre.txt"]`, `hooks.post-create` is `["git branch --show-current > post.txt"]`, and the user runs `wt create feat` in a repository whose main worktree is `/src/repo`
- **THEN** `/src/repo/pre.txt` exists and is empty
- **AND** `post.txt` in the new worktree contains `feat`
- **AND** the exit code is 0

#### Scenario: Removing from inside does not fire cd events
- **WHEN** `hooks.pre-cd` and `hooks.post-cd` each hold a command that creates a file in `/tmp/cd-ran`, the shell is inside the clean worktree `feat`, and the user runs `wt remove @` through the function
- **THEN** the shell's working directory is the main worktree's path
- **AND** no file exists in `/tmp/cd-ran`

#### Scenario: Previous directory outside every repository
- **WHEN** `hooks.pre-cd` holds a command that creates a file in its working directory, the shell is in `/tmp/x`, which is in no repository, the user runs `wt cd feat` and then `wt cd -` through the function
- **THEN** the shell is back in `/tmp/x`
- **AND** `/tmp/x` has no file created by the hook

#### Scenario: Worktree of another repository
- **WHEN** `repos_root` is `["<root>"]`, `hooks.post-create` is `["echo {repo} > repo.txt"]`, and from inside `<root>/web` the user runs `wt create api:feat`
- **THEN** `repo.txt` in the new worktree of `<root>/api` contains `api`

#### Scenario: Printing a path runs no hook
- **WHEN** `hooks.pre-cd` is `["exit 1"]` and the user runs `wt path feat` inside a repository with the linked worktree `feat`
- **THEN** stdout is the path of `feat`
- **AND** the exit code is 0

### Requirement: Hook configuration
The commands of an event SHALL be the elements of the configuration key `hooks.<event>`, in order (see the `config-layers` capability). Only the user's configuration file SHALL set them: a repository's `.wt.toml` SHALL NOT, and its `[hooks]` table SHALL be ignored with a warning, as the `config-layers` capability specifies for keys a repository may not set.

#### Scenario: Hooks of a repository are ignored
- **WHEN** `.wt.toml` at the root of the main worktree contains `[hooks]` with `post-create = ["echo pwned > pwned.txt"]`, the user file sets no hooks, and the user runs `wt create feat`
- **THEN** the new worktree has no file `pwned.txt`
- **AND** stderr contains a warning naming `hooks.post-create` and `.wt.toml`
- **AND** the exit code is 0

### Requirement: Variables of hook commands
Each command SHALL be a template, with the syntax and filters of the `path-templates` capability. It SHALL be able to use these variables:

| Variable | Value | Events |
|---|---|---|
| `{repo}` | as in `worktree_path` | every event |
| `{repo_parent}` | as in `worktree_path` | every event |
| `{repo_path}` | the main worktree's path | every event |
| `{name}` | the worktree's `NAME`, as `wt list` shows it | every event |
| `{branch}` | the worktree's branch; empty when its HEAD is detached or it is a bare repository | every event |
| `{path}` | the worktree's root | every event |
| `{base}` | the base the new branch starts at, as written in the `created worktree` line | `pre-create`, `post-create` |

The worktree SHALL be the one the event concerns: for `pre-create` and `post-create`, the new worktree, whose path and `NAME` are known before it is created; for `pre-cd` and `post-cd`, the destination's worktree; for `pre-remove` and `post-remove`, the worktree removed. Paths SHALL be absolute and in the operating system's native form. Values SHALL be inserted as they are, without quoting: a command that needs quotes around a value SHALL write them.

`{name}` in a hook is the `NAME` of the worktree, the last component of its path, while `{name}` in `worktree_path` is the `<name>` given to `wt create`, as typed. A command that uses a variable not available for its event SHALL be an invalid template (see the `path-templates` capability).

#### Scenario: Values of a create event
- **WHEN** `hooks.post-create` is `["echo {repo} {name} {branch} {base} > vars.txt"]`, and the user runs `wt create feature/x` in the repository `/src/repo`, whose default branch is `origin/main`, with the default `worktree_path`
- **THEN** `vars.txt` in the new worktree contains `repo feature-x feature/x origin/main`

#### Scenario: Name made by worktree_path
- **WHEN** `worktree_path` is `{repo_parent}/{repo}.worktrees/{branch|sanitize}`, `branch_prefix` is `jm/`, `hooks.post-create` is `["echo {name} > name.txt"]`, and the user runs `wt create auth`
- **THEN** `name.txt` in the new worktree contains `jm-auth`

#### Scenario: Base outside a create event
- **WHEN** the user file sets `hooks.pre-remove` to `["echo {base}"]` and the user runs `wt list`
- **THEN** stderr names `hooks.pre-remove`, the user file and `base`
- **AND** the exit code is 1

#### Scenario: Literal braces
- **WHEN** `hooks.post-create` is `["awk '{{print $1}}' in.txt"]` and the user runs `wt create feat --dry-run`
- **THEN** stdout contains `would run post-create hook: awk '{print $1}' in.txt`

### Requirement: Running hook commands
The commands of an event SHALL run one at a time, in the order of their key, each with the event's directory (see "Events") as its working directory: as `sh -c <command>` on macOS and Linux, and as a command line of `cmd.exe` on Windows, the interpreter of `-x` (see the `create-worktree` capability). Each command SHALL start only after the previous one exits. A command that exits with a code other than 0, that on macOS or Linux is terminated by a signal, or that cannot be started SHALL fail its event, and the event's remaining commands SHALL NOT run. Hook commands SHALL NOT receive `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR` (see the `shell-integration` capability).

#### Scenario: In order
- **WHEN** `hooks.post-create` is `["echo a> out.txt", "echo b>> out.txt"]` and the user runs `wt create feat`
- **THEN** `out.txt` in the new worktree contains `a` and `b`, each on its own line, in that order

#### Scenario: First failure stops the event
- **WHEN** `hooks.post-create` is `["exit 3", "echo second> second.txt"]` and the user runs `wt create feat`
- **THEN** the new worktree has no file `second.txt`
- **AND** the exit code is 7

#### Scenario: Interpreter operators (macOS, Linux)
- **WHEN** on macOS or Linux `hooks.post-create` is `["echo a > a.txt && echo b > b.txt"]` and the user runs `wt create feat`
- **THEN** the new worktree has the files `a.txt` and `b.txt`

#### Scenario: Interpreter operators (Windows)
- **WHEN** on Windows `hooks.post-create` is `["echo a> a.txt& echo b> b.txt"]` and the user runs `wt create feat`
- **THEN** the new worktree has the files `a.txt` and `b.txt`

#### Scenario: Protocol variables not passed
- **WHEN** `hooks.post-create` and `hooks.post-cd` each hold a command that records its environment, and through the function the user runs `wt create feat` and then `wt cd feat`
- **THEN** no recorded environment contains `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR`

### Requirement: Blocking hooks
`pre-create`, `post-create`, `pre-cd` and `pre-remove` SHALL be blocking: `wt` SHALL wait until the event's commands are done before it goes on. Without `--json`, before each command `wt` SHALL write to stderr the line `==> <event>  <command>`, with the command as rendered, and the command SHALL use `wt`'s standard input and write both its output and its errors to `wt`'s standard error, so that `wt`'s standard output carries only what `wt` itself prints. These lines SHALL never contain color sequences. With `--json`, see "JSON output with hooks".

#### Scenario: Output on stderr
- **WHEN** `hooks.post-create` is `["echo hello"]` and the user runs `wt create feat` in a repository whose default branch is `origin/main`
- **THEN** stderr contains the line `==> post-create  echo hello`, followed by the line `hello`
- **AND** stdout is the single line `created worktree <path> on new branch feat from origin/main`

#### Scenario: The command waits (macOS, Linux)
- **WHEN** on macOS or Linux `hooks.post-create` is `["sleep 1; touch prepared"]` and the user runs `wt create feat -x "test -f prepared"`
- **THEN** the exit code is 0

#### Scenario: The command waits (Windows)
- **WHEN** on Windows `hooks.post-create` is `["ping -n 2 127.0.0.1 > nul& echo x> prepared"]` and the user runs `wt create feat -x "if exist prepared (exit /b 0) else (exit /b 1)"`
- **THEN** the exit code is 0

### Requirement: Failed blocking hook
When a command of a blocking event fails, `wt` SHALL stop the operation and exit with code 7. The message SHALL be `<event> hook failed: <command> exited with <code>`, where `<code>` is the command's exit code or, for a command terminated by a signal, 128 plus the signal number; or `<event> hook failed: <command> could not start: <reason>`. What is left done SHALL depend on the event:

- `pre-create`: no branch, directory or worktree SHALL be created, the shell SHALL NOT move, and the `-x` command SHALL NOT run. A hint SHALL name `--no-hooks`.
- `post-create`: the branch and the worktree SHALL remain created, the shell SHALL move as it would have (see the `create-worktree` capability, "Moving the shell"), the `-x` command SHALL NOT run, and nothing SHALL be written to stdout. A hint SHALL name the new worktree's path, stating that it was created.
- `pre-cd`: the directive SHALL NOT be written, so the shell SHALL NOT move, and `post-cd` SHALL NOT run. A hint SHALL name `--no-hooks`.
- `pre-remove`: no worktree, directory or branch SHALL be removed, the shell SHALL NOT move, and `post-remove` SHALL NOT run. A hint SHALL name `--no-hooks`.

#### Scenario: pre-create fails
- **WHEN** `hooks.pre-create` is `["exit 1"]` and the user runs `wt create feat`
- **THEN** stderr's last lines are `wt: pre-create hook failed: exit 1 exited with 1` and a `hint:` line naming `--no-hooks`
- **AND** no branch `feat`, no directory at its path and no worktree exist afterwards
- **AND** the exit code is 7

#### Scenario: post-create fails
- **WHEN** `hooks.post-create` is `["exit 2"]` and through the function the user runs `wt create feat -x "echo ran> ran.txt"`
- **THEN** stdout is empty
- **AND** stderr has a `hint:` line naming the path of the new worktree
- **AND** the worktree `feat` and the branch `feat` exist
- **AND** the shell's working directory is the path of `feat`
- **AND** the new worktree has no file `ran.txt`
- **AND** the exit code is 7

#### Scenario: pre-cd fails
- **WHEN** `hooks.pre-cd` is `["exit 1"]`, the shell is in the main worktree, and the user runs `wt cd feat` through the function
- **THEN** the shell's working directory does not change
- **AND** the exit code is 7

#### Scenario: pre-remove fails
- **WHEN** `hooks.pre-remove` is `["exit 1"]`, the branch `feat` is merged, and the user runs `wt remove feat`
- **THEN** the worktree `feat` and the branch `feat` still exist
- **AND** the exit code is 7

### Requirement: Background hooks
`post-cd` and `post-remove` SHALL run in the background: `wt` SHALL start a process, detached from the terminal, that runs the event's commands as "Running hook commands" specifies, and SHALL NOT wait for it. The commands SHALL get an empty standard input and SHALL write their output and errors to the event's log (see "Logs"). What they do and how they exit SHALL NOT change `wt`'s standard output or its exit code.

The commands SHALL keep running after `wt` exits, and SHALL NOT be ended by an interrupt from the terminal (Ctrl-C) or by closing the terminal: on macOS and Linux they SHALL NOT receive a SIGINT or SIGHUP sent to the process group `wt` ran in; on Windows they SHALL NOT receive the Ctrl-C, Ctrl-Break or close events of `wt`'s console.

Without `--json`, `wt` SHALL write to stderr, once per event it starts, the line `==> <event>  running in the background, log: <path of the log>`. When the background process cannot be started, `wt` SHALL write a warning naming the event and the reason, and SHALL exit as it would otherwise.

#### Scenario: wt does not wait
- **WHEN** `hooks.post-remove` holds a command that waits two seconds and then creates the file `done.txt` in its working directory, and the user runs `wt remove feat`
- **THEN** `wt` exits before `done.txt` exists
- **AND** the main worktree has the file `done.txt` once the command finishes
- **AND** the exit code is 0

#### Scenario: A failure does not change the exit code
- **WHEN** `hooks.post-remove` is `["exit 1"]` and the user runs `wt remove feat`
- **THEN** the exit code is 0
- **AND** the log of `post-remove` for `feat` ends with the line `==> exited with 1`

#### Scenario: Notice on stderr
- **WHEN** `hooks.post-cd` is `["git fetch"]` and the user runs `wt cd feat` through the function, in the repository whose main worktree is `/src/repo`
- **THEN** stderr contains `==> post-cd  running in the background, log: /src/repo/.git/wt/logs/feat/post-cd.log`
- **AND** stdout is empty

#### Scenario: Signals to the terminal (macOS, Linux)
- **WHEN** on macOS or Linux `hooks.post-remove` holds a command that waits two seconds and then creates the file `done.txt`, the user runs `wt remove feat`, and right after `wt` exits SIGHUP and SIGINT are sent to the process group `wt` ran in
- **THEN** the main worktree has the file `done.txt` once the command finishes

#### Scenario: Closing the console (Windows)
- **WHEN** on Windows, in Windows Terminal with PowerShell 7, `hooks.post-remove` holds a command that waits ten seconds and then writes a line, the user runs `wt remove feat`, and closes the tab right after `wt` returns
- **THEN** the log of `post-remove` for `feat` ends with the line `==> exited with 0`

### Requirement: Logs
The log of an event for a worktree SHALL be the file `wt/logs/<name|sanitize>/<event>.log` in the repository's common git directory, where `<name|sanitize>` is the worktree's `NAME` after the `sanitize` filter (see the `path-templates` capability). The common git directory SHALL be the one git shares among all the worktrees of the repository, as `git rev-parse --git-common-dir` reports it: the `.git` directory of the main worktree, or the bare repository itself. Missing directories SHALL be created. Every run of the event SHALL replace the file's previous contents.

For each command the event ran, in order, the log SHALL contain the line `==> <event>  <command>`, with the command as rendered, then everything the command wrote to its output and its errors, then the line `==> exited with <code>` or `==> could not start: <reason>`. A log SHALL be written for every background event, and for a blocking event only with `--json` (see "JSON output with hooks"). Logs SHALL remain when the worktree is removed.

#### Scenario: Log of post-remove
- **WHEN** the repository's main worktree is `/src/repo`, `hooks.post-remove` is `["echo bye"]`, and the user runs `wt remove feat`
- **THEN** once the command finishes, `/src/repo/.git/wt/logs/feat/post-remove.log` contains the lines `==> post-remove  echo bye`, `bye` and `==> exited with 0`, in that order

#### Scenario: Each run replaces the log
- **WHEN** `hooks.post-cd` is `["echo {name}"]`, and through the function the user runs `wt cd feat` twice
- **THEN** once both commands finish, the log of `post-cd` for `feat` contains the line `feat` once

#### Scenario: Bare repository
- **WHEN** `<root>/proj/.bare` is a bare repository with the linked worktree `<root>/proj/feat`, `hooks.post-remove` is `["echo bye"]`, and the user runs `wt remove feat`
- **THEN** once the command finishes, `<root>/proj/.bare/wt/logs/feat/post-remove.log` contains the line `bye`

### Requirement: Interrupt during a blocking hook
An interrupt from the terminal (Ctrl-C) while a command of a blocking event runs SHALL reach that command and SHALL NOT end `wt` before the command exits. Once it exits, whatever its exit code, `wt` SHALL NOT run the event's remaining commands, SHALL stop the operation leaving done what "Failed blocking hook" specifies for that event, and SHALL exit with code 130 and the message `interrupted: <event> hook did not finish`, on every operating system.

#### Scenario: Interrupt during pre-create (macOS, Linux)
- **WHEN** on macOS or Linux `hooks.pre-create` is `["sleep 1"]`, the user runs `wt create feat`, and `wt` receives SIGINT while the hook runs
- **THEN** `wt` exits only after the hook does
- **AND** no branch `feat` and no worktree exist afterwards
- **AND** stderr contains `wt: interrupted: pre-create hook did not finish`
- **AND** the exit code is 130

#### Scenario: Ctrl-C in the console (Windows)
- **WHEN** on Windows `hooks.pre-create` holds a command that waits for a key, and the user runs `wt create feat` and presses Ctrl-C while it waits
- **THEN** no worktree `feat` exists afterwards
- **AND** `$LASTEXITCODE` is 130

### Requirement: Skipping hooks
`wt create`, `wt cd` and `wt remove` SHALL accept the flag `--no-hooks`, which SHALL make that run fire no event; everything else SHALL happen as without it. `--no-hooks` SHALL be a usage error (exit 2) on every other command.

#### Scenario: Create without hooks
- **WHEN** `hooks.pre-create` is `["exit 1"]` and the user runs `wt create feat --no-hooks`
- **THEN** the worktree `feat` is created
- **AND** the exit code is 0

#### Scenario: Jump without hooks
- **WHEN** `hooks.pre-cd` is `["exit 1"]` and the user runs `wt cd feat --no-hooks` through the function
- **THEN** the shell's working directory is the path of `feat`

#### Scenario: Command without hooks
- **WHEN** the user runs `wt list --no-hooks`
- **THEN** stderr names `--no-hooks` as an unknown flag
- **AND** the exit code is 2

### Requirement: Dry run with hooks
Under `--dry-run`, no hook command SHALL run, no log SHALL be written, and no background process SHALL start. Without `--json`, the command SHALL print to stdout, for each command of each event it would fire and at the place the command's own dry run specifies, the line `would run <event> hook: <command>` for a blocking event, or `would run <event> hook in the background: <command>` for a background event, with the command as rendered with the real values. With `--no-hooks` it SHALL print none of these lines. With `--json`, the document SHALL be the same as without hooks.

#### Scenario: Preview of a create with hooks
- **WHEN** `fetch_before_create` is true, `hooks.pre-create` is `["echo {name}"]`, `hooks.post-create` is `["npm ci"]`, and the user runs `wt create feat --dry-run --no-cd` in a repository whose default branch is `origin/main`
- **THEN** stdout is `would fetch origin`, `would run pre-create hook: echo feat`, `would create worktree <path> on new branch feat from origin/main` and `would run post-create hook: npm ci`, in that order
- **AND** no worktree exists afterwards
- **AND** no log exists afterwards

### Requirement: JSON output with hooks
With `--json`, hooks SHALL run as without it, except that `wt` SHALL write no `==>` line to stderr, and the commands of a blocking event SHALL get an empty standard input and SHALL write their output and errors to the event's log (see "Logs") instead of `wt`'s standard error. When a blocking hook fails or is interrupted, stdout SHALL be empty, stderr SHALL be the `wt.error.v1` document alone, with `code` 7 or 130, and its `hints` SHALL include the path of the log, in addition to the hints of "Failed blocking hook".

#### Scenario: Hook output with JSON
- **WHEN** `hooks.post-create` is `["echo hello"]` and the user runs `wt create feat --json`
- **THEN** stdout parses as a single JSON object with `schema` `wt.create.v1`
- **AND** stderr is empty
- **AND** the log of `post-create` for `feat` contains the line `hello`

#### Scenario: Failed hook with JSON
- **WHEN** `hooks.post-create` is `["echo hello", "exit 1"]` and the user runs `wt create feat --json`
- **THEN** stdout is empty
- **AND** stderr parses as a single `wt.error.v1` document with `code` 7, whose `hints` name the path of the new worktree and the path of the log of `post-create` for `feat`
- **AND** that log contains the line `hello`
- **AND** the exit code is 7

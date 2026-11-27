## MODIFIED Requirements

### Requirement: Events
`wt` SHALL run hooks on exactly these eight events, each fired only by the command named:

| Event | Fired by | When | Runs in | Kind |
|---|---|---|---|---|
| `pre-create` | `wt create` | once every check passes, after the fetch, before the branch is created | the main worktree | blocking |
| `post-create` | `wt create` | once the worktree is created, before the directive is written and before the `-x` command | the new worktree | blocking |
| `pre-cd` | `wt cd` | once the destination is resolved and found to exist, before the directive is written | the destination's worktree | blocking |
| `post-cd` | `wt cd` | once the directive is written | the destination's worktree | background |
| `pre-remove` | `wt remove`, `wt merge` | once every check passes, before anything is removed | the worktree to remove, or the main worktree when that worktree's directory does not exist | blocking |
| `post-remove` | `wt remove`, `wt merge` | once the worktree is removed, its branch deleted or kept, and the directive written | the main worktree | background |
| `pre-merge` | `wt merge` | once the branch is rebased, before the target is moved | the worktree being merged | blocking |
| `post-merge` | `wt merge` | once the target is moved and, unless `--no-remove`, the worktree removed | the main worktree | background |

The main worktree SHALL be the one the `git-worktrees` capability defines: for a bare repository, the bare repository itself. The destination's worktree SHALL be the worktree that contains the destination of `wt cd`, and the hook SHALL run at its root. When that destination is inside no worktree, as the previous directory of `wt cd -` can be, `wt cd` SHALL fire neither `pre-cd` nor `post-cd`. When a command operates on a worktree of another repository (see the `cross-repo-resolution` capability), its events SHALL concern that worktree and run in that repository. An event with no commands SHALL do nothing.

`wt create`, `wt remove` and `wt merge` SHALL NOT fire `pre-cd` or `post-cd`, even when they move the shell. `wt merge` SHALL fire `pre-remove` and `post-remove` as `wt remove` does, for the worktree it removes (see the `merge-worktree` capability). No other command SHALL run hooks.

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

#### Scenario: Order of the merge events
- **WHEN** every event of `wt merge` and `wt remove` has a command that appends its event's name to the file `<sandbox>/events.txt`, and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** once the background commands finish, `events.txt` contains `pre-merge`, `pre-remove`, then `post-remove` and `post-merge` in either order

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
| `{target}` | the branch the worktree's branch is merged into | `pre-merge`, `post-merge` |

The worktree SHALL be the one the event concerns: for `pre-create` and `post-create`, the new worktree, whose path and `NAME` are known before it is created; for `pre-cd` and `post-cd`, the destination's worktree; for `pre-remove` and `post-remove`, the worktree removed; for `pre-merge` and `post-merge`, the worktree merged. Paths SHALL be absolute and in the operating system's native form. Values SHALL be inserted as they are, without quoting: a command that needs quotes around a value SHALL write them.

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

#### Scenario: Values of a merge event
- **WHEN** `hooks.pre-merge` is `["echo {branch} {target} > vars.txt"]` and from inside the clean worktree `feat`, on the branch `feat`, the user runs `wt merge main --no-remove`
- **THEN** `vars.txt` in `feat` contains `feat main`

#### Scenario: Target outside a merge event
- **WHEN** the user file sets `hooks.pre-remove` to `["echo {target}"]` and the user runs `wt list`
- **THEN** stderr names `hooks.pre-remove`, the user file and `target`

### Requirement: Blocking hooks
`pre-create`, `post-create`, `pre-cd`, `pre-remove` and `pre-merge` SHALL be blocking: `wt` SHALL wait until the event's commands are done before it goes on. Without `--json`, before each command `wt` SHALL write to stderr the line `==> <event>  <command>`, with the command as rendered, and the command SHALL use `wt`'s standard input and write both its output and its errors to `wt`'s standard error, so that `wt`'s standard output carries only what `wt` itself prints. These lines SHALL never contain color sequences. With `--json`, see "JSON output with hooks".

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
- `pre-remove`: no worktree, directory or branch SHALL be removed, the shell SHALL NOT move, and `post-remove` SHALL NOT run. A hint SHALL name `--no-hooks`. Under `wt merge`, the target SHALL stay moved and `post-merge` SHALL NOT run.
- `pre-merge`: the target SHALL NOT be moved, the worktree SHALL NOT be removed, the shell SHALL NOT move, and no later event SHALL run; the branch SHALL stay rebased. A hint SHALL name `--no-hooks`.

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

#### Scenario: pre-merge fails
- **WHEN** `hooks.pre-merge` is `["exit 1"]` and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** the target branch points to the commit it pointed to before
- **AND** the worktree `feat` and the branch `feat` still exist
- **AND** the exit code is 7

### Requirement: Background hooks
`post-cd`, `post-remove` and `post-merge` SHALL run in the background: `wt` SHALL start a process, detached from the terminal, that runs the event's commands as "Running hook commands" specifies, and SHALL NOT wait for it. The commands SHALL get an empty standard input and SHALL write their output and errors to the event's log (see "Logs"). What they do and how they exit SHALL NOT change `wt`'s standard output or its exit code.

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

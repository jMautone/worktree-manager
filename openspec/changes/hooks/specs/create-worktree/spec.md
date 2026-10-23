## MODIFIED Requirements

### Requirement: Arguments and flags
`wt create` SHALL take exactly one argument, `<name>`, which may also be written `<repo>:<name>` (see "Repository"), and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `-b`, `--branch <branch>` | the branch to create, used as given |
| `--base <ref>` | the revision the new branch starts at |
| `-x <cmd>` | a command to run in the new worktree |
| `--cd` | move the shell into the new worktree |
| `--no-cd` | do not move the shell |
| `--no-hooks` | run no hook (see the `hooks` capability) |

In the form `<repo>:<name>`, the text before the first `:` is the repository and the rest is the name, and the rules below for `<name>` apply to that name. These SHALL be usage errors, with exit code 2, detected before anything is fetched, created or run: a missing `<name>` or an extra argument; an empty `<name>`, including the empty name of `<repo>:`; an empty repository, as in `:feat`; a `<name>` with a component, separated by `/` or `\`, equal to `.` or `..`; `--cd` together with `--no-cd`; `-x` with an empty command; and `-x` together with `--json`.

#### Scenario: No name
- **WHEN** the user runs `wt create`
- **THEN** stderr reports the missing `<name>` argument
- **AND** the exit code is 2

#### Scenario: Name that escapes its directory
- **WHEN** the user runs `wt create ../x -b fix`
- **THEN** stderr names `../x`
- **AND** nothing is created
- **AND** the exit code is 2

#### Scenario: Contradictory flags
- **WHEN** the user runs `wt create feat --cd --no-cd`
- **THEN** the exit code is 2
- **AND** nothing is created

#### Scenario: Command with JSON
- **WHEN** the user runs `wt create feat -x claude --json`
- **THEN** stderr is a `wt.error.v1` document with `code` 2
- **AND** nothing is created

#### Scenario: Repository without a name
- **WHEN** the user runs `wt create api:`
- **THEN** stderr reports the empty name
- **AND** nothing is created
- **AND** the exit code is 2

#### Scenario: Name without a repository
- **WHEN** the user runs `wt create :feat`
- **THEN** stderr names `:feat`
- **AND** nothing is created
- **AND** the exit code is 2

### Requirement: Creating the worktree
After every check passes and the `pre-create` hook succeeds (see the `hooks` capability), `wt create` SHALL create the branch at its base and a worktree at the path with that branch checked out, registered with git so that `wt list` shows it, and SHALL then run the `post-create` hook. Once that hook succeeds, without `--json`, it SHALL print `created worktree <path> on new branch <branch> from <base>` to stdout and exit 0. When git fails while creating, the command SHALL fail with exit code 1, include git's error message, leave behind neither the new branch nor the worktree, and SHALL NOT run the `post-create` hook. When a hook fails or is interrupted, the command SHALL leave done what the `hooks` capability specifies.

#### Scenario: Create from the main worktree
- **WHEN** in a repository whose main worktree is `/src/repo` and whose default branch is `origin/main` the user runs `wt create feat`
- **THEN** stdout contains `created worktree /src/repo.worktrees/feat on new branch feat from origin/main`
- **AND** `wt list` shows a worktree named `feat` with the branch `feat`
- **AND** the exit code is 0

#### Scenario: Git fails while creating
- **WHEN** `<parent>/blocker` is a regular file, `worktree_path` is `{repo_parent}/blocker/{name|sanitize}`, and the user runs `wt create feat`
- **THEN** stderr contains git's error message
- **AND** no branch `feat` and no worktree exist afterwards
- **AND** the exit code is 1

#### Scenario: Line after the post-create hook
- **WHEN** `hooks.post-create` is `["echo installing"]` and the user runs `wt create feat` with stdout and stderr written to the same file
- **THEN** in that file, the line `installing` comes before the line `created worktree <path> on new branch feat from <base>`

### Requirement: Running a command
With `-x <cmd>`, once the worktree is created, the `post-create` hook has succeeded, and the directive is written (when the shell moves), `wt create` SHALL run `<cmd>` with the new worktree's root as its working directory: as `sh -c <cmd>` on macOS and Linux, and as a command line of `cmd.exe` on Windows. The command SHALL use `wt`'s own standard input, output and error, and `wt` SHALL wait for it to exit. Once the command has started, `wt` SHALL exit with the command's exit code, unchanged, whatever its value; on macOS and Linux, a command terminated by a signal SHALL make `wt` exit with 128 plus the signal number. An interrupt from the terminal (Ctrl-C) SHALL reach the command and SHALL NOT end `wt` before the command exits. The command SHALL NOT receive `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR` (see the `shell-integration` capability). When the command cannot be started, `wt` SHALL fail with exit code 1. When the worktree is not created, or a hook fails or is interrupted, the command SHALL NOT run.

#### Scenario: Runs inside the worktree
- **WHEN** the user runs `wt create feat -x "git branch --show-current"`
- **THEN** stdout contains `feat`
- **AND** the exit code is 0

#### Scenario: Exit code (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt create feat -x "exit 3"`
- **THEN** the worktree is created
- **AND** the exit code is 3

#### Scenario: Exit code (Windows)
- **WHEN** on Windows the user runs `wt create feat -x "exit /b 3"`
- **THEN** the worktree is created
- **AND** the exit code is 3

#### Scenario: Interpreter operators (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt create feat -x "echo a && echo b"`
- **THEN** stdout contains `a` and `b`, each on its own line

#### Scenario: Interpreter operators (Windows)
- **WHEN** on Windows the user runs `wt create feat -x "echo a& echo b"`
- **THEN** stdout contains `a` and `b`, each on its own line

#### Scenario: Terminated by a signal (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt create feat -x 'kill -INT $$'`
- **THEN** the exit code is 130

#### Scenario: Interrupt while the command runs (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt create feat -x "sleep 1; exit 7"` and `wt` receives SIGINT while the command runs
- **THEN** `wt` exits only after the command does
- **AND** the exit code is 7

#### Scenario: The shell ends in the worktree
- **WHEN** the user runs `wt create feat -x "exit 1"` through the function
- **THEN** the function's result is 1
- **AND** the shell's working directory is the path of `feat`

#### Scenario: Protocol variables not passed
- **WHEN** the user runs `wt create feat -x <a command that records its environment>` through the function
- **THEN** the recorded environment contains neither `WT_DIRECTIVE_CD_FILE` nor `WT_PREVIOUS_DIR`

#### Scenario: Not run when creation fails
- **WHEN** the local branch `feat` exists and the user runs `wt create feat -x "touch ran"`
- **THEN** the exit code is 5
- **AND** the command did not run

#### Scenario: Not run when a hook fails
- **WHEN** `hooks.post-create` is `["exit 1"]` and the user runs `wt create feat -x "echo ran> ran.txt"`
- **THEN** the worktree `feat` exists
- **AND** the new worktree has no file `ran.txt`
- **AND** the exit code is 7

### Requirement: Dry run
`wt create --dry-run` SHALL perform every check of `wt create`, with the same errors and exit codes, and SHALL NOT fetch, create any branch, directory or worktree, write the directive, run a hook, or run the `-x` command. Because nothing is fetched, the base and the remote-tracking branches SHALL be checked against the references already present. Without `--json`, on success it SHALL print to stdout, one per line and in this order: `would fetch <remote>` when it would fetch; the lines of the `pre-create` hook; `would create worktree <path> on new branch <branch> from <base>`; the lines of the `post-create` hook; `would change directory to <path>` when it would move the shell and the shell integration is active; and `would run <cmd>` when `-x` is given. The lines of a hook SHALL be those the `hooks` capability specifies for a dry run, and there SHALL be none with `--no-hooks`. With `--json` it SHALL print the same `wt.create.v1` document as without `--dry-run`.

#### Scenario: Preview
- **WHEN** with `fetch_before_create` true and no hooks configured, through the function, the user runs `wt create feat --dry-run -x claude` in a repository whose default branch is `origin/main`
- **THEN** stdout is `would fetch origin`, `would create worktree <path> on new branch feat from origin/main`, `would change directory to <path>` and `would run claude`, in that order
- **AND** no branch `feat`, no directory at `<path>` and no worktree exist afterwards
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Preview with hooks
- **WHEN** with `fetch_before_create` false, `hooks.pre-create` is `["echo check {name}"]` and `hooks.post-create` is `["npm ci"]`, through the function, the user runs `wt create feat --dry-run -x claude`
- **THEN** stdout is `would run pre-create hook: echo check feat`, `would create worktree <path> on new branch feat from <base>`, `would run post-create hook: npm ci`, `would change directory to <path>` and `would run claude`, in that order

#### Scenario: Errors under dry run
- **WHEN** the local branch `feat` exists and the user runs `wt create feat --dry-run`
- **THEN** the exit code is 5

#### Scenario: Nothing fetched under dry run
- **WHEN** `main` on `origin` has a commit that `origin/main` does not have, and the user runs `wt create feat --dry-run`
- **THEN** `origin/main` still does not have that commit afterwards

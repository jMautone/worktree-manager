# Spec Delta

## Purpose

Defines `wt create`, the command that creates a worktree of the current repository on a new branch, at the path the configuration dictates, leaves the shell inside it, and optionally runs a command there in the foreground.

## ADDED Requirements

### Requirement: Arguments and flags
`wt create` SHALL take exactly one argument, `<name>`, and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `-b`, `--branch <branch>` | the branch to create, used as given |
| `--base <ref>` | the revision the new branch starts at |
| `-x <cmd>` | a command to run in the new worktree |
| `--cd` | move the shell into the new worktree |
| `--no-cd` | do not move the shell |

These SHALL be usage errors, with exit code 2, detected before anything is fetched, created or run: a missing `<name>` or an extra argument; an empty `<name>`; a `<name>` with a component, separated by `/` or `\`, equal to `.` or `..`; `--cd` together with `--no-cd`; `-x` with an empty command; and `-x` together with `--json`.

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

### Requirement: Repository
`wt create` SHALL create the worktree in the repository associated with the working directory (see the `git-worktrees` capability), whether that directory is in the main worktree, in a linked worktree, in a subdirectory of either, or in a bare repository. Outside a repository it SHALL fail as that capability specifies (exit 3) without creating anything.

#### Scenario: Outside a repository
- **WHEN** the user runs `wt create feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

#### Scenario: From a bare repository
- **WHEN** the working directory is a bare repository and the user runs `wt create feat`
- **THEN** a linked worktree of that repository is created with the branch `feat` checked out
- **AND** the exit code is 0

### Requirement: Branch name
The new branch SHALL be the value of `-b` when it is given, used as given; otherwise it SHALL be the value of `branch_prefix` followed by `<name>`. The branch SHALL be a name git accepts for a new branch, as written and without expanding it; otherwise the command SHALL fail with exit code 2 and a message naming the branch, and when `branch_prefix` is not empty and `-b` was not given, a hint naming `branch_prefix`.

#### Scenario: Branch from the name
- **WHEN** `branch_prefix` is empty and the user runs `wt create feature/abc1`
- **THEN** the new worktree has the branch `feature/abc1` checked out

#### Scenario: Branch prefix
- **WHEN** `branch_prefix` is `jm/` and the user runs `wt create auth` with the default `worktree_path`
- **THEN** the new worktree has the branch `jm/auth` checked out
- **AND** the last component of its path is `auth`

#### Scenario: Explicit branch ignores the prefix
- **WHEN** `branch_prefix` is `jm/` and the user runs `wt create api -b feature/api-v2`
- **THEN** the new worktree has the branch `feature/api-v2` checked out
- **AND** the last component of its path is `api`

#### Scenario: Invalid branch
- **WHEN** the user runs `wt create "a b"`
- **THEN** stderr names `a b` as an invalid branch name
- **AND** nothing is created
- **AND** the exit code is 2

#### Scenario: Name that is not a branch
- **WHEN** the user runs `wt create "my task" -b task`
- **THEN** the new worktree has the branch `task` checked out
- **AND** the last component of its path is `my task`

### Requirement: Only new branches
`wt create` SHALL only create new branches. When a local branch with the new branch's name exists, the command SHALL fail with exit code 5 and the message `branch "<branch>" already exists`; when that branch is checked out in a worktree, a hint SHALL name the command `wt cd <NAME>` for that worktree. When a remote-tracking branch `<remote>/<branch>` exists for any configured remote, the command SHALL fail with exit code 5 and a message naming the branch and the remote. In both cases nothing SHALL be created. Remote-tracking branches SHALL be checked after fetching, when `wt create` fetches (see "Fetching before creating").

#### Scenario: Local branch exists
- **WHEN** the local branch `feat` exists, is not checked out in any worktree, and the user runs `wt create feat`
- **THEN** stderr's first line is `wt: branch "feat" already exists`
- **AND** nothing is created
- **AND** the exit code is 5

#### Scenario: Branch checked out in a worktree
- **WHEN** the branch `feat` is checked out in the linked worktree named `feat` and the user runs `wt create feat`
- **THEN** stderr has a `hint:` line containing `wt cd feat`
- **AND** the exit code is 5

#### Scenario: Branch exists on a remote
- **WHEN** the remote-tracking branch `origin/feat` exists, the local branch `feat` does not, and the user runs `wt create feat`
- **THEN** stderr names `feat` and `origin`
- **AND** no local branch `feat` is created
- **AND** the exit code is 5

#### Scenario: Branch pushed after the last fetch
- **WHEN** `fetch_before_create` is true, someone pushed the branch `feat` to `origin` after the last fetch, and the user runs `wt create feat` with the base `origin/main`
- **THEN** the exit code is 5
- **AND** nothing is created

### Requirement: Base
The new branch SHALL start at the commit its base resolves to. The base SHALL be the value of `--base` when given; otherwise the value of `default_base` when it is not empty; otherwise the repository's default branch (see the `git-worktrees` capability). Any revision that resolves to a commit SHALL be accepted as a base: a local branch, a remote-tracking branch, a tag or a commit id. A base that does not resolve to a commit SHALL fail with exit code 3 and a message naming the base. When the base would be the default branch and the repository has none, the command SHALL fail with exit code 3, a message stating that there is no default branch, and hints naming `--base` and `default_base`. In both cases nothing SHALL be created.

The new branch SHALL have no upstream configured, whatever the base.

#### Scenario: Default branch of a clone
- **WHEN** the repository was cloned, `origin/HEAD` points to `origin/main`, `default_base` is empty, and the user runs `wt create feat`
- **THEN** the branch `feat` starts at the commit of `origin/main`

#### Scenario: Base from the repository configuration
- **WHEN** `.wt.toml` sets `default_base = "develop"` and the user runs `wt create feat`
- **THEN** the branch `feat` starts at the commit of `develop`

#### Scenario: Explicit base
- **WHEN** the tag `v1.0` exists and the user runs `wt create hotfix --base v1.0`
- **THEN** the branch `hotfix` starts at the commit of `v1.0`

#### Scenario: Base not found
- **WHEN** the user runs `wt create feat --base nope` and no revision `nope` exists
- **THEN** stderr names `nope`
- **AND** nothing is created
- **AND** the exit code is 3

#### Scenario: No default branch
- **WHEN** the repository has no `origin/HEAD`, its main worktree has a detached HEAD, `default_base` is empty, and the user runs `wt create feat`
- **THEN** stderr states that there is no default branch
- **AND** stderr has `hint:` lines naming `--base` and `default_base`
- **AND** the exit code is 3

#### Scenario: No upstream
- **WHEN** the user runs `wt create feat --base origin/main`
- **THEN** the branch `feat` has no upstream configured

### Requirement: Fetching before creating
When `fetch_before_create` is true and the base, as written, starts with the name of a configured remote followed by `/`, `wt create` SHALL fetch that remote before resolving the base and before checking remote-tracking branches. When more than one remote name matches, the longest SHALL win. When fetching fails, `wt create` SHALL write a warning naming the remote and SHALL continue with the references it already has. When the base starts with no remote name, or `fetch_before_create` is false, nothing SHALL be fetched.

#### Scenario: Remote advanced since the last fetch
- **WHEN** `main` on `origin` has a commit that `origin/main` does not have yet, `default_base` is empty, and the user runs `wt create feat`
- **THEN** the branch `feat` starts at that new commit

#### Scenario: Base that only exists on the remote
- **WHEN** someone pushed the branch `release` to `origin` after the last fetch and the user runs `wt create fix --base origin/release`
- **THEN** the branch `fix` starts at the commit of `release` on `origin`

#### Scenario: Unreachable remote
- **WHEN** `origin` cannot be reached, `origin/main` exists locally, and the user runs `wt create feat --base origin/main`
- **THEN** stderr contains a warning naming `origin`
- **AND** the branch `feat` starts at the local commit of `origin/main`
- **AND** the exit code is 0

#### Scenario: Local base
- **WHEN** the user runs `wt create feat --base main`
- **THEN** no remote is fetched

#### Scenario: Fetching disabled
- **WHEN** `WT_FETCH_BEFORE_CREATE` is `false` and the user runs `wt create feat --base origin/main`
- **THEN** no remote is fetched
- **AND** the branch `feat` starts at the local commit of `origin/main`

### Requirement: Worktree path
The new worktree's path SHALL be the value of `worktree_path` rendered as the `path-templates` capability specifies. When anything exists at that path (a file, a directory, even an empty one, or a symbolic link), the command SHALL fail with exit code 5 and the message `path already exists: <path>`. When git has a worktree registered at that path, even if its directory no longer exists, the command SHALL fail with exit code 5, a message naming the path, and a hint naming `git worktree prune`. In both cases nothing SHALL be created. Missing parent directories of the path SHALL be created.

#### Scenario: Directory already there
- **WHEN** the directory `/src/repo.worktrees/feat` exists and the user runs `wt create feat` with the default `worktree_path`
- **THEN** stderr's first line is `wt: path already exists: /src/repo.worktrees/feat`
- **AND** no branch `feat` is created
- **AND** the exit code is 5

#### Scenario: Letter case differs (macOS, Windows)
- **WHEN** on macOS or Windows the directory `<parent>/repo.worktrees/feat` exists and the user runs `wt create Feat`
- **THEN** the exit code is 5

#### Scenario: Letter case differs (Linux)
- **WHEN** on Linux the directory `<parent>/repo.worktrees/feat` exists and the user runs `wt create Feat`
- **THEN** a worktree is created at `<parent>/repo.worktrees/Feat`
- **AND** the exit code is 0

#### Scenario: Stale registration
- **WHEN** a worktree at `<parent>/repo.worktrees/feat` was deleted from disk without `git worktree remove`, and the user runs `wt create feat -b feat2`
- **THEN** stderr names the path and has a `hint:` line naming `git worktree prune`
- **AND** the exit code is 5

#### Scenario: Missing parent directories
- **WHEN** `worktree_path` is `{repo_parent}/wt/{repo}/{name|sanitize}` and `<parent>/wt` does not exist
- **THEN** `wt create feat` creates `<parent>/wt/repo/feat`
- **AND** the exit code is 0

### Requirement: Creating the worktree
After every check passes, `wt create` SHALL create the branch at its base and a worktree at the path with that branch checked out, registered with git so that `wt list` shows it. Without `--json`, it SHALL print `created worktree <path> on new branch <branch> from <base>` to stdout and exit 0. When git fails while creating, the command SHALL fail with exit code 1, include git's error message, and leave behind neither the new branch nor the worktree.

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

### Requirement: Moving the shell
`wt create` SHALL move the shell into the new worktree when `--cd` is given, or when `create_cd` is true and `--no-cd` is not given. When it moves the shell and the shell integration is active (see the `shell-integration` capability), it SHALL write the new worktree's path as the directive, so that the shell ends in the worktree's root. When it moves the shell and the shell integration is not active, it SHALL still create the worktree, SHALL write a warning stating that the shell integration is not active, and SHALL exit as it would otherwise. When it does not move the shell, it SHALL write no directive and no such warning. When the directive cannot be written, the command SHALL fail with exit code 1, and the worktree SHALL remain created.

#### Scenario: Through the function
- **WHEN** the shell is in a subdirectory of the main worktree and the user runs `wt create feat` through the function
- **THEN** the shell's working directory is the path of the new worktree
- **AND** the exit code is 0

#### Scenario: Staying put
- **WHEN** the user runs `wt create feat --no-cd` through the function
- **THEN** the worktree is created
- **AND** the shell's working directory does not change

#### Scenario: create_cd off
- **WHEN** the user file sets `create_cd = false` and the user runs `wt create feat` through the function
- **THEN** the shell's working directory does not change
- **AND** running `wt create fix --cd` moves the shell into the worktree `fix`

#### Scenario: Without the function
- **WHEN** the user runs `git-wt create feat` in a shell that did not load the script
- **THEN** the worktree is created
- **AND** stderr contains a warning stating that the shell integration is not active
- **AND** the exit code is 0

### Requirement: Running a command
With `-x <cmd>`, once the worktree is created and the directive written (when the shell moves), `wt create` SHALL run `<cmd>` with the new worktree's root as its working directory: as `sh -c <cmd>` on macOS and Linux, and as a command line of `cmd.exe` on Windows. The command SHALL use `wt`'s own standard input, output and error, and `wt` SHALL wait for it to exit. Once the command has started, `wt` SHALL exit with the command's exit code, unchanged, whatever its value; on macOS and Linux, a command terminated by a signal SHALL make `wt` exit with 128 plus the signal number. An interrupt from the terminal (Ctrl-C) SHALL reach the command and SHALL NOT end `wt` before the command exits. The command SHALL NOT receive `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR` (see the `shell-integration` capability). When the command cannot be started, `wt` SHALL fail with exit code 1. When the worktree is not created, the command SHALL NOT run.

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

### Requirement: JSON output
`wt create <name> --json` SHALL create the worktree and move the shell like `wt create <name>`, and SHALL print `{"schema":"wt.create.v1","name":...,"path":...,"branch":...,"base":...,"head":...}` instead of the `created worktree` line, where `name` is `<name>` as given, `path` the new worktree's absolute path in the operating system's native form, `branch` the new branch, `base` the base as written, and `head` the full commit id the branch starts at.

#### Scenario: Created worktree as JSON
- **WHEN** the user runs `wt create feat --json` in a repository whose default branch is `origin/main`
- **THEN** stdout parses as a JSON object with `schema` `wt.create.v1`, `name` `feat`, `branch` `feat` and `base` `origin/main`
- **AND** `head` is the full commit id of `origin/main`
- **AND** `path` is the path of the new worktree

### Requirement: Dry run
`wt create --dry-run` SHALL perform every check of `wt create`, with the same errors and exit codes, and SHALL NOT fetch, create any branch, directory or worktree, write the directive, or run the `-x` command. Because nothing is fetched, the base and the remote-tracking branches SHALL be checked against the references already present. Without `--json`, on success it SHALL print to stdout, one per line and in this order: `would fetch <remote>` when it would fetch; `would create worktree <path> on new branch <branch> from <base>`; `would change directory to <path>` when it would move the shell and the shell integration is active; and `would run <cmd>` when `-x` is given. With `--json` it SHALL print the same `wt.create.v1` document as without `--dry-run`.

#### Scenario: Preview
- **WHEN** with `fetch_before_create` true, through the function, the user runs `wt create feat --dry-run -x claude` in a repository whose default branch is `origin/main`
- **THEN** stdout is `would fetch origin`, `would create worktree <path> on new branch feat from origin/main`, `would change directory to <path>` and `would run claude`, in that order
- **AND** no branch `feat`, no directory at `<path>` and no worktree exist afterwards
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Errors under dry run
- **WHEN** the local branch `feat` exists and the user runs `wt create feat --dry-run`
- **THEN** the exit code is 5

#### Scenario: Nothing fetched under dry run
- **WHEN** `main` on `origin` has a commit that `origin/main` does not have, and the user runs `wt create feat --dry-run`
- **THEN** `origin/main` still does not have that commit afterwards

### Requirement: Working directory override
With `-C <dir>`, `wt create` SHALL create the worktree in the repository associated with `<dir>`, SHALL read `.wt.toml` from the worktree that contains `<dir>`, and SHALL resolve relative paths of `worktree_path` against that repository's main worktree.

#### Scenario: Create in another repository
- **WHEN** the shell is in repository `a` and the user runs `wt -C <path of repository b> create feat` through the function
- **THEN** the new worktree belongs to repository `b`
- **AND** the shell's working directory is that worktree

### Requirement: Completions
Completions for `wt create` SHALL offer, for the value of `--base`, the local branches of the repository associated with the shell's working directory and its remote-tracking branches written `<remote>/<branch>`, except `<remote>/HEAD`. They SHALL offer nothing for `<name>` and for the value of `-b`, and never file names. Requesting a completion SHALL never write to the terminal: outside a repository, or when git fails, nothing is offered.

#### Scenario: Bases
- **WHEN** in a clone with the local branch `main` and the remote-tracking branch `origin/main`, the user requests completions for `wt create feat --base `
- **THEN** `main` and `origin/main` are offered
- **AND** `origin/HEAD` is not offered

#### Scenario: Bases outside a repository
- **WHEN** outside any repository the user requests completions for `wt create feat --base `
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

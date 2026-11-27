## Purpose

Defines `wt step`, which runs one step of the pipeline of `wt merge` on its own (commit, squash, rebase or push), and the commit message that `wt step commit`, `wt step squash` and `wt merge` share.

## ADDED Requirements

### Requirement: Steps
`wt step` SHALL take exactly one subcommand: `commit`, `squash`, `rebase` or `push`. `wt step` with no subcommand SHALL print its help to stderr and exit 2; an unknown subcommand SHALL be a usage error (exit 2).

#### Scenario: No subcommand
- **WHEN** the user runs `wt step`
- **THEN** the exit code is 2

#### Scenario: Unknown subcommand
- **WHEN** the user runs `wt step fixup`
- **THEN** stderr names `fixup`
- **AND** the exit code is 2

### Requirement: Arguments and flags of the steps
The steps SHALL accept these arguments and flags, besides the global ones. Any other argument SHALL be a usage error (exit 2), detected before anything is read.

| Step | Argument | Flags |
|---|---|---|
| `commit` | none | `-m <msg>`, `--no-hooks` |
| `squash` | `[<target>]` | `-m <msg>`, `--no-hooks` |
| `rebase` | `[<target>]` | none |
| `push` | `[<target>]` | `--no-hooks` |

#### Scenario: Commit takes no target
- **WHEN** the user runs `wt step commit main`
- **THEN** the exit code is 2

#### Scenario: Rebase runs no hook
- **WHEN** the user runs `wt step rebase --no-hooks`
- **THEN** stderr names `--no-hooks` as an unknown flag
- **AND** the exit code is 2

### Requirement: Worktree of a step
A step SHALL operate on the current worktree, as `wt merge` does (see the `merge-worktree` capability, "Worktree merged"): outside a repository it SHALL fail with exit 3, in the main worktree with exit 2 and `cannot run wt step in the main worktree`, with a detached HEAD with exit 5, and with a rebase, merge, cherry-pick or revert in progress with exit 5. Nothing SHALL change.

#### Scenario: Main worktree
- **WHEN** the user runs `wt step commit` in the main worktree
- **THEN** stderr's first line is `wt: cannot run wt step in the main worktree`
- **AND** the exit code is 2

#### Scenario: Rebase in progress
- **WHEN** the worktree `feat` has a rebase stopped on a conflict and from inside it the user runs `wt step squash`
- **THEN** stderr states that a rebase is in progress
- **AND** the exit code is 5

### Requirement: Target of a step
The target of `squash`, `rebase` and `push` SHALL be chosen and checked as the target of `wt merge` (see the `merge-worktree` capability, "Target"): `<target>` or the base without its remote, a local branch (exit 3 otherwise), and not the worktree's own branch (exit 2). Nothing SHALL change.

#### Scenario: Default target
- **WHEN** `origin/HEAD` points to `origin/main`, `default_base` is empty, and from inside `feat` the user runs `wt step rebase`
- **THEN** `feat` is rebased onto the local branch `main`

#### Scenario: Target not found
- **WHEN** from inside `feat` the user runs `wt step squash nope` and there is no local branch `nope`
- **THEN** stderr names `nope`
- **AND** the exit code is 3

### Requirement: Pending changes
The pending changes of a worktree SHALL be its changes to tracked files, staged or not, and its untracked files that are not ignored, as the `remove-worktree` capability counts them. A step that commits them SHALL commit all of them; ignored files SHALL never be committed.

#### Scenario: Untracked and ignored files
- **WHEN** `.gitignore` in `feat` lists `*.log`, `feat` has the untracked files `new.txt` and `debug.log`, and from inside `feat` the user runs `wt step commit -m "add new"`
- **THEN** the new commit contains `new.txt`
- **AND** the new commit does not contain `debug.log`
- **AND** `debug.log` is still in the worktree

### Requirement: Committing pending changes
`wt step commit` SHALL commit the worktree's pending changes on its branch, as one new commit whose parent is the previous HEAD, with the message of "Commit message", and SHALL print `committed <short id> to <branch>`. With no pending changes it SHALL print `nothing to commit`, run no hook and exit 0.

#### Scenario: Commit with a message
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt step commit -m "fix login"`
- **THEN** stdout is `committed <short id> to feat`
- **AND** the branch `feat` has a new commit with the message `fix login`, whose parent is the commit it pointed to before
- **AND** `feat` has no pending changes
- **AND** the exit code is 0

#### Scenario: Nothing to commit
- **WHEN** `feat` has no pending changes and from inside it the user runs `wt step commit`
- **THEN** stdout is `nothing to commit`
- **AND** the exit code is 0

### Requirement: Squashing the branch
`wt step squash` SHALL replace the commits of the branch that the target does not contain, and its pending changes, by one commit whose parent is the merge base of the branch and the target, whose files are those of the worktree, and whose message is that of "Commit message". It SHALL print `squashed <n> commits of <branch> into <short id>`, where `<n>` counts the commits replaced.

#### Scenario: Three commits
- **WHEN** `feat` has three commits that `main` does not contain, no pending changes, and from inside `feat` the user runs `wt step squash -m "add login"`
- **THEN** stdout is `squashed 3 commits of feat into <short id>`
- **AND** `feat` has one commit that `main` does not contain, with the message `add login`
- **AND** the files of `feat`'s HEAD are those of the commit it pointed to before

#### Scenario: Commits and pending changes
- **WHEN** `feat` has two commits that `main` does not contain and a modified tracked file, and from inside `feat` the user runs `wt step squash -m "x"`
- **THEN** `feat` has one commit that `main` does not contain, which includes the modification
- **AND** `feat` has no pending changes

### Requirement: Nothing to squash
When the branch has no commit that the target does not contain and no pending changes, or exactly one such commit and no pending changes, `wt step squash` SHALL print `nothing to squash`, run no hook, change nothing and exit 0. When it has no such commit but has pending changes, it SHALL commit them as `wt step commit` does, with its line.

#### Scenario: One commit
- **WHEN** `feat` has exactly one commit that `main` does not contain, no pending changes, and from inside `feat` the user runs `wt step squash`
- **THEN** stdout is `nothing to squash`
- **AND** `feat` points to the commit it pointed to before

### Requirement: Commit message
The message of a commit made by `wt step commit`, `wt step squash` or `wt merge` SHALL be, when `-m <msg>` is given, `<msg>` as written. Without `-m`, git's editor (the one `git commit` would open) SHALL open in the terminal, and the message SHALL be what the user saves, after git's cleanup of comment lines and blank lines.

#### Scenario: Message from the editor
- **WHEN** git's editor is a command that writes `from editor` into the file it is given, `feat` has a modified tracked file, the standard input is a terminal, and from inside `feat` the user runs `wt step commit`
- **THEN** the new commit's message is `from editor`

### Requirement: Initial message of a squash
Without `-m`, the editor of `wt step squash`, and of `wt merge --squash`, SHALL open with the full messages of the commits being replaced, oldest first, separated by a blank line, after a comment line naming how many commits are squashed. With no commit replaced, it SHALL open as for `wt step commit`.

#### Scenario: Messages of the branch
- **WHEN** `feat` has the commits `add form` and then `fix typo`, which `main` does not contain, git's editor is a command that copies the file it is given to `<sandbox>/msg.txt`, and from inside `feat` the user runs `wt step squash` in a terminal
- **THEN** `msg.txt` contains `add form` before `fix typo`

### Requirement: Empty message
When the message is empty after git's cleanup, the commit SHALL NOT be made: the command SHALL fail with exit code 1 and the message `empty commit message; nothing committed`, and the branch, the index and the worktree's files SHALL be as they were before the command.

#### Scenario: Editor saves nothing
- **WHEN** git's editor is a command that empties the file it is given, `feat` has a modified tracked file, and from inside `feat` the user runs `wt step commit` in a terminal
- **THEN** stderr's first line is `wt: empty commit message; nothing committed`
- **AND** `feat` points to the commit it pointed to before, and the file is still modified and not staged
- **AND** the exit code is 1

### Requirement: Message without a terminal
When a message is needed, `-m` is not given, and the standard input is not a terminal or `--json` is given, the command SHALL fail with exit code 2, the message `a commit message is needed`, and a hint naming `-m`, before any hook runs and with nothing changed.

#### Scenario: Script without a message
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt step commit < /dev/null`
- **THEN** stderr's first line is `wt: a commit message is needed`
- **AND** the exit code is 2

#### Scenario: No message needed
- **WHEN** `feat` has no pending changes and from inside `feat` the user runs `wt step commit --json < /dev/null`
- **THEN** the exit code is 0

### Requirement: Failed commit
When git fails to make the commit (a native git hook of the repository, such as its `pre-commit` or `commit-msg`, exits with an error), the command SHALL fail with exit code 1 and git's message, and the branch, the index and the worktree's files SHALL be as they were before the command.

#### Scenario: Native hook rejects the commit
- **WHEN** the repository's git hook `commit-msg` exits with 1, `feat` has a staged change to `a.txt` and an unstaged change to `b.txt`, and from inside `feat` the user runs `wt step commit -m "x"`
- **THEN** `feat` points to the commit it pointed to before
- **AND** the change to `a.txt` is staged and the change to `b.txt` is not
- **AND** the exit code is 1

#### Scenario: Squash rejected
- **WHEN** the repository's git hook `commit-msg` exits with 1, `feat` has three commits that `main` does not contain, and from inside `feat` the user runs `wt step squash -m "x"`
- **THEN** `feat` points to the commit it pointed to before
- **AND** the exit code is 1

### Requirement: Interrupt while committing
An interrupt from the terminal (Ctrl-C) while the editor is open or git is committing SHALL reach them and SHALL NOT end `wt` before git exits; `wt` SHALL then leave the branch, the index and the worktree's files as they were before the command, and exit 130. During a hook, the `hooks` capability applies.

#### Scenario: Interrupt in the editor (macOS, Linux)
- **WHEN** on macOS or Linux git's editor is a command that waits, `feat` has three commits that `main` does not contain, the user runs `wt step squash` from inside `feat`, and `wt` receives SIGINT while the editor waits
- **THEN** `feat` points to the commit it pointed to before
- **AND** the exit code is 130

### Requirement: Rebasing as a step
`wt step rebase` SHALL rebase the branch onto the target exactly as `wt merge` does (see the `merge-worktree` capability, "Rebasing onto the target", "Conflict while rebasing" and "Interrupt during a merge"), with the same line, conflict handling, exit codes and interrupt, and nothing else. When the branch already contains the target, it SHALL print `<branch> already contains <target>` and exit 0.

#### Scenario: Rebased
- **WHEN** `main` has a commit `feat` does not contain and from inside the clean worktree `feat` the user runs `wt step rebase`
- **THEN** stdout is `rebased feat onto main`
- **AND** `feat` contains `main`'s commit
- **AND** `main` points to the commit it pointed to before

#### Scenario: Conflict
- **WHEN** `feat` and `main` change the same line of a file and from inside `feat` the user runs `wt step rebase`
- **THEN** `feat` points to the commit it pointed to before and has no rebase in progress
- **AND** the exit code is 6

### Requirement: State checked before rebasing
Before anything changes, `wt step rebase` SHALL fail with exit code 5 when the worktree has changes to tracked files, with a hint naming `wt step commit`. Untracked files SHALL NOT block it.

#### Scenario: Modified file
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt step rebase`
- **THEN** the exit code is 5
- **AND** `feat` points to the commit it pointed to before

#### Scenario: Untracked file
- **WHEN** `main` has a commit `feat` does not contain, `feat` has only the untracked file `notes.txt`, and from inside `feat` the user runs `wt step rebase`
- **THEN** stdout is `rebased feat onto main`
- **AND** `notes.txt` is still in the worktree

### Requirement: Pushing as a step
`wt step push` SHALL run `pre-merge`, fast-forward the target to the worktree's HEAD as `wt merge` does (see the `merge-worktree` capability, "Moving the target"), with the same lines, and then run `post-merge`. It SHALL NOT rebase, remove anything, or contact any remote.

#### Scenario: Target moved, worktree kept
- **WHEN** `feat` contains `main` and has a commit of its own, and from inside the clean worktree `feat` the user runs `wt step push`
- **THEN** stdout is `merged feat into main`
- **AND** `main` points to `feat`'s HEAD
- **AND** the worktree `feat` and the branch `feat` still exist

### Requirement: State checked before pushing
Before anything runs or moves, `wt step push` SHALL fail with exit code 5 when the worktree has changes to tracked files (with a hint naming `wt step commit`), when the target is checked out in a worktree with changes to tracked files, or when the branch does not contain the target (with the message `<branch> does not contain <target>` and a hint naming `wt step rebase`).

#### Scenario: Behind the target
- **WHEN** `main` has a commit `feat` does not contain and from inside the clean worktree `feat` the user runs `wt step push`
- **THEN** stderr's first line is `wt: feat does not contain main`
- **AND** stderr has a `hint:` line naming `wt step rebase`
- **AND** the exit code is 5

### Requirement: Hooks of the steps
`wt step commit` and `wt step squash` SHALL fire `pre-commit` and `post-commit`, and `wt step push` SHALL fire `pre-merge` and `post-merge` (see the `hooks` capability); `wt step rebase` SHALL fire none. A step that has nothing to do SHALL fire none. `--no-hooks` SHALL make the step fire no event.

#### Scenario: pre-commit fails
- **WHEN** `hooks.pre-commit` is `["exit 1"]`, `feat` has a modified tracked file, and from inside `feat` the user runs `wt step commit -m "x"`
- **THEN** `feat` points to the commit it pointed to before, and the file is still modified
- **AND** the exit code is 7

### Requirement: Dry run of the steps
Under `--dry-run`, a step SHALL make every check, with the same errors and exit codes, read no message and open no editor, and change nothing. It SHALL print the lines of its blocking hook, then `would commit pending changes to <branch>`, `would squash <n> commits of <branch>`, `would rebase <branch> onto <target>` or `would merge <branch> into <target>`, then the lines of its background hook; or the line it prints when there is nothing to do.

#### Scenario: Preview of a squash
- **WHEN** `feat` has three commits that `main` does not contain, no hook is configured, and from inside `feat` the user runs `wt step squash --dry-run`
- **THEN** stdout is `would squash 3 commits of feat`
- **AND** `feat` points to the commit it pointed to before

#### Scenario: Dry run needs no message
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt step commit --dry-run < /dev/null`
- **THEN** stdout is `would commit pending changes to feat`
- **AND** the exit code is 0

### Requirement: JSON output of the steps
With `--json`, a step SHALL print `{"schema":"wt.step.v1","step":...,"name":...,"path":...,"branch":...,"target":...,"changed":...,"head":...}`, where `step` is the subcommand, `target` is empty for `commit`, `changed` says whether the step made a commit, rebased or moved the target, and `head` is the full commit id of the branch afterwards, or for `push` of the target; under `--dry-run`, what would be done.

#### Scenario: Commit as JSON
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt step commit -m "x" --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.step.v1`, `step` `commit`, `branch` `feat`, `target` `""` and `changed` `true`

### Requirement: Completions of the steps
Completions after `wt step ` SHALL offer `commit`, `squash`, `rebase` and `push`. For `<target>` of `squash`, `rebase` and `push`, they SHALL offer what `wt merge` offers (see the `merge-worktree` capability, "Completions of merge"), and never file names. Requesting a completion SHALL never write to the terminal.

#### Scenario: Subcommands
- **WHEN** the user requests completions for `wt step `
- **THEN** `commit`, `squash`, `rebase` and `push` are offered

#### Scenario: Targets of a squash
- **WHEN** a repository has the local branches `main`, `develop` and `feat`, and from inside the worktree `feat` the user requests completions for `wt step squash `
- **THEN** `main` and `develop` are offered
- **AND** `feat` is not offered

# Spec Delta

## Purpose

Defines `wt merge`, the command that closes a worktree from inside it: it checks the worktree is clean, rebases its branch onto the target, runs the `pre-merge` hook, fast-forwards the target to the branch, removes the worktree and its branch, and runs `post-merge`, stopping at the first step that fails with nothing left half done.

## ADDED Requirements

### Requirement: Arguments and flags of merge
`wt merge` SHALL take at most one argument, `<target>`, and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `--no-rebase` | do not rebase; the branch must already contain the target |
| `--no-remove` | keep the worktree and its branch once merged |
| `--no-hooks` | run no hook (see the `hooks` capability) |

An extra argument SHALL be a usage error (exit 2), detected before anything is read.

#### Scenario: Two targets
- **WHEN** the user runs `wt merge main develop`
- **THEN** the exit code is 2

### Requirement: Worktree merged
`wt merge` SHALL merge the current worktree (see the `git-worktrees` capability). Outside a repository it SHALL fail as `wt list` does (exit 3). In a main worktree it SHALL fail with exit code 2 and the message `cannot merge the main worktree`. In a worktree with a detached HEAD it SHALL fail with exit code 5 and a message stating that HEAD is detached. In every case nothing SHALL change.

#### Scenario: Main worktree
- **WHEN** the user runs `wt merge` in the main worktree
- **THEN** stderr's first line is `wt: cannot merge the main worktree`
- **AND** the exit code is 2

#### Scenario: Working directory override
- **WHEN** the shell is in the main worktree and the user runs `wt -C <path of feat> merge`
- **THEN** the worktree `feat` is merged

### Requirement: Target
The target SHALL be a local branch: `<target>` when given; otherwise the base of the repository, as the `git-worktrees` capability defines it in "Merged branch", without its `<remote>/` prefix when it names a remote-tracking branch, so that `origin/main` gives `main`. A target that is not a local branch SHALL fail with exit code 3 and a message naming it; a target that is the worktree's own branch, with exit code 2. Nothing SHALL change.

#### Scenario: Default target
- **WHEN** `origin/HEAD` points to `origin/main`, `default_base` is empty, and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** the local branch `main` is the target

#### Scenario: Target not found
- **WHEN** from inside `feat` the user runs `wt merge nope` and there is no local branch `nope`
- **THEN** stderr names `nope`
- **AND** the exit code is 3

### Requirement: State checked before merging
Before anything changes, `wt merge` SHALL fail with exit code 5 when the worktree has changes to tracked files or untracked files (as the `remove-worktree` capability counts them), when it has a rebase, merge, cherry-pick or revert in progress, when it is locked and `--no-remove` is not given, or when the target is checked out in a worktree with changes to tracked files. The message SHALL state which, and nothing SHALL change.

#### Scenario: Uncommitted change
- **WHEN** a tracked file in `feat` was modified and from inside `feat` the user runs `wt merge`
- **THEN** the target and the branch `feat` point to the commits they pointed to before
- **AND** the exit code is 5

#### Scenario: Target checked out with changes
- **WHEN** the target `main` is checked out in the main worktree, which has a modified tracked file, and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** the exit code is 5
- **AND** the branch `feat` was not rebased

### Requirement: Order of the checks
`wt merge` SHALL make every check of "Worktree merged", "Target" and "State checked before merging", and SHALL check the approvals of the repository's hooks (see the `project-hooks-approval` capability), before it rebases, runs a hook or moves anything.

#### Scenario: Hook not approved
- **WHEN** `.wt.toml` sets `pre-merge = ["npm test"]`, not approved, and from inside the clean worktree `feat`, behind its target, the user runs `wt merge`
- **THEN** the branch `feat` was not rebased
- **AND** the exit code is 8

### Requirement: Rebasing onto the target
Unless `--no-rebase` is given, when the target's commit is not the worktree's HEAD or an ancestor of it, `wt merge` SHALL rebase the branch onto the target as the `sync-worktrees` capability rebases a worktree, never stashing, and SHALL print `rebased <branch> onto <target>`.

#### Scenario: Behind the target
- **WHEN** `main` has a commit that `feat` does not contain and from inside the clean worktree `feat` the user runs `wt merge --no-remove`
- **THEN** stdout's first line is `rebased feat onto main`
- **AND** `main` contains `feat`'s own commits

### Requirement: Conflict while rebasing
When that rebase stops on a conflict, `wt merge` SHALL abort it, leaving the worktree as it was, and SHALL fail with exit code 6, the message `conflict rebasing <branch> onto <target>; rebase aborted`, and a hint naming `git rebase <target>`. When git stops it for another reason, it SHALL abort it too and fail with exit code 1. In both cases no hook SHALL run and the target SHALL NOT move.

#### Scenario: Conflict
- **WHEN** `feat` and `main` change the same line of a file and from inside `feat` the user runs `wt merge`
- **THEN** stderr's first line is `wt: conflict rebasing feat onto main; rebase aborted`
- **AND** `feat` points to the commit it pointed to before and has no rebase in progress
- **AND** `main` points to the commit it pointed to before
- **AND** the exit code is 6

### Requirement: Merging without rebasing
With `--no-rebase`, when the target's commit is not the worktree's HEAD or an ancestor of it, `wt merge` SHALL fail with exit code 5, the message `<branch> does not contain <target>`, and a hint naming `--no-rebase`, before any hook runs and with nothing changed.

#### Scenario: Behind without rebasing
- **WHEN** `main` has a commit that `feat` does not contain and from inside `feat` the user runs `wt merge --no-rebase`
- **THEN** stderr's first line is `wt: feat does not contain main`
- **AND** the exit code is 5

### Requirement: Moving the target
Once `pre-merge` succeeds, `wt merge` SHALL fast-forward the target to the worktree's HEAD and print `merged <branch> into <target>`. When the target is checked out in a worktree, that worktree's files SHALL be updated to match, as a fast-forward merge there does. When the target already contains the branch, it SHALL print `<branch> is already in <target>` and go on.

#### Scenario: Target checked out in the main worktree
- **WHEN** `main` is checked out in the clean main worktree, `feat` has a commit that adds `f.txt`, and from inside `feat` the user runs `wt merge`
- **THEN** stdout contains `merged feat into main`
- **AND** the main worktree has the file `f.txt`

#### Scenario: Nothing to merge
- **WHEN** the user runs `wt create feat`, and then from inside `feat`, with no commit of its own, `wt merge`
- **THEN** stdout contains `feat is already in main`
- **AND** the exit code is 0

### Requirement: Removing after the merge
Unless `--no-remove` is given, once the target is moved `wt merge` SHALL remove the worktree as `wt remove` does (see the `remove-worktree` capability), with its hooks, its lines on stdout and its move of the shell to the main worktree, and SHALL delete its branch, which the target now contains, whatever the base. With `--no-remove`, the worktree and its branch SHALL stay.

#### Scenario: Merged and removed
- **WHEN** `default_base` is `origin/main`, `origin/main` does not contain `feat`, and from inside the clean worktree `feat` the user runs `wt merge main` through the function
- **THEN** stdout contains `merged feat into main`, `removed worktree <path of feat>` and `deleted branch feat`, in that order
- **AND** the shell's working directory is the main worktree's path
- **AND** the exit code is 0

#### Scenario: Keeping the worktree
- **WHEN** from inside the clean worktree `feat` the user runs `wt merge --no-remove`
- **THEN** the worktree `feat` and the branch `feat` still exist
- **AND** the shell's working directory does not change

### Requirement: Failure after the target moved
When a step after the target moved fails (`pre-remove`, or removing the worktree), the target SHALL stay moved, the command SHALL exit as `wt remove` would, and a hint SHALL state that the branch was merged into the target.

#### Scenario: pre-remove fails after the merge
- **WHEN** `hooks.pre-remove` is `["exit 1"]` and from inside the clean worktree `feat`, with a commit of its own, the user runs `wt merge`
- **THEN** `main` contains `feat`'s commit
- **AND** the worktree `feat` still exists
- **AND** the exit code is 7

### Requirement: Interrupt during a merge
An interrupt from the terminal (Ctrl-C) during the rebase SHALL reach git and SHALL NOT end `wt` before git exits; `wt merge` SHALL then abort a rebase left in progress, SHALL NOT move the target, and SHALL exit 130. During a hook, the `hooks` capability applies.

#### Scenario: Interrupt during the rebase (macOS, Linux)
- **WHEN** on macOS or Linux `wt` receives SIGINT while `wt merge` rebases `feat`
- **THEN** `feat` has no rebase in progress
- **AND** the target points to the commit it pointed to before
- **AND** the exit code is 130

### Requirement: Dry run of merge
`wt merge --dry-run` SHALL make every check of `wt merge`, with the same errors and exit codes, and SHALL NOT rebase, run a hook, move the target, remove anything or write the directive. It SHALL print, in order: `would rebase <branch> onto <target>` when it would; the lines of `pre-merge` (see the `hooks` capability); `would merge <branch> into <target>`; then, unless `--no-remove`, the lines of `wt remove --dry-run`; and the lines of `post-merge`.

#### Scenario: Preview
- **WHEN** `main` has a commit `feat` does not contain, no hook is configured, and from inside the clean worktree `feat` the user runs `wt merge --dry-run`
- **THEN** stdout starts with `would rebase feat onto main` and `would merge feat into main`
- **AND** `feat` and `main` point to the commits they pointed to before

### Requirement: JSON output of merge
With `--json`, `wt merge` SHALL print `{"schema":"wt.merge.v1","name":...,"path":...,"branch":...,"target":...,"rebased":...,"head":...,"removed":...}`, where `rebased` says whether the branch was rebased, `head` is the target's full commit id afterwards, and `removed` whether the worktree was removed; under `--dry-run`, what would be done.

#### Scenario: Merged as JSON
- **WHEN** from inside the clean worktree `feat` the user runs `wt merge --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.merge.v1`, `branch` `feat`, `target` `main` and `removed` `true`

### Requirement: Completions of merge
Completions for `<target>` SHALL offer the local branches of the repository associated with the shell's working directory, except the current worktree's branch, and never file names. Requesting a completion SHALL never write to the terminal.

#### Scenario: Targets
- **WHEN** a repository has the local branches `main`, `develop` and `feat`, and from inside the worktree `feat` the user requests completions for `wt merge `
- **THEN** `main` and `develop` are offered
- **AND** `feat` is not offered

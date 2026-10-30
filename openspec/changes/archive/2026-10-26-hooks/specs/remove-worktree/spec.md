## MODIFIED Requirements

### Requirement: Arguments and flags of remove
`wt remove` SHALL take exactly one argument, `<target>`, and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `--keep-branch` | keep the worktree's branch, even if it is merged |
| `-D`, `--force-delete-branch` | delete the worktree's branch, even if it is not merged |
| `-f`, `--force` | remove the worktree even if it has modified or untracked files |
| `--no-hooks` | run no hook (see the `hooks` capability) |

A missing `<target>`, an extra argument, and `--keep-branch` together with `-D` SHALL be usage errors, with exit code 2, detected before anything is removed.

#### Scenario: No target
- **WHEN** the user runs `wt remove`
- **THEN** stderr reports the missing `<target>` argument
- **AND** the exit code is 2

#### Scenario: Contradictory branch flags
- **WHEN** the user runs `wt remove feat --keep-branch -D`
- **THEN** the exit code is 2
- **AND** the worktree `feat` still exists

#### Scenario: Remove without hooks
- **WHEN** `hooks.pre-remove` is `["exit 1"]` and the user runs `wt remove feat --no-hooks`
- **THEN** the worktree `feat` no longer exists
- **AND** the exit code is 0

### Requirement: Removing the worktree
Once every check passes and the `pre-remove` hook succeeds (see the `hooks` capability), `wt remove` SHALL delete the worktree's directory and git's record of it, so that `wt list` no longer shows it; when the directory no longer exists, it SHALL only remove git's record. Without `--json`, it SHALL print to stdout `removed worktree <path>`, followed, when the worktree had a branch, by one of `deleted branch <branch>`, `kept branch <branch>` (with `--keep-branch`), or `kept branch <branch>: not merged into <base>` (`kept branch <branch>: not merged` when there is no base to name, which includes a base that does not resolve to a commit), and SHALL exit 0. Once the worktree is removed, its branch deleted or kept, and the directive written (see "Moving the shell"), `wt remove` SHALL start the `post-remove` hook.

When git fails while removing the worktree, the command SHALL fail with exit code 1 and include git's message, with a hint naming the directory when it still exists; it SHALL NOT delete the branch, move the shell, or start the `post-remove` hook. When git fails while deleting the branch, the command SHALL fail with exit code 1 and include git's message; the worktree stays removed, and the `post-remove` hook SHALL still start. When the `pre-remove` hook fails or is interrupted, the command SHALL leave done what the `hooks` capability specifies.

#### Scenario: Remove a linked worktree
- **WHEN** the linked worktree `feat` lives at `/src/repo.worktrees/feat` and the user runs `wt remove feat` from the main worktree
- **THEN** stdout's first line is `removed worktree /src/repo.worktrees/feat`
- **AND** the directory `/src/repo.worktrees/feat` no longer exists
- **AND** `wt list` does not show `feat`
- **AND** the exit code is 0

#### Scenario: Directory already gone
- **WHEN** the directory of the linked worktree `feat` was deleted without `git worktree remove`, and the user runs `wt remove feat`
- **THEN** `wt list` does not show `feat`
- **AND** the exit code is 0

#### Scenario: Directory cannot be deleted (macOS, Linux)
- **WHEN** on macOS or Linux the worktree `feat` has a committed subdirectory without write permission, and the user runs `wt remove feat`
- **THEN** stderr contains git's error message
- **AND** stderr has a `hint:` line naming the directory of `feat`
- **AND** the branch `feat` still exists
- **AND** the exit code is 1

#### Scenario: Directory in use by another process (macOS, Linux)
- **WHEN** on macOS or Linux another process has its working directory inside the worktree `feat` and the user runs `wt remove feat`
- **THEN** the directory of `feat` no longer exists
- **AND** the exit code is 0

#### Scenario: Directory in use by another process (Windows)
- **WHEN** on Windows another process has its working directory inside the worktree `feat` and the user runs `wt remove feat`
- **THEN** stderr contains git's error message
- **AND** stderr has a `hint:` line naming the directory of `feat`
- **AND** the exit code is 1

#### Scenario: Hooks around the removal
- **WHEN** `hooks.pre-remove` is `["git status --short > {repo_path}/pre.txt"]`, `hooks.post-remove` is `["git worktree list > post.txt"]`, the worktree `feat` is clean, and the user runs `wt remove feat`
- **THEN** the main worktree has the file `pre.txt`
- **AND** once the `post-remove` hook finishes, the main worktree has the file `post.txt`, which does not name the path of `feat`
- **AND** the exit code is 0

#### Scenario: No post-remove when the removal fails (macOS, Linux)
- **WHEN** on macOS or Linux `hooks.post-remove` holds a command that creates a file, the worktree `feat` has a committed subdirectory without write permission, and the user runs `wt remove feat`
- **THEN** the exit code is 1
- **AND** no log of `post-remove` exists for `feat`

### Requirement: Dry run of remove
`wt remove --dry-run` SHALL perform every check of `wt remove`, with the same errors and exit codes, SHALL decide what happens to the branch, and SHALL NOT delete any directory, worktree record or branch, write the directive, or run a hook. Without `--json`, on success it SHALL print to stdout, one per line and in this order: the lines of the `pre-remove` hook; `would remove worktree <path>`; when the worktree has a branch, `would delete branch <branch>`, `would keep branch <branch>`, `would keep branch <branch>: not merged into <base>`, or `would keep branch <branch>: not merged` when there is no base to name; `would change directory to <path of the main worktree>` when it would move the shell and the shell integration is active; and the lines of the `post-remove` hook. The lines of a hook SHALL be those the `hooks` capability specifies for a dry run, and there SHALL be none with `--no-hooks`. With `--json` it SHALL print the same `wt.remove.v1` document as without `--dry-run`.

#### Scenario: Preview
- **WHEN** no hooks are configured, the shell is inside the worktree `feat`, whose branch is merged, and the user runs `wt remove @ --dry-run` through the function
- **THEN** stdout is `would remove worktree <path of feat>`, `would delete branch feat` and `would change directory to <path of the main worktree>`, in that order
- **AND** the worktree `feat` and the branch `feat` still exist
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Preview with hooks
- **WHEN** `hooks.pre-remove` is `["echo bye {name}"]`, `hooks.post-remove` is `["docker rm -f {repo}-{name}-db"]`, the branch of the worktree `feat` of the repository `repo` is merged, and from the main worktree the user runs `wt remove feat --dry-run`
- **THEN** stdout is `would run pre-remove hook: echo bye feat`, `would remove worktree <path of feat>`, `would delete branch feat` and `would run post-remove hook in the background: docker rm -f repo-feat-db`, in that order

#### Scenario: Errors under dry run
- **WHEN** the worktree `feat` has an untracked file and the user runs `wt remove feat --dry-run`
- **THEN** the exit code is 5

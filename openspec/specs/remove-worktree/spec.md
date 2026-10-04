# remove-worktree Specification

## Purpose

Defines how a worktree of the current repository is removed without losing work: `wt remove`, which refuses while the worktree holds uncommitted work, takes the branch along only when it is merged, and moves the shell out of the directory it deletes; and the maintenance commands around it: `wt lock` and `wt unlock`, which protect a worktree from removal, and `wt prune`, which forgets worktrees whose directory is gone.

## Requirements

### Requirement: Arguments and flags of remove
`wt remove` SHALL take exactly one argument, `<target>`, and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `--keep-branch` | keep the worktree's branch, even if it is merged |
| `-D`, `--force-delete-branch` | delete the worktree's branch, even if it is not merged |
| `-f`, `--force` | remove the worktree even if it has modified or untracked files |

A missing `<target>`, an extra argument, and `--keep-branch` together with `-D` SHALL be usage errors, with exit code 2, detected before anything is removed.

#### Scenario: No target
- **WHEN** the user runs `wt remove`
- **THEN** stderr reports the missing `<target>` argument
- **AND** the exit code is 2

#### Scenario: Contradictory branch flags
- **WHEN** the user runs `wt remove feat --keep-branch -D`
- **THEN** the exit code is 2
- **AND** the worktree `feat` still exists

### Requirement: Target
`wt remove` SHALL resolve `<target>` among the worktrees of the repository associated with the working directory as the `navigate` capability specifies for `wt cd` in "Resolving a name" and "Main and current worktree": `^` is the main worktree, `@` the current worktree, and any other target the worktree with that `NAME` or, when none has it, the worktree with that branch, with the same errors and exit codes. When the target resolves to the main worktree, including a bare repository, the command SHALL fail with exit code 2 and the message `cannot remove the main worktree`, and nothing SHALL be removed.

#### Scenario: By branch
- **WHEN** the linked worktree at `<parent>/repo.worktrees/feature-abc1` has the branch `feature/abc1` checked out and the user runs `wt remove feature/abc1`
- **THEN** that worktree is removed

#### Scenario: Current worktree
- **WHEN** the working directory is `<feat>/src`, inside the linked worktree `feat`, and the user runs `wt remove @`
- **THEN** the worktree `feat` is removed

#### Scenario: Main worktree
- **WHEN** the user runs `wt remove ^`
- **THEN** stderr's first line is `wt: cannot remove the main worktree`
- **AND** the exit code is 2

#### Scenario: No such worktree
- **WHEN** the user runs `wt remove nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree named "nope"`
- **AND** the exit code is 3

#### Scenario: Ambiguous name
- **WHEN** two worktrees of the repository live at `/a/x` and `/b/x` and the user runs `wt remove x`
- **THEN** the exit code is 4
- **AND** neither worktree is removed

#### Scenario: Outside a repository
- **WHEN** the user runs `wt remove feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

### Requirement: Locked worktree
A locked worktree SHALL NOT be removed, with or without `--force`. The command SHALL fail with exit code 5, a message stating that the worktree is locked and naming the lock reason when it has one, and a hint naming `wt unlock <NAME>`.

#### Scenario: Locked with a reason
- **WHEN** the worktree `feat` is locked with the reason `on usb drive` and the user runs `wt remove feat`
- **THEN** stderr's first line contains `locked` and `on usb drive`
- **AND** stderr has a `hint:` line containing `wt unlock feat`
- **AND** the worktree `feat` still exists
- **AND** the exit code is 5

#### Scenario: Locked and forced
- **WHEN** the worktree `feat` is locked and the user runs `wt remove feat --force`
- **THEN** the worktree `feat` still exists
- **AND** the exit code is 5

### Requirement: Worktree inside the worktree
When another worktree of the repository lies inside the directory of the worktree to remove, the command SHALL fail with exit code 5 and a message naming both worktrees, with or without `--force`, and nothing SHALL be removed. Paths SHALL be compared by whole components: case-insensitively on macOS and Windows, and case-sensitively on Linux.

#### Scenario: Nested worktree
- **WHEN** the linked worktree `inner` lives at `<outer>/inner`, inside the linked worktree `outer`, has an untracked file, and the user runs `wt remove outer --force`
- **THEN** stderr names `outer` and `inner`
- **AND** the untracked file in `inner` still exists
- **AND** the exit code is 5

### Requirement: Modified and untracked files
Without `--force`, when the worktree's directory exists and has changes to tracked files, staged or not, or untracked files, `wt remove` SHALL fail with exit code 5, a message stating that the worktree has modified or untracked files, and a hint naming `--force`, and nothing SHALL be removed. Untracked files SHALL count whatever git's configuration says about showing them. Files that git ignores SHALL NOT count, and SHALL be deleted with the worktree. With `--force`, the worktree SHALL be removed with all of its files. `--force` SHALL NOT change what happens to the branch (see "Branch").

#### Scenario: Untracked file
- **WHEN** the worktree `feat` has an untracked file and the user runs `wt remove feat`
- **THEN** stderr has a `hint:` line naming `--force`
- **AND** the worktree `feat` and its branch still exist
- **AND** the exit code is 5

#### Scenario: Modified file
- **WHEN** a tracked file in the worktree `feat` was modified and the user runs `wt remove feat`
- **THEN** the exit code is 5

#### Scenario: Untracked files hidden by the configuration
- **WHEN** git's `status.showUntrackedFiles` is `no`, the worktree `feat` has an untracked file, and the user runs `wt remove feat`
- **THEN** the exit code is 5

#### Scenario: Forced
- **WHEN** the worktree `feat` has an untracked file, its branch has a commit that is not merged, and the user runs `wt remove feat --force`
- **THEN** the directory of `feat` no longer exists
- **AND** the branch `feat` still exists
- **AND** the exit code is 0

#### Scenario: Ignored files
- **WHEN** the worktree `feat` has a `.gitignore` that matches `node_modules/`, has a `node_modules` directory and no other changes, and the user runs `wt remove feat`
- **THEN** the directory of `feat` no longer exists
- **AND** the exit code is 0

### Requirement: Branch
When the worktree has a branch checked out, `wt remove` SHALL, once the worktree is removed:

- with `--keep-branch`, keep the branch;
- with `-D`, delete the branch;
- otherwise, delete the branch when it is merged (see the `git-worktrees` capability, "Merged branch"), and keep it when it is not.

A worktree with a detached HEAD has no branch: no branch SHALL be deleted, whatever the flags.

#### Scenario: Fresh worktree
- **WHEN** the user runs `wt create feat` and then `wt remove feat` without committing anything
- **THEN** the branch `feat` no longer exists
- **AND** stdout contains `deleted branch feat`

#### Scenario: Unmerged branch is kept
- **WHEN** the branch `feat` has a commit that is not merged and the user runs `wt remove feat`
- **THEN** the worktree `feat` is removed
- **AND** the branch `feat` still exists
- **AND** stdout contains `kept branch feat: not merged into <base>`
- **AND** the exit code is 0

#### Scenario: Keeping a merged branch
- **WHEN** the branch `feat` is merged and the user runs `wt remove feat --keep-branch`
- **THEN** the branch `feat` still exists
- **AND** stdout contains `kept branch feat`

#### Scenario: Deleting an unmerged branch
- **WHEN** the branch `feat` has a commit that is not merged and the user runs `wt remove feat -D`
- **THEN** the branch `feat` no longer exists
- **AND** stdout contains `deleted branch feat`

#### Scenario: Detached worktree
- **WHEN** the linked worktree `scratch` has a detached HEAD and the user runs `wt remove scratch -D`
- **THEN** the worktree is removed
- **AND** no branch is deleted
- **AND** the exit code is 0

### Requirement: Removing the worktree
Once every check passes, `wt remove` SHALL delete the worktree's directory and git's record of it, so that `wt list` no longer shows it; when the directory no longer exists, it SHALL only remove git's record. Without `--json`, it SHALL print to stdout `removed worktree <path>`, followed, when the worktree had a branch, by one of `deleted branch <branch>`, `kept branch <branch>` (with `--keep-branch`), or `kept branch <branch>: not merged into <base>` (`kept branch <branch>: not merged` when there is no base to name, which includes a base that does not resolve to a commit), and SHALL exit 0.

When git fails while removing the worktree, the command SHALL fail with exit code 1 and include git's message, with a hint naming the directory when it still exists; it SHALL NOT delete the branch or move the shell. When git fails while deleting the branch, the command SHALL fail with exit code 1 and include git's message; the worktree stays removed.

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

### Requirement: Removing from inside
Removing a worktree SHALL succeed, on every operating system, when the directory `wt` was started in is inside that worktree: while removing it, `wt` SHALL NOT keep the worktree's directory as its own working directory or as that of the processes it starts.

#### Scenario: Binary started inside the worktree
- **WHEN** `git-wt` is started with its working directory in `<feat>/src`, inside the clean worktree `feat`, with the arguments `remove @`
- **THEN** the directory of `feat` no longer exists
- **AND** the exit code is 0

### Requirement: Moving the shell
When the directory `wt` was started in (not the one given with `-C`) is inside the worktree being removed, compared as the `git-worktrees` capability compares paths for the current worktree, `wt remove` SHALL move the shell to the main worktree once the worktree is removed. When the shell integration is active (see the `shell-integration` capability), it SHALL write the main worktree's path as the directive. When it is not active, it SHALL write a warning stating that the shell integration is not active and that the shell stays in a directory that no longer exists, and SHALL exit as it would otherwise. When that directory is not inside the worktree being removed, it SHALL write no directive. When the directive cannot be written, the command SHALL fail with exit code 1, and the worktree SHALL stay removed.

#### Scenario: From inside, through the function
- **WHEN** the shell is in `<feat>/src`, inside the worktree `feat`, and the user runs `wt remove @` through the function
- **THEN** the shell's working directory is the main worktree's path
- **AND** the exit code is 0

#### Scenario: From elsewhere
- **WHEN** the shell is in the main worktree and the user runs `wt remove feat` through the function
- **THEN** the shell's working directory does not change

#### Scenario: Working directory override
- **WHEN** the shell is inside the worktree `feat` and the user runs `wt -C <main worktree> remove feat` through the function
- **THEN** the shell's working directory is the main worktree's path

#### Scenario: Without the function
- **WHEN** the user runs `git-wt remove @` from inside the worktree `feat` in a shell that did not load the script
- **THEN** the worktree `feat` is removed
- **AND** stderr contains a warning stating that the shell integration is not active
- **AND** the exit code is 0

### Requirement: JSON output of remove
`wt remove <target> --json` SHALL remove the worktree and move the shell like `wt remove <target>`, and SHALL print `{"schema":"wt.remove.v1","name":...,"path":...,"branch":...,"branch_deleted":...}` instead of the text lines, where `name` is the worktree's `NAME`, `path` its absolute path in the operating system's native form, `branch` its branch or null when it had none, and `branch_deleted` whether the branch was deleted.

#### Scenario: Removed as JSON
- **WHEN** the branch `feat` is merged and the user runs `wt remove feat --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.remove.v1`, `name` `feat`, `branch` `feat` and `branch_deleted` `true`
- **AND** `path` is the path the worktree had

### Requirement: Dry run of remove
`wt remove --dry-run` SHALL perform every check of `wt remove`, with the same errors and exit codes, SHALL decide what happens to the branch, and SHALL NOT delete any directory, worktree record or branch, or write the directive. Without `--json`, on success it SHALL print to stdout, one per line and in this order: `would remove worktree <path>`; when the worktree has a branch, `would delete branch <branch>`, `would keep branch <branch>`, `would keep branch <branch>: not merged into <base>`, or `would keep branch <branch>: not merged` when there is no base to name; and `would change directory to <path of the main worktree>` when it would move the shell and the shell integration is active. With `--json` it SHALL print the same `wt.remove.v1` document as without `--dry-run`.

#### Scenario: Preview
- **WHEN** the shell is inside the worktree `feat`, whose branch is merged, and the user runs `wt remove @ --dry-run` through the function
- **THEN** stdout is `would remove worktree <path of feat>`, `would delete branch feat` and `would change directory to <path of the main worktree>`, in that order
- **AND** the worktree `feat` and the branch `feat` still exist
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Errors under dry run
- **WHEN** the worktree `feat` has an untracked file and the user runs `wt remove feat --dry-run`
- **THEN** the exit code is 5

### Requirement: Locking
`wt lock` SHALL take a `<target>` and an optional `<reason>`; a missing `<target>` or a third argument SHALL be a usage error (exit 2). The target SHALL be resolved as for `wt remove`. When it is the main worktree, the command SHALL fail with exit code 2 and the message `the main worktree cannot be locked`. When the worktree is already locked, the command SHALL fail with exit code 5 and a message stating that it is already locked, naming its reason when it has one. Otherwise `wt lock` SHALL lock the worktree, with `<reason>` as the lock reason when it is given and not empty, SHALL print `locked worktree <path>` to stdout, and SHALL exit 0. A worktree whose directory no longer exists SHALL be lockable, and once locked it SHALL no longer be reported as prunable.

#### Scenario: Lock with a reason
- **WHEN** the user runs `wt lock feat "on usb drive"`
- **THEN** stdout is `locked worktree <path of feat>`
- **AND** in the output of `wt list --json`, the element of `feat` has `"locked": true` and `"locked_reason": "on usb drive"`
- **AND** the exit code is 0

#### Scenario: Lock without a reason
- **WHEN** the user runs `wt lock feat`
- **THEN** in the output of `wt list --json`, the element of `feat` has `"locked": true` and `"locked_reason": null`

#### Scenario: Already locked
- **WHEN** the worktree `feat` is locked with the reason `review` and the user runs `wt lock feat`
- **THEN** stderr's first line contains `already locked` and `review`
- **AND** the exit code is 5

#### Scenario: Lock the main worktree
- **WHEN** the user runs `wt lock ^`
- **THEN** stderr's first line is `wt: the main worktree cannot be locked`
- **AND** the exit code is 2

#### Scenario: Directory gone
- **WHEN** the directory of the linked worktree `feat` was deleted without `git worktree remove`, and the user runs `wt lock feat`
- **THEN** the exit code is 0
- **AND** `wt list` shows `locked` and not `prunable` in the `STATE` column of `feat`

### Requirement: Unlocking
`wt unlock` SHALL take exactly one argument, `<target>`, resolved as for `wt remove`. When it is the main worktree, the command SHALL fail with exit code 2 and the message `the main worktree cannot be unlocked`. When the worktree is not locked, the command SHALL fail with exit code 5 and the message `worktree "<NAME>" is not locked`. Otherwise `wt unlock` SHALL unlock it, SHALL print `unlocked worktree <path>` to stdout, and SHALL exit 0.

#### Scenario: Unlock
- **WHEN** the worktree `feat` is locked and the user runs `wt unlock feat`
- **THEN** stdout is `unlocked worktree <path of feat>`
- **AND** `wt remove feat` no longer fails because of a lock
- **AND** the exit code is 0

#### Scenario: Not locked
- **WHEN** the worktree `feat` is not locked and the user runs `wt unlock feat`
- **THEN** stderr's first line is `wt: worktree "feat" is not locked`
- **AND** the exit code is 5

### Requirement: JSON output and dry run of lock and unlock
With `--json`, `wt lock` SHALL print `{"schema":"wt.lock.v1","name":...,"path":...,"reason":...}`, where `reason` is the lock reason or null, and `wt unlock` SHALL print `{"schema":"wt.unlock.v1","name":...,"path":...}`, where `name` is the worktree's `NAME` and `path` its absolute path in the operating system's native form. With `--dry-run`, both commands SHALL perform every check, with the same errors and exit codes, SHALL NOT change any lock, and on success SHALL print `would lock worktree <path>` or `would unlock worktree <path>`; with `--json` as well, they SHALL print the same document as without `--dry-run`.

#### Scenario: Lock as JSON
- **WHEN** the user runs `wt lock feat review --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.lock.v1`, `name` `feat` and `reason` `review`

#### Scenario: Preview a lock
- **WHEN** the user runs `wt lock feat --dry-run`
- **THEN** stdout is `would lock worktree <path of feat>`
- **AND** the worktree `feat` is not locked afterwards

### Requirement: Pruning
`wt prune` SHALL take no arguments. It SHALL remove git's record of every worktree of the repository associated with the working directory that the `git-worktrees` capability reports as prunable; locked worktrees are never prunable. It SHALL NOT delete any branch. It SHALL print to stdout one line `pruned worktree <path>` per worktree it pruned, in the order `wt list` uses, or `nothing to prune` when there was none, and SHALL exit 0. Outside a repository it SHALL fail as that capability specifies (exit 3).

#### Scenario: Prune a deleted worktree
- **WHEN** the directory of the linked worktree `feat` was deleted without `git worktree remove`, and the user runs `wt prune`
- **THEN** stdout is `pruned worktree <path of feat>`
- **AND** `wt list` does not show `feat`
- **AND** the branch `feat` still exists
- **AND** the exit code is 0

#### Scenario: Locked worktree is kept
- **WHEN** the worktree `feat` is locked, its directory was deleted, and the user runs `wt prune`
- **THEN** stdout is `nothing to prune`
- **AND** `wt list` still shows `feat`

#### Scenario: Nothing to prune
- **WHEN** every worktree's directory exists and the user runs `wt prune`
- **THEN** stdout is `nothing to prune`
- **AND** the exit code is 0

### Requirement: JSON output and dry run of prune
With `--json`, `wt prune` SHALL print `{"schema":"wt.prune.v1","pruned":[...]}`, where each element has `name`, `path` (absolute, in the operating system's native form) and `reason` (git's reason for considering it prunable), in the order of the text output, and `pruned` is an empty array when nothing was pruned. With `--dry-run`, it SHALL NOT remove any record, and SHALL print `would prune worktree <path>` for each worktree it would prune, or `nothing to prune`; with `--json` as well, it SHALL print the same document as without `--dry-run`.

#### Scenario: Pruned as JSON
- **WHEN** the directory of the linked worktree `feat` was deleted and the user runs `wt prune --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.prune.v1` and a `pruned` array with one element whose `name` is `feat`

#### Scenario: Preview a prune
- **WHEN** the directory of the linked worktree `feat` was deleted and the user runs `wt prune --dry-run`
- **THEN** stdout is `would prune worktree <path of feat>`
- **AND** `wt list` still shows `feat`

### Requirement: Completions
Completions SHALL offer, for the `<target>` of `wt remove` and of `wt lock`, the `NAME` of every worktree of the repository associated with the shell's working directory that is neither the main worktree nor locked, including those whose directory no longer exists; for the `<target>` of `wt unlock`, the `NAME` of every locked worktree; and nothing for the `<reason>` of `wt lock`. They SHALL never offer file names. Requesting a completion SHALL never write to the terminal: outside a repository, or when git fails, nothing is offered.

#### Scenario: Targets for remove
- **WHEN** a repository has the linked worktrees `feat`, `gone` (its directory deleted) and `usb` (locked), and the user requests completions for `wt remove `
- **THEN** `feat` and `gone` are offered
- **AND** `usb` and the main worktree's `NAME` are not offered

#### Scenario: Targets for unlock
- **WHEN** in the same repository the user requests completions for `wt unlock `
- **THEN** only `usb` is offered

#### Scenario: Outside a repository
- **WHEN** outside any repository the user requests completions for `wt remove `
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

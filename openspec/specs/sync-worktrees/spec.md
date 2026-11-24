# sync-worktrees Specification

## Purpose

Defines `wt sync`, the command that brings one or more worktrees up to date with their base by rebasing their branches: which worktrees it syncs, what the base is and when it is fetched, which worktrees it skips so that no work is lost, what it does on a conflict or an interrupt, and how it reports the result.

## Requirements

### Requirement: Arguments and flags of sync
`wt sync` SHALL take zero or more `<name>` arguments and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `--all` | sync every linked worktree of the current repository |
| `--base <ref>` | rebase onto `<ref>` instead of each repository's base |
| `--no-fetch` | fetch nothing |

`--all` together with a `<name>`, and an empty `--base`, SHALL be usage errors (exit 2), reported before any repository is read.

#### Scenario: All and a name
- **WHEN** the user runs `wt sync feat --all`
- **THEN** stderr names `--all`
- **AND** the exit code is 2

#### Scenario: Hooks flag
- **WHEN** the user runs `wt sync --no-hooks`
- **THEN** stderr names `--no-hooks` as an unknown flag
- **AND** the exit code is 2

### Requirement: Worktrees synced
With no `<name>` and no `--all`, `wt sync` SHALL sync the current worktree. Each `<name>` SHALL be resolved as `wt remove` resolves its target (see the `remove-worktree` capability), with its errors and exit codes, except that it MAY designate a main worktree. A worktree named twice SHALL be synced once. Worktrees SHALL be synced one at a time, in the order named.

#### Scenario: Current worktree
- **WHEN** the working directory is inside the linked worktree `feat` and the user runs `wt sync`
- **THEN** only `feat` is synced

#### Scenario: Main worktree by name
- **WHEN** the user runs `wt sync ^` in a repository whose main worktree has the branch `main`
- **THEN** the main worktree is synced

#### Scenario: Name not found
- **WHEN** no worktree is named `nope` and the user runs `wt sync feat nope`
- **THEN** the exit code is 3
- **AND** `feat` was not rebased

### Requirement: All worktrees of the repository
With `--all`, `wt sync` SHALL sync, in the order `wt list` lists them, every worktree of the repository associated with the working directory except its main worktree, a bare repository, and a worktree whose HEAD is detached; those SHALL be left out without a message. When no worktree is left, it SHALL write nothing to stdout and SHALL exit 0.

#### Scenario: Linked worktrees only
- **WHEN** a repository has its main worktree, the linked worktrees `feat` and `fix`, and the linked worktree `scratch` with a detached HEAD, and the user runs `wt sync --all`
- **THEN** `feat` and `fix` are synced, in that order
- **AND** neither the main worktree nor `scratch` is mentioned in stdout

#### Scenario: No linked worktree
- **WHEN** a repository has no linked worktree and the user runs `wt sync --all`
- **THEN** stdout is empty
- **AND** the exit code is 0

### Requirement: Outside a repository
With no `<name>`, or with `--all`, outside a repository `wt sync` SHALL fail as `wt list` does (exit 3, `not a git repository: <dir>`), and nothing SHALL be fetched or rebased. With names, it SHALL resolve them as "Worktrees synced" specifies, including in other repositories.

#### Scenario: All outside a repository
- **WHEN** the user runs `wt sync --all` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

### Requirement: Label of a worktree
Each worktree SHALL be named in the output by its label: `^` for a main worktree and its `NAME` for any other, preceded by the display name of its repository and `:` when that is not the repository associated with the working directory (see the `cross-repo-resolution` capability).

#### Scenario: Worktree of another repository
- **WHEN** from inside `<root>/web` the user runs `wt sync api:feat` and `feat` of `api` is behind its base
- **THEN** stdout is `rebased api:feat onto <base>`

### Requirement: Base
The base of a worktree SHALL be the value of `--base` when given; otherwise the base of its repository, as the `git-worktrees` capability defines it in "Merged branch": `default_base` from that repository's configuration layers when not empty, else its default branch. In the output, the base SHALL be written as given or configured.

#### Scenario: Base from the repository configuration
- **WHEN** `.wt.toml` sets `default_base = "develop"` and the user runs `wt sync feat`
- **THEN** `feat` is rebased onto `develop`

#### Scenario: Explicit base
- **WHEN** the user runs `wt sync feat --base origin/release`
- **THEN** `feat` is rebased onto `origin/release`

### Requirement: Fetching the base
Unless `--no-fetch` is given, when a base starts with the name of a configured remote of its repository followed by `/` (the longest such name), `wt sync` SHALL fetch that remote once per repository, before any rebase. When a fetch fails, it SHALL write a warning naming the remote and SHALL continue with the references already present.

#### Scenario: One fetch for several worktrees
- **WHEN** the base is `origin/main` and the user runs `wt sync --all` in a repository with three linked worktrees
- **THEN** `origin` is fetched once
- **AND** the three worktrees are rebased onto the fetched `origin/main`

#### Scenario: Remote cannot be reached
- **WHEN** `origin` cannot be reached, `origin/main` exists locally, and the user runs `wt sync feat`
- **THEN** stderr contains a warning naming `origin`
- **AND** `feat` is rebased onto the local `origin/main`

#### Scenario: Local base
- **WHEN** the user runs `wt sync feat --base main`
- **THEN** nothing is fetched

### Requirement: Base that cannot be used
After fetching and before any rebase, a base that does not resolve to a commit SHALL fail the command with exit code 3 and a message naming the base. A repository with no `--base`, an empty `default_base` and no default branch SHALL fail with exit code 3, a message stating that there is no default branch, and hints naming `--base` and `default_base`. In both cases no worktree SHALL be rebased.

#### Scenario: Base not found
- **WHEN** the user runs `wt sync feat fix --base nope` and no revision `nope` exists
- **THEN** stderr names `nope`
- **AND** neither `feat` nor `fix` was rebased
- **AND** the exit code is 3

### Requirement: Worktrees skipped
`wt sync` SHALL NOT stash, and SHALL skip, without changing it, a worktree that:

- has changes to tracked files, staged or not (untracked and ignored files SHALL NOT count);
- has a detached HEAD;
- has a rebase, merge, cherry-pick or revert in progress;
- has a directory that does not exist.

It SHALL print `skipped <label>: <reason>`, with the reason `has uncommitted changes`, `HEAD is detached`, `a <operation> is in progress` or `<path> does not exist`.

#### Scenario: Modified file
- **WHEN** a tracked file in `feat` was modified, `fix` is behind its base, and the user runs `wt sync --all`
- **THEN** stdout contains `skipped feat: has uncommitted changes`
- **AND** `feat`'s HEAD and its modified file are unchanged
- **AND** `fix` is rebased

#### Scenario: Untracked file
- **WHEN** `feat` is behind its base and has only an untracked file `notes.txt` that its base does not have, and the user runs `wt sync feat`
- **THEN** `feat` is rebased
- **AND** `notes.txt` is unchanged

#### Scenario: Rebase in progress
- **WHEN** `feat` has a rebase stopped on a conflict and the user runs `wt sync feat`
- **THEN** stdout is `skipped feat: a rebase is in progress`
- **AND** the rebase in `feat` is still in progress

### Requirement: Worktree up to date
When the commit the base resolves to is the worktree's HEAD or an ancestor of it, `wt sync` SHALL NOT rebase it and SHALL print `<label> is up to date with <base>`.

#### Scenario: Fresh worktree
- **WHEN** the user runs `wt create feat` and then `wt sync feat` with no new commit on the base
- **THEN** stdout is `feat is up to date with <base>`
- **AND** the exit code is 0

### Requirement: Rebasing a worktree
Otherwise, `wt sync` SHALL rebase the worktree's branch onto the base's commit, as `git rebase <base>` does in that worktree, without stashing whatever git's configuration says, and SHALL print `rebased <label> onto <base>`. The branch's upstream SHALL be left as it was, and nothing SHALL be pushed.

#### Scenario: Behind the base
- **WHEN** `feat` has one commit of its own, `origin/main` has a commit `feat` does not contain, and the user runs `wt sync feat`
- **THEN** stdout is `rebased feat onto origin/main`
- **AND** `origin/main` is an ancestor of `feat`
- **AND** `feat` still has its own commit's changes
- **AND** the exit code is 0

#### Scenario: Autostash configured
- **WHEN** git's configuration sets `rebase.autoStash = true`, a tracked file in `feat` was modified, and the user runs `wt sync feat`
- **THEN** stdout is `skipped feat: has uncommitted changes`
- **AND** no stash entry was created

### Requirement: Conflict
When the rebase of a worktree stops on a conflict, `wt sync` SHALL abort it, leaving the worktree's branch, HEAD and files as they were before, SHALL print `conflict in <label>: rebase onto <base> aborted`, and SHALL go on with the next worktree.

#### Scenario: Conflict then the next worktree
- **WHEN** `feat` and the base change the same line of a file, `fix` is behind its base, and the user runs `wt sync --all`
- **THEN** stdout contains `conflict in feat: rebase onto <base> aborted`
- **AND** `feat` points to the commit it pointed to before and has no rebase in progress
- **AND** `fix` is rebased
- **AND** the exit code is 6

### Requirement: Failed rebase
When git cannot start the rebase, or stops it for a reason other than a conflict, `wt sync` SHALL abort any rebase left in progress, SHALL print `failed <label>: <message>`, where `<message>` is git's first error line, and SHALL go on with the next worktree.

#### Scenario: Untracked file in the way
- **WHEN** `feat` has an untracked file `new.txt` and the base adds a tracked `new.txt`, and the user runs `wt sync feat`
- **THEN** stdout starts with `failed feat: `
- **AND** `feat` points to the commit it pointed to before and has no rebase in progress
- **AND** `new.txt` in `feat` is unchanged
- **AND** the exit code is 1

### Requirement: Summary
When a worktree was skipped, failed or had a conflict, after the last worktree `wt sync` SHALL write to stderr `wt: <k> of <m> worktrees not synced`, followed by one `hint:` line per such worktree, in order: `<label>: <reason>` when skipped, `<label>: <message>` when failed, and `<label>: conflicts with <base>; run "git rebase <base>" in <path> to resolve` on a conflict.

#### Scenario: Skipped and conflict
- **WHEN** `feat` has a modified tracked file, `fix` conflicts with its base, `docs` is behind its base, and the user runs `wt sync --all`
- **THEN** stderr contains `wt: 2 of 3 worktrees not synced`, `hint: feat: has uncommitted changes` and a `hint:` line naming `fix`, `git rebase` and the path of `fix`
- **AND** `docs` is rebased

### Requirement: Exit code of sync
`wt sync` SHALL exit 0 when every worktree was rebased or was up to date. Otherwise the exit code SHALL be 1 when a rebase failed, else 6 when one had a conflict, else 5.

#### Scenario: Conflict wins over skipped
- **WHEN** `feat` has a modified tracked file, `fix` conflicts with its base, and the user runs `wt sync --all`
- **THEN** the exit code is 6

#### Scenario: Only skipped
- **WHEN** `feat` has a modified tracked file and the user runs `wt sync feat`
- **THEN** the exit code is 5

### Requirement: Interrupt
An interrupt from the terminal (Ctrl-C) SHALL reach the git command that is running and SHALL NOT end `wt` before that command exits. `wt sync` SHALL then abort a rebase that command left in progress, SHALL NOT start another fetch or rebase, SHALL write to stderr `wt: interrupted: <k> of <m> worktrees not synced`, and SHALL exit 130 on every operating system.

#### Scenario: Interrupt during a rebase (macOS, Linux)
- **WHEN** on macOS or Linux `wt sync --all` runs in a repository with the linked worktrees `feat` and `fix`, both behind their base, and `wt` receives SIGINT while `feat` is being rebased
- **THEN** `feat` has no rebase in progress
- **AND** `fix` was not rebased
- **AND** the exit code is 130

#### Scenario: Ctrl-C in the console (Windows)
- **WHEN** on Windows the user presses Ctrl-C while `wt sync --all` rebases the first of two worktrees
- **THEN** the second worktree was not rebased
- **AND** no worktree has a rebase in progress
- **AND** `$LASTEXITCODE` is 130

### Requirement: Dry run of sync
`wt sync --dry-run` SHALL perform every check and read of `wt sync`, with the same errors and exit codes, and SHALL NOT fetch or rebase. The base SHALL be resolved against the references already present. It SHALL print `would fetch <remote>` once per fetch, before anything else, and for each worktree the line it would print, with `would rebase <label> onto <base>` in place of `rebased ...`. Conflicts SHALL NOT be predicted.

#### Scenario: Preview
- **WHEN** the base is `origin/main`, `feat` is behind it, `fix` is up to date, `docs` has a modified tracked file, and the user runs `wt sync --all --dry-run`
- **THEN** stdout is `would fetch origin`, `would rebase feat onto origin/main`, `fix is up to date with origin/main` and `skipped docs: has uncommitted changes`, in that order
- **AND** no branch moved
- **AND** the exit code is 5

### Requirement: JSON output of sync
With `--json` and exit code 0, `wt sync` SHALL print `{"schema":"wt.sync.v1","worktrees":[...]}`, one object per worktree in order, with `name` (its `NAME`), `repo` (its repository's display name), `path` (native form), `branch`, `base` (as written), `result` and `head` (the full commit id of HEAD afterwards). `result` SHALL be `rebased` or `up_to_date`; under `--dry-run`, `would_rebase` replaces `rebased`.

#### Scenario: Synced as JSON
- **WHEN** `feat` is behind its base and the user runs `wt sync feat --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.sync.v1` and one worktree with `name` `feat` and `result` `rebased`

### Requirement: JSON output when a worktree is not synced
With `--json` and an exit code other than 0, `wt sync` SHALL fail as the `cli-contract` capability specifies, with the message and hints of "Summary", having rebased what it rebased.

#### Scenario: Conflict as JSON
- **WHEN** `feat` conflicts with its base and the user runs `wt sync feat --json`
- **THEN** stdout is empty
- **AND** stderr parses as a JSON object with `schema` `wt.error.v1` and `code` 6

### Requirement: Completions
Completions for `<name>` SHALL offer what they offer for the `<target>` of `wt remove` (see the `remove-worktree` capability), leaving out worktrees with a detached HEAD instead of locked and main worktrees, and not offering a name already given. For `--base` they SHALL offer what they offer for the `--base` of `wt create`. They SHALL never offer file names.

#### Scenario: Names
- **WHEN** a repository has the linked worktrees `feat`, `usb` (locked) and `scratch` (detached), and the user requests completions for `wt sync feat `
- **THEN** `usb` is offered
- **AND** `feat` and `scratch` are not offered

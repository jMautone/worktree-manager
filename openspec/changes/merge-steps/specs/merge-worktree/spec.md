## ADDED Requirements

### Requirement: Commit flags of merge
Besides the flags of "Arguments and flags of merge", `wt merge` SHALL accept `--squash` (see "Squashing before merging"), `-m <msg>` (the message of the commit or the squash, see the `merge-steps` capability, "Commit message") and `--no-commit` (pending changes are not committed and block the merge, see "State checked before merging"). `--squash` with `--no-commit` SHALL squash only the branch's commits.

#### Scenario: Squash without committing
- **WHEN** `feat` has pending changes and from inside `feat` the user runs `wt merge --squash --no-commit`
- **THEN** the exit code is 5
- **AND** `feat` points to the commit it pointed to before

### Requirement: Dry run of a merge that commits
`wt merge --dry-run` SHALL NOT commit or open an editor, and SHALL need no message. When it would commit or squash, it SHALL print, before `would rebase` and the rest of "Dry run of merge": the lines of `pre-commit`, then `would commit pending changes to <branch>` or `would squash <n> commits of <branch>`.

#### Scenario: Preview of a commit
- **WHEN** `feat` has a modified tracked file, no hook is configured, `feat` contains `main`, and from inside `feat` the user runs `wt merge --dry-run < /dev/null`
- **THEN** stdout starts with `would commit pending changes to feat` and `would merge feat into main`
- **AND** the file is still modified and `feat` points to the commit it pointed to before

### Requirement: Committing before merging
Unless `--no-commit` or `--squash` is given, when the worktree has pending changes, `wt merge` SHALL, once every check passes and before rebasing, commit them as `wt step commit` does (see the `merge-steps` capability), with `pre-commit`, the message, its line and `post-commit`. When committing fails or is interrupted, `wt merge` SHALL stop as that step does, with nothing rebased or moved.

#### Scenario: Pending change committed and merged
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt merge -m "finish"` through the function
- **THEN** stdout contains `committed <short id> to feat`, `merged feat into main` and `removed worktree <path of feat>`, in that order
- **AND** `main`'s HEAD has the message `finish`
- **AND** the exit code is 0

#### Scenario: Empty message stops the merge
- **WHEN** git's editor is a command that empties the file it is given, `feat` has a modified tracked file, and from inside `feat` the user runs `wt merge` in a terminal
- **THEN** the file is still modified, and `feat` and `main` point to the commits they pointed to before
- **AND** the worktree `feat` still exists
- **AND** the exit code is 1

### Requirement: Squashing before merging
With `--squash`, once every check passes and before rebasing, `wt merge` SHALL squash the branch onto the target as `wt step squash` does (see the `merge-steps` capability), pending changes included, with its hooks, message and line; then it SHALL go on as without it. When there is nothing to squash, it SHALL print `nothing to squash` and go on.

#### Scenario: Merged as one commit
- **WHEN** `feat` has three commits that `main` does not contain and from inside the clean worktree `feat` the user runs `wt merge --squash -m "add login"`
- **THEN** `main` has exactly one new commit, with the message `add login`
- **AND** the worktree `feat` and the branch `feat` no longer exist
- **AND** the exit code is 0

### Requirement: Post-commit of a merge that removes
When `wt merge` commits or squashes and then removes the worktree, `post-commit` SHALL start once the worktree is removed, with `post-remove` and `post-merge`, and SHALL run in the main worktree (see the `hooks` capability, "Events"). With `--no-remove`, it SHALL start right after the commit, in the worktree.

#### Scenario: Order of the events of a merge that commits
- **WHEN** every event of `wt merge`, `wt remove` and `wt step commit` has a command that appends its event's name to `<sandbox>/events.txt`, `feat` has a modified tracked file, and from inside `feat` the user runs `wt merge -m "x"`
- **THEN** once the background commands finish, `events.txt` contains `pre-commit`, `pre-merge`, `pre-remove`, and then `post-commit`, `post-remove` and `post-merge` in any order

## MODIFIED Requirements

### Requirement: State checked before merging
Before anything changes, `wt merge` SHALL fail with exit code 5 when `--no-commit` is given and the worktree has pending changes (see the `merge-steps` capability, "Pending changes"), when it has a rebase, merge, cherry-pick or revert in progress, when it is locked and `--no-remove` is not given, or when the target is checked out in a worktree with changes to tracked files. The message SHALL state which, and nothing SHALL change.

#### Scenario: Uncommitted change
- **WHEN** a tracked file in `feat` was modified and from inside `feat` the user runs `wt merge --no-commit`
- **THEN** the target and the branch `feat` point to the commits they pointed to before
- **AND** the exit code is 5

#### Scenario: Target checked out with changes
- **WHEN** the target `main` is checked out in the main worktree, which has a modified tracked file, and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** the exit code is 5
- **AND** the branch `feat` was not rebased

### Requirement: Order of the checks
`wt merge` SHALL make every check of "Worktree merged", "Target" "State checked before merging" and, when it would commit, of the `merge-steps` capability's "Message without a terminal", and SHALL check the approvals of the repository's hooks (see the `project-hooks-approval` capability), before it commits, rebases, runs a hook or moves anything.

#### Scenario: Hook not approved
- **WHEN** `.wt.toml` sets `pre-merge = ["npm test"]`, not approved, and from inside the clean worktree `feat`, behind its target, the user runs `wt merge`
- **THEN** the branch `feat` was not rebased
- **AND** the exit code is 8

#### Scenario: Message needed in a script
- **WHEN** `feat` has a modified tracked file and from inside `feat` the user runs `wt merge < /dev/null`
- **THEN** stderr's first line is `wt: a commit message is needed`
- **AND** `feat` and its target point to the commits they pointed to before
- **AND** the exit code is 2

### Requirement: JSON output of merge
With `--json`, `wt merge` SHALL print `{"schema":"wt.merge.v1","name":...,"path":...,"branch":...,"target":...,"committed":...,"squashed":...,"rebased":...,"head":...,"removed":...}`, where `committed` says whether pending changes were committed, `squashed` whether the branch was squashed, `rebased` says whether the branch was rebased, `head` is the target's full commit id afterwards, and `removed` whether the worktree was removed; under `--dry-run`, what would be done.

#### Scenario: Merged as JSON
- **WHEN** from inside the clean worktree `feat` the user runs `wt merge --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.merge.v1`, `branch` `feat`, `target` `main` and `removed` `true`

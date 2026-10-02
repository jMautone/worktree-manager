## ADDED Requirements

### Requirement: Default branch
Commands that need the repository's default branch, such as `wt create` when neither `--base` nor `default_base` is given, SHALL use:

1. the remote-tracking branch that `origin/HEAD` points to, written `origin/<branch>`, when `origin/HEAD` exists;
2. otherwise, the local branch that the main worktree's HEAD points to (for a bare repository, the bare repository's HEAD);
3. otherwise, when the main worktree's HEAD is detached, the repository has no default branch.

#### Scenario: Clone
- **WHEN** the repository was cloned, `origin/HEAD` points to `origin/main`, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from origin/main`

#### Scenario: No remote
- **WHEN** the repository has no remote, its main worktree has the branch `trunk` checked out, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from trunk`

#### Scenario: Bare repository
- **WHEN** the working directory is a bare repository without `origin/HEAD` whose HEAD points to `main`, and the user runs `wt create feat --dry-run` with `default_base` empty
- **THEN** stdout contains `on new branch feat from main`

#### Scenario: Detached main worktree
- **WHEN** the repository has no `origin/HEAD`, its main worktree has a detached HEAD, and the user runs `wt create feat` with `default_base` empty
- **THEN** stderr states that there is no default branch
- **AND** the exit code is 3

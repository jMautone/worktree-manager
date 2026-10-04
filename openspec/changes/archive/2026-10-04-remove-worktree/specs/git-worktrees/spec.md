## ADDED Requirements

### Requirement: Merged branch
Commands that need to know whether a branch is merged, such as `wt remove` when it decides whether to delete a worktree's branch, SHALL consider a branch merged when its tip commit is the commit of, or an ancestor of, either:

1. the base: the value of `default_base` when it is not empty, otherwise the repository's default branch (see "Default branch"); or
2. the branch's upstream, when it has one.

This SHALL be judged against the references already present in the repository: nothing is fetched. A branch whose changes reached the base through other commits, such as a squash merge, a rebase or a cherry-pick, SHALL NOT count as merged. When `default_base` is empty and the repository has no default branch, or when the base does not resolve to a commit, only the upstream SHALL count, and the command SHALL write a warning stating why the base could not be used.

#### Scenario: No commits of its own
- **WHEN** the branch `feat` points to the same commit as `origin/main`, `origin/HEAD` points to `origin/main`, `default_base` is empty, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Merged into the base
- **WHEN** the tip of the branch `feat` is an ancestor of `origin/main`, `origin/HEAD` points to `origin/main`, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Commit not in the base
- **WHEN** the branch `feat` has a commit that `origin/main` does not contain, `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists
- **AND** stdout contains `kept branch feat: not merged into origin/main`

#### Scenario: Pushed to its upstream
- **WHEN** the branch `feat` has a commit that `origin/main` does not contain, its upstream is `origin/feat`, `origin/feat` contains that commit, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: Ahead of its upstream
- **WHEN** the branch `feat` has a commit that neither `origin/main` nor its upstream `origin/feat` contains, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists

#### Scenario: Squash merge
- **WHEN** the changes of the branch `feat` reached `origin/main` as one new commit made by a squash merge, `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** the branch `feat` still exists

#### Scenario: Base from the configuration
- **WHEN** `default_base` is `develop`, the tip of the branch `feat` is an ancestor of `develop` but not of `origin/main`, and the user runs `wt remove feat`
- **THEN** the branch `feat` no longer exists

#### Scenario: No base
- **WHEN** the repository has no `origin/HEAD`, its main worktree has a detached HEAD, `default_base` is empty, the branch `feat` has no upstream, and the user runs `wt remove feat`
- **THEN** stderr contains a warning stating that the repository has no default branch
- **AND** the branch `feat` still exists
- **AND** the exit code is 0

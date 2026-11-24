## MODIFIED Requirements

### Requirement: Checking approvals
Approvals SHALL be checked for every event the command would fire, and only for those: `pre-create` and `post-create` for `wt create`, `pre-cd` and `post-cd` for `wt cd`, `pre-remove` and `post-remove` for `wt remove`; `pre-merge` and `post-merge` for `wt merge`, and with them `pre-remove` and `post-remove` unless `--no-remove` is given. With `--no-hooks` no approval SHALL be checked. Under `--dry-run` approvals SHALL be checked as without it.

#### Scenario: Other events are not checked
- **WHEN** `.wt.toml` sets `pre-remove = ["make clean"]`, not approved, and no other hook, and the user runs `wt create feat`
- **THEN** the worktree `feat` is created
- **AND** the exit code is 0

#### Scenario: Skipping hooks skips the check
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt create feat --no-hooks`
- **THEN** the worktree `feat` is created
- **AND** the exit code is 0

#### Scenario: Dry run not approved
- **WHEN** `.wt.toml` sets `post-create = ["npm ci"]`, not approved, and the user runs `wt create feat --dry-run`
- **THEN** stdout is empty
- **AND** the exit code is 8

#### Scenario: Merge checks the remove events
- **WHEN** `.wt.toml` sets `pre-remove = ["make clean"]`, not approved, and from inside the clean worktree `feat` the user runs `wt merge`
- **THEN** the target branch points to the commit it pointed to before
- **AND** the branch `feat` was not rebased
- **AND** the exit code is 8

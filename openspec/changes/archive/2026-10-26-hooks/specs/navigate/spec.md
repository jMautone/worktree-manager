## MODIFIED Requirements

### Requirement: Moving the shell
When the target resolves to an existing directory, `wt cd` SHALL run the `pre-cd` hook, then write that directory as the directive (see the `shell-integration` capability), then start the `post-cd` hook (see the `hooks` capability). It SHALL print nothing to stdout and SHALL exit 0. Through the function, the shell ends in that directory. When the `pre-cd` hook fails or is interrupted, `wt cd` SHALL NOT write the directive, and SHALL leave done what the `hooks` capability specifies. `wt cd` SHALL accept `--no-hooks`, which runs neither hook.

#### Scenario: Jump to a worktree
- **WHEN** the shell is in a subdirectory of the main worktree and the user runs `wt cd feat`, where `feat` is a linked worktree
- **THEN** the shell's working directory is the path of `feat`
- **AND** stdout is empty
- **AND** the exit code is 0

#### Scenario: Hooks around the jump
- **WHEN** `hooks.pre-cd` is `["echo {name}> {repo_path}/pre.txt"]`, `hooks.post-cd` is `["echo {branch}> post.txt"]`, and the user runs `wt cd feat` through the function, where `feat` is a linked worktree with the branch `feat`
- **THEN** the main worktree has the file `pre.txt`, which contains `feat`
- **AND** once the `post-cd` hook finishes, `feat` has the file `post.txt`, which contains `feat`
- **AND** the shell's working directory is the path of `feat`
- **AND** stdout is empty
- **AND** the exit code is 0

### Requirement: Dry run
`wt cd --dry-run <target>` SHALL perform every check of `wt cd`, with the same errors and exit codes, SHALL NOT write the directive or run a hook, and on success SHALL print to stdout, one per line and in this order, the lines of the `pre-cd` hook, `would change directory to <destination>`, and the lines of the `post-cd` hook, and exit 0. The lines of a hook SHALL be those the `hooks` capability specifies for a dry run, and there SHALL be none with `--no-hooks`. With `--json` it SHALL print the same `wt.cd.v1` document as without `--dry-run`.

#### Scenario: Preview a jump
- **WHEN** no hooks are configured and the user runs `wt cd --dry-run feat` through the function
- **THEN** stdout is `would change directory to <path of feat>`
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: Preview a jump with hooks
- **WHEN** `hooks.pre-cd` is `["echo in {name}"]` and `hooks.post-cd` is `["git fetch"]`, and the user runs `wt cd --dry-run feat` through the function
- **THEN** stdout is `would run pre-cd hook: echo in feat`, `would change directory to <path of feat>` and `would run post-cd hook in the background: git fetch`, in that order
- **AND** the shell's working directory does not change

# Spec Delta

## MODIFIED Requirements

### Requirement: Hook configuration
The commands of an event SHALL be, in this order, the elements of the configuration key `hooks.<event>` of the user's configuration (see the `config-layers` capability), and then the elements of `hooks.<event>` in the `[hooks]` table of the repository's `.wt.toml` (see the `project-hooks-approval` capability). They SHALL run as one list: a failure of a command of either SHALL stop the event. A repository's commands SHALL replace none of the user's, and SHALL run only once approved; an event with a repository command not approved SHALL fail the command with exit code 8 before anything is done, as the `project-hooks-approval` capability specifies.

#### Scenario: User commands before the repository's
- **WHEN** the user file sets `[hooks]` with `post-create = ["echo user> order.txt"]`, `.wt.toml` sets `post-create = ["echo repo>> order.txt"]`, approved, and the user runs `wt create feat`
- **THEN** `order.txt` in the new worktree contains `user` and `repo`, each on its own line, in that order

#### Scenario: A failing user command stops the repository's
- **WHEN** the user file sets `post-create = ["exit 1"]`, `.wt.toml` sets `post-create = ["echo repo> repo.txt"]`, approved, and the user runs `wt create feat`
- **THEN** the new worktree has no file `repo.txt`
- **AND** the exit code is 7

#### Scenario: Hooks of a repository are ignored
- **WHEN** the user file sets no hooks, `.wt.toml` at the root of the main worktree contains `[hooks]` with `post-create = ["echo pwned > pwned.txt"]`, not approved, and the user runs `wt create feat`
- **THEN** no file `pwned.txt` exists anywhere in the repository
- **AND** no worktree `feat` exists
- **AND** the exit code is 8

### Requirement: Dry run with hooks
Under `--dry-run`, no hook command SHALL run, no log SHALL be written, and no background process SHALL start. Without `--json`, the command SHALL print to stdout, for each command of each event it would fire, the user's and the repository's in the order they would run, and at the place the command's own dry run specifies, the line `would run <event> hook: <command>` for a blocking event, or `would run <event> hook in the background: <command>` for a background event, with the command as rendered with the real values. With `--no-hooks` it SHALL print none of these lines. With `--json`, the document SHALL be the same as without hooks. A repository command not approved SHALL fail the dry run as it fails the command (see the `project-hooks-approval` capability).

#### Scenario: Preview of a create with hooks
- **WHEN** `fetch_before_create` is true, `hooks.pre-create` is `["echo {name}"]`, `hooks.post-create` is `["npm ci"]`, and the user runs `wt create feat --dry-run --no-cd` in a repository whose default branch is `origin/main`
- **THEN** stdout is `would fetch origin`, `would run pre-create hook: echo feat`, `would create worktree <path> on new branch feat from origin/main` and `would run post-create hook: npm ci`, in that order
- **AND** no worktree exists afterwards
- **AND** no log exists afterwards

#### Scenario: Preview with repository hooks
- **WHEN** the user file sets `post-create = ["cp ~/.npmrc ."]`, `.wt.toml` sets `post-create = ["npm ci"]`, approved, and the user runs `wt create feat --dry-run --no-cd`
- **THEN** stdout contains `would run post-create hook: cp ~/.npmrc .` followed by `would run post-create hook: npm ci`

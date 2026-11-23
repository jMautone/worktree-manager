# Spec Delta

## MODIFIED Requirements

### Requirement: Inspecting configuration
`wt config path` SHALL print the user file path, the repository file path (or state that there is none), and the path of the approvals file (see the `project-hooks-approval` capability), and whether each exists. `wt config list` SHALL print every key with its effective value and the layer it came from (`default`, `user`, `repo` or `env`). `wt config get <key>` SHALL print only the effective value followed by a newline; for a list key, it SHALL print each element followed by a newline instead (see "List values"). An unknown key given to `wt config get` SHALL be a usage error (exit 2). With `--json`, these commands SHALL use the schemas `wt.config.path.v1`, `wt.config.list.v1` and `wt.config.get.v1` respectively, each entry carrying `key`, `value` and `source`; `wt.config.path.v1` SHALL also carry `approvals`, an object with `path` and `exists`.

#### Scenario: Listing sources
- **WHEN** the user file sets `default_base = "origin/main"` and nothing else is configured
- **THEN** `wt config list` shows `default_base` with value `origin/main` and source `user`
- **AND** shows `worktree_path` with source `default`

#### Scenario: Unknown key requested
- **WHEN** the user runs `wt config get nope`
- **THEN** stderr names `nope` as an unknown key
- **AND** the exit code is 2

#### Scenario: Paths outside a repository
- **WHEN** the user runs `wt config path` outside any git repository
- **THEN** the user file path is printed
- **AND** the repository file is reported as none
- **AND** the exit code is 0

#### Scenario: Path of the approvals file
- **WHEN** `WT_CONFIG` is `/tmp/x/wt.toml`, no approval was ever made, and the user runs `wt config path`
- **THEN** stdout contains `/tmp/x/approvals.toml` and states that it does not exist

### Requirement: Hook commands
In a configuration file, `hooks` SHALL be a TOML table, and each of its keys SHALL be the name of a hook event (see the `hooks` capability). Any other value for `hooks`, such as an array or a string, SHALL fail every command that reads configuration with exit code 1 and a message naming `hooks` and the file. A key of the table that is not an event SHALL be an unknown key, named `hooks.<key>` in its warning (see "Invalid configuration").

The value of a key `hooks.<event>` SHALL be an array of strings, each one a command. A single string SHALL fail as "Configuration keys" specifies for list keys, showing the value written as an array. An element that is empty or contains only whitespace SHALL fail every command that reads configuration with exit code 1 and a message naming the key and the file. Each element SHALL be a template with the variables of its event (see the `hooks` capability, "Variables of hook commands"); an invalid one is an invalid value, as the `path-templates` capability specifies.

A repository's `.wt.toml` MAY have a `[hooks]` table, read and validated by these same rules, with no warning. Its commands SHALL NOT take part in layer precedence: they are added to the user's when the event runs (see the `hooks` and `project-hooks-approval` capabilities), and `wt config get` and `wt config list` SHALL show for `hooks.<event>` only the value of the user file or the default.

`wt config get` SHALL print each command on its own line, as written, and nothing for an empty list. `wt config list` SHALL show the list as a TOML array, and an empty one as `[]`. With `--json`, the `value` SHALL be a JSON array of strings.

#### Scenario: Two commands
- **WHEN** the user file contains `[hooks]` with `post-create = ["npm ci", "npm run build"]`
- **THEN** `wt config get hooks.post-create` prints `npm ci` and `npm run build`, each on its own line
- **AND** `wt config list` shows `hooks.post-create` with value `["npm ci", "npm run build"]` and source `user`

#### Scenario: String for a command list
- **WHEN** the user file contains `[hooks]` with `post-create = "npm ci"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `hooks.post-create` and the file
- **AND** stderr shows the value written as an array, `["npm ci"]`

#### Scenario: hooks that is not a table
- **WHEN** the user file contains `hooks = ["npm ci"]`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `hooks` and the file

#### Scenario: Event that does not exist
- **WHEN** the user file contains `[hooks]` with `post-merge = ["npm test"]`
- **THEN** `wt config list` succeeds
- **AND** stderr contains a warning naming `hooks.post-merge`

#### Scenario: Empty command
- **WHEN** the user file contains `[hooks]` with `pre-remove = ["  "]`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `hooks.pre-remove` and the file

#### Scenario: Unknown variable in a command
- **WHEN** the user file contains `[hooks]` with `post-create = ["echo {nope}"]` and the user runs `wt list`
- **THEN** stderr names `hooks.post-create`, the user file and `nope`
- **AND** the exit code is 1

#### Scenario: Repository sets hooks
- **WHEN** the user file sets no hooks and `.wt.toml` at the worktree root contains `[hooks]` with `post-create = ["npm ci"]`
- **THEN** `wt config get hooks.post-create` prints nothing
- **AND** `wt config list` shows `hooks.post-create` with source `default`
- **AND** stderr contains no warning naming `hooks.post-create`

#### Scenario: Invalid repository hook
- **WHEN** `.wt.toml` at the worktree root contains `[hooks]` with `post-create = "npm ci"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `hooks.post-create` and `.wt.toml`

#### Scenario: Command list as JSON
- **WHEN** no configuration sets hooks and the user runs `wt config get hooks.pre-cd --json`
- **THEN** stdout parses as a JSON object whose `value` is an empty JSON array

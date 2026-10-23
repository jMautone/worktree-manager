## MODIFIED Requirements

### Requirement: Configuration keys
`wt` SHALL recognise exactly the following keys, each with a type and a default:

| Key | Type | Default | Allowed in repository file |
|---|---|---|---|
| `default_base` | string | `""` (empty: the repository's default branch, see the `git-worktrees` capability) | yes |
| `worktree_path` | path template (see the `path-templates` capability) | `{repo_parent}/{repo}.worktrees/{name\|sanitize}` | yes |
| `branch_prefix` | string | `""` | yes |
| `fetch_before_create` | boolean | `true` | yes |
| `create_cd` | boolean | `true` | no |
| `repos_root` | list of paths (see "List values") | `[]` (empty: no roots, see the `workspace-discovery` capability) | no |
| `repos_depth` | integer from 1 to 3 (see "Integer values") | `1` | no |
| `hooks.pre-create` | list of commands (see "Hook commands") | `[]` | no |
| `hooks.post-create` | list of commands | `[]` | no |
| `hooks.pre-cd` | list of commands | `[]` | no |
| `hooks.post-cd` | list of commands | `[]` | no |
| `hooks.pre-remove` | list of commands | `[]` | no |
| `hooks.post-remove` | list of commands | `[]` | no |

Keys added by later changes SHALL declare the same four properties. A key whose name contains a `.` SHALL be written in a configuration file as a key of the TOML table named by the part before the `.`: `hooks.post-create` is the key `post-create` of the table `[hooks]`. In a configuration file, a value SHALL have the TOML type of its key: a boolean key takes `true` or `false`, not a string; a list key takes an array of strings, not a single string; an integer key takes an integer, not a string or a float.

#### Scenario: Defaults with no configuration files
- **WHEN** no user or repository configuration file exists and no `WT_*` variable is set
- **THEN** `wt config get worktree_path` prints `{repo_parent}/{repo}.worktrees/{name|sanitize}`
- **AND** `wt config get create_cd` prints `true`
- **AND** `wt config get repos_depth` prints `1`
- **AND** `wt config get repos_root` prints nothing
- **AND** `wt config get hooks.post-create` prints nothing

#### Scenario: String for a boolean key
- **WHEN** the user file contains `create_cd = "false"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `create_cd` and the file

#### Scenario: Repository sets create_cd
- **WHEN** `.wt.toml` at the worktree root contains `create_cd = false`
- **THEN** stderr contains a warning naming `create_cd` and `.wt.toml`
- **AND** `wt config get create_cd` prints `true`

#### Scenario: String for a list key
- **WHEN** the user file contains `repos_root = "~/GIT"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `repos_root` and the file
- **AND** stderr shows the value written as an array, `["~/GIT"]`

#### Scenario: Repository sets repos_root
- **WHEN** the user file contains `repos_root = ["~/GIT"]` and `.wt.toml` at the worktree root contains `repos_root = ["/tmp"]`
- **THEN** stderr contains a warning naming `repos_root` and `.wt.toml`
- **AND** `wt config get repos_root` prints `~/GIT`

#### Scenario: Key of a table
- **WHEN** the user file contains the table `[hooks]` with `post-create = ["npm ci"]`
- **THEN** `wt config get hooks.post-create` prints `npm ci`
- **AND** `wt config list` shows `hooks.post-create` with source `user`

### Requirement: Layer precedence
The effective value of each key SHALL come from the highest-precedence layer that sets it, in this order from lowest to highest: default, user file, repository file, environment. The environment variable for a key SHALL be `WT_` followed by the key in upper case (for example `WT_DEFAULT_BASE`), except for the keys `hooks.<event>`, which SHALL have no environment variable. An environment variable set to the empty string SHALL be treated as unset. `WT_*` variables that do not correspond to a key SHALL be ignored.

#### Scenario: Environment beats files
- **WHEN** the user file sets `default_base = "origin/main"`, `.wt.toml` sets `default_base = "origin/develop"`, and `WT_DEFAULT_BASE=origin/release`
- **THEN** `wt config get default_base` prints `origin/release`

#### Scenario: Repository beats user
- **WHEN** the user file sets `default_base = "origin/main"` and `.wt.toml` sets `default_base = "origin/develop"`
- **THEN** `wt config get default_base` prints `origin/develop`

#### Scenario: Empty environment variable
- **WHEN** the user file sets `default_base = "origin/main"` and `WT_DEFAULT_BASE` is set to the empty string
- **THEN** `wt config get default_base` prints `origin/main`

#### Scenario: No environment variable for hooks
- **WHEN** no configuration file sets hooks and `WT_HOOKS_POST_CREATE` is set to `npm ci`
- **THEN** `wt config get hooks.post-create` prints nothing
- **AND** `wt config list` shows `hooks.post-create` with source `default`

## ADDED Requirements

### Requirement: Hook commands
In a configuration file, `hooks` SHALL be a TOML table, and each of its keys SHALL be the name of a hook event (see the `hooks` capability). Any other value for `hooks`, such as an array or a string, SHALL fail every command that reads configuration with exit code 1 and a message naming `hooks` and the file. A key of the table that is not an event SHALL be an unknown key, named `hooks.<key>` in its warning (see "Invalid configuration").

The value of a key `hooks.<event>` SHALL be an array of strings, each one a command. A single string SHALL fail as "Configuration keys" specifies for list keys, showing the value written as an array. An element that is empty or contains only whitespace SHALL fail every command that reads configuration with exit code 1 and a message naming the key and the file. Each element SHALL be a template with the variables of its event (see the `hooks` capability, "Variables of hook commands"); an invalid one is an invalid value, as the `path-templates` capability specifies.

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
- **THEN** stderr contains a warning naming `hooks.post-create` and `.wt.toml`
- **AND** `wt config get hooks.post-create` prints nothing

#### Scenario: Command list as JSON
- **WHEN** no configuration sets hooks and the user runs `wt config get hooks.pre-cd --json`
- **THEN** stdout parses as a JSON object whose `value` is an empty JSON array

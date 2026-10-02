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

Keys added by later changes SHALL declare the same four properties. In a configuration file, a value SHALL have the TOML type of its key: a boolean key takes `true` or `false`, not a string.

#### Scenario: Defaults with no configuration files
- **WHEN** no user or repository configuration file exists and no `WT_*` variable is set
- **THEN** `wt config get worktree_path` prints `{repo_parent}/{repo}.worktrees/{name|sanitize}`
- **AND** `wt config get create_cd` prints `true`

#### Scenario: String for a boolean key
- **WHEN** the user file contains `create_cd = "false"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `create_cd` and the file

#### Scenario: Repository sets create_cd
- **WHEN** `.wt.toml` at the worktree root contains `create_cd = false`
- **THEN** stderr contains a warning naming `create_cd` and `.wt.toml`
- **AND** `wt config get create_cd` prints `true`

## ADDED Requirements

### Requirement: Boolean values in the environment
The environment variable of a boolean key SHALL accept `true` and `false`, compared ignoring case, and `1` and `0`. Any other non-empty value SHALL fail every command that reads configuration with exit code 1 and a message naming the variable. `wt config get` SHALL print a boolean as `true` or `false`, and with `--json` its `value` SHALL be a JSON boolean.

#### Scenario: Zero turns a key off
- **WHEN** `WT_CREATE_CD=0` is set
- **THEN** `wt config get create_cd` prints `false`
- **AND** `wt config list` shows `create_cd` with source `env`

#### Scenario: Upper case
- **WHEN** `WT_FETCH_BEFORE_CREATE=FALSE` is set
- **THEN** `wt config get fetch_before_create` prints `false`

#### Scenario: Not a boolean
- **WHEN** `WT_CREATE_CD=yes` is set and the user runs `wt list`
- **THEN** stderr names `WT_CREATE_CD`
- **AND** the exit code is 1

#### Scenario: Boolean as JSON
- **WHEN** no configuration sets `create_cd` and the user runs `wt config get create_cd --json`
- **THEN** stdout parses as a JSON object whose `value` is the boolean `true`

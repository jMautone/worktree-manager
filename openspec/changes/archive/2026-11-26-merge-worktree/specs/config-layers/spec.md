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
| `hooks.pre-merge` | list of commands | `[]` | no |
| `hooks.post-merge` | list of commands | `[]` | no |

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

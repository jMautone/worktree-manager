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

Keys added by later changes SHALL declare the same four properties. In a configuration file, a value SHALL have the TOML type of its key: a boolean key takes `true` or `false`, not a string; a list key takes an array of strings, not a single string; an integer key takes an integer, not a string or a float.

#### Scenario: Defaults with no configuration files
- **WHEN** no user or repository configuration file exists and no `WT_*` variable is set
- **THEN** `wt config get worktree_path` prints `{repo_parent}/{repo}.worktrees/{name|sanitize}`
- **AND** `wt config get create_cd` prints `true`
- **AND** `wt config get repos_depth` prints `1`
- **AND** `wt config get repos_root` prints nothing

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

### Requirement: Inspecting configuration
`wt config path` SHALL print the user file path and the repository file path (or state that there is none) and whether each exists. `wt config list` SHALL print every key with its effective value and the layer it came from (`default`, `user`, `repo` or `env`). `wt config get <key>` SHALL print only the effective value followed by a newline; for a list key, it SHALL print each element followed by a newline instead (see "List values"). An unknown key given to `wt config get` SHALL be a usage error (exit 2). With `--json`, these commands SHALL use the schemas `wt.config.path.v1`, `wt.config.list.v1` and `wt.config.get.v1` respectively, each entry carrying `key`, `value` and `source`.

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

## ADDED Requirements

### Requirement: List values
Every element of a list of paths SHALL be an absolute path or SHALL be `~` or start with `~/`; on Windows, an element starting with `~\` SHALL be accepted too. An absolute path SHALL be one that starts with `/` on macOS and Linux, and one that starts with a drive letter followed by `:\` or `:/`, or with `\\` (a UNC path), on Windows. Any other element, including an empty string, SHALL fail every command that reads configuration with exit code 1 and a message naming the key, the element, and the file or environment variable that set it. Elements SHALL be kept as written: `~` is expanded by the command that uses the key, not by the configuration.

The environment variable of a list key SHALL be split into elements at `:` on macOS and Linux and at `;` on Windows, the separator `PATH` uses on each; empty elements SHALL be ignored.

`wt config get` SHALL print each element of a list on its own line, as written, and nothing for an empty list. `wt config list` SHALL show a list as a TOML array (`["~/GIT", "/Volumes/x"]`), and an empty one as `[]`. With `--json`, the `value` of a list SHALL be a JSON array of strings.

#### Scenario: Two roots
- **WHEN** the user file contains `repos_root = ["~/GIT", "/Volumes/x"]`
- **THEN** `wt config get repos_root` prints `~/GIT` and `/Volumes/x`, each on its own line
- **AND** `wt config list` shows `repos_root` with value `["~/GIT", "/Volumes/x"]` and source `user`

#### Scenario: Relative path
- **WHEN** the user file contains `repos_root = ["GIT"]`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `repos_root`, `GIT` and the file

#### Scenario: List in the environment (macOS, Linux)
- **WHEN** on macOS or Linux `WT_REPOS_ROOT=~/GIT:/Volumes/x` is set
- **THEN** `wt config get repos_root` prints `~/GIT` and `/Volumes/x`, each on its own line
- **AND** `wt config list` shows `repos_root` with source `env`

#### Scenario: List in the environment (Windows)
- **WHEN** on Windows `WT_REPOS_ROOT=C:\Repos;D:\Work` is set
- **THEN** `wt config get repos_root` prints `C:\Repos` and `D:\Work`, each on its own line

#### Scenario: Path without a drive (Windows)
- **WHEN** on Windows the user file contains `repos_root = ["\\Repos"]`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `repos_root` and `\Repos`

#### Scenario: List as JSON
- **WHEN** no configuration sets `repos_root` and the user runs `wt config get repos_root --json`
- **THEN** stdout parses as a JSON object whose `value` is an empty JSON array

### Requirement: Integer values
In a configuration file, an integer key SHALL take a TOML integer. Its environment variable SHALL take the integer written in decimal digits. A value outside the key's range, or an environment value that is not an integer, SHALL fail every command that reads configuration with exit code 1 and a message naming the key and the file, or naming the environment variable. `wt config get` SHALL print an integer in decimal, and with `--json` its `value` SHALL be a JSON number.

#### Scenario: Depth from the file
- **WHEN** the user file contains `repos_depth = 2`
- **THEN** `wt config get repos_depth` prints `2`

#### Scenario: Out of range
- **WHEN** the user file contains `repos_depth = 4`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `repos_depth` and the file

#### Scenario: Integer in the environment
- **WHEN** `WT_REPOS_DEPTH=3` is set
- **THEN** `wt config get repos_depth` prints `3`
- **AND** `wt config list` shows `repos_depth` with source `env`

#### Scenario: Not an integer in the environment
- **WHEN** `WT_REPOS_DEPTH=two` is set and the user runs `wt repos`
- **THEN** stderr names `WT_REPOS_DEPTH`
- **AND** the exit code is 1

#### Scenario: Integer as JSON
- **WHEN** no configuration sets `repos_depth` and the user runs `wt config get repos_depth --json`
- **THEN** stdout parses as a JSON object whose `value` is the number `1`

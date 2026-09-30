# Spec Delta

## Purpose

Defines where `wt` reads its configuration from on each operating system, how values from defaults, the user file, the repository file and the environment combine, which keys a repository is allowed to set, and how the effective configuration can be inspected.

## ADDED Requirements

### Requirement: Configuration keys
`wt` SHALL recognise exactly the following keys in this change, each with a type and a default:

| Key | Type | Default | Allowed in repository file |
|---|---|---|---|
| `default_base` | string | `""` (empty: the repository's default branch, resolved by the command that consumes it) | yes |
| `worktree_path` | string, non-empty | `{repo_parent}/{repo}.worktrees/{branch\|sanitize}` | yes |

Keys added by later changes SHALL declare the same four properties.

#### Scenario: Defaults with no configuration files
- **WHEN** no user or repository configuration file exists and no `WT_*` variable is set
- **THEN** `wt config get worktree_path` prints `{repo_parent}/{repo}.worktrees/{branch|sanitize}`

### Requirement: User configuration file location
The user configuration file SHALL be:
- on macOS and Linux: `$XDG_CONFIG_HOME/wt/config.toml` when `XDG_CONFIG_HOME` is set to an absolute path, otherwise `~/.config/wt/config.toml`;
- on Windows: `%APPDATA%\wt\config.toml`.

When the `WT_CONFIG` environment variable is set to a non-empty value, it SHALL replace that path on every OS. A missing user file SHALL NOT be an error.

#### Scenario: Default location on macOS
- **WHEN** `XDG_CONFIG_HOME` and `WT_CONFIG` are unset on macOS
- **THEN** `wt config path` reports the user file as `~/.config/wt/config.toml` expanded to the home directory

#### Scenario: XDG override on macOS or Linux
- **WHEN** `XDG_CONFIG_HOME=/tmp/cfg` on macOS or Linux
- **THEN** the user file is `/tmp/cfg/wt/config.toml`

#### Scenario: Default location on Windows
- **WHEN** `WT_CONFIG` is unset on Windows and `APPDATA` is `C:\Users\me\AppData\Roaming`
- **THEN** the user file is `C:\Users\me\AppData\Roaming\wt\config.toml`

#### Scenario: Explicit override
- **WHEN** `WT_CONFIG=/some/where/wt.toml`
- **THEN** that file is read as the user file on every OS

### Requirement: Repository configuration file
When the working directory is inside a worktree, the file `.wt.toml` at the root of that worktree SHALL be read as the repository layer. Outside any worktree there SHALL be no repository layer. Keys not allowed in the repository file SHALL be ignored with a warning naming the key and the file, because the repository file comes from a possibly untrusted clone.

#### Scenario: Repository sets an allowed key
- **WHEN** `.wt.toml` at the worktree root contains `default_base = "origin/develop"`
- **THEN** `wt config get default_base` prints `origin/develop`

#### Scenario: Repository sets a key it may not set
- **WHEN** `.wt.toml` sets a known key that is not allowed in repository files
- **THEN** the key is ignored
- **AND** stderr contains a warning naming the key and `.wt.toml`
- **AND** the exit code is unaffected

### Requirement: Layer precedence
The effective value of each key SHALL come from the highest-precedence layer that sets it, in this order from lowest to highest: default, user file, repository file, environment. The environment variable for a key SHALL be `WT_` followed by the key in upper case (for example `WT_DEFAULT_BASE`). An environment variable set to the empty string SHALL be treated as unset. `WT_*` variables that do not correspond to a key SHALL be ignored.

#### Scenario: Environment beats files
- **WHEN** the user file sets `default_base = "origin/main"`, `.wt.toml` sets `default_base = "origin/develop"`, and `WT_DEFAULT_BASE=origin/release`
- **THEN** `wt config get default_base` prints `origin/release`

#### Scenario: Repository beats user
- **WHEN** the user file sets `default_base = "origin/main"` and `.wt.toml` sets `default_base = "origin/develop"`
- **THEN** `wt config get default_base` prints `origin/develop`

#### Scenario: Empty environment variable
- **WHEN** the user file sets `default_base = "origin/main"` and `WT_DEFAULT_BASE` is set to the empty string
- **THEN** `wt config get default_base` prints `origin/main`

### Requirement: Invalid configuration
A configuration file that is not valid TOML SHALL fail every command that reads configuration with exit code 1 and a message naming the file and the line of the error. A known key whose value has the wrong type or violates its constraint SHALL fail with exit code 1 and a message naming the key and the file (or the environment variable) that set it. An unknown key in the user file SHALL produce a warning naming the key and SHALL NOT fail the command.

#### Scenario: Malformed TOML
- **WHEN** the user file contains `default_base = ` with no value on line 3
- **THEN** `wt list` fails with exit code 1
- **AND** stderr names the file and line 3

#### Scenario: Wrong type
- **WHEN** the user file contains `default_base = 42`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `default_base` and the file

#### Scenario: Unknown key
- **WHEN** the user file contains `colour = "blue"`
- **THEN** `wt config list` succeeds
- **AND** stderr contains a warning naming `colour`

### Requirement: Inspecting configuration
`wt config path` SHALL print the user file path and the repository file path (or state that there is none) and whether each exists. `wt config list` SHALL print every key with its effective value and the layer it came from (`default`, `user`, `repo` or `env`). `wt config get <key>` SHALL print only the effective value followed by a newline. An unknown key given to `wt config get` SHALL be a usage error (exit 2). With `--json`, these commands SHALL use the schemas `wt.config.path.v1`, `wt.config.list.v1` and `wt.config.get.v1` respectively, each entry carrying `key`, `value` and `source`.

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

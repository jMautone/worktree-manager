## MODIFIED Requirements

### Requirement: Repository configuration file
When the working directory is inside a worktree, the file `.wt.toml` at the root of that worktree SHALL be read as the repository layer. Outside any worktree there SHALL be no repository layer. A command that operates on a repository other than the one associated with the working directory, because its target is a qualified target or a worktree of another repository (see the `cross-repo-resolution` capability), SHALL instead read as the repository layer the `.wt.toml` at the root of that repository's main worktree, and none when that repository is bare. The `wt config` commands SHALL always use the repository layer of the working directory. Keys not allowed in the repository file SHALL be ignored with a warning naming the key and the file, because the repository file comes from a possibly untrusted clone; since `repos_root` is one of them, the roots are always those of the user file and the environment.

#### Scenario: Repository sets an allowed key
- **WHEN** `.wt.toml` at the worktree root contains `default_base = "origin/develop"`
- **THEN** `wt config get default_base` prints `origin/develop`

#### Scenario: Repository sets a key it may not set
- **WHEN** `.wt.toml` sets a known key that is not allowed in repository files
- **THEN** the key is ignored
- **AND** stderr contains a warning naming the key and `.wt.toml`
- **AND** the exit code is unaffected

#### Scenario: File of the other repository
- **WHEN** the shell is inside `<root>/web`, whose `.wt.toml` contains `default_base = "trunk"`, the main worktree of `<root>/api` has a `.wt.toml` containing `default_base = "develop"`, `api` has the branch `develop`, and the user runs `wt create api:feat --dry-run`
- **THEN** stdout contains `on new branch feat from develop`

#### Scenario: Other repository without a file
- **WHEN** the shell is inside `<root>/web`, whose `.wt.toml` contains `branch_prefix = "web/"`, the main worktree of `<root>/api` has no `.wt.toml`, and the user runs `wt create api:feat --dry-run`
- **THEN** stdout contains `on new branch feat`

#### Scenario: Merged into the other repository's base
- **WHEN** the main worktree of `<root>/api` has a `.wt.toml` containing `default_base = "develop"`, the tip of the branch `feat` of `api` is an ancestor of `develop` but not of its `origin/main`, and from inside `<root>/web` the user runs `wt remove api:feat`
- **THEN** the branch `feat` of `api` no longer exists

#### Scenario: Inspecting from another repository
- **WHEN** the shell is inside `<root>/web`, whose `.wt.toml` contains `default_base = "trunk"`, and the user runs `wt config get default_base`
- **THEN** stdout is `trunk`

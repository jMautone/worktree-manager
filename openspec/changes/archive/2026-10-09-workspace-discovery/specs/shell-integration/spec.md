## MODIFIED Requirements

### Requirement: Completions
Loading the script SHALL also register completions for `wt`. They SHALL offer the subcommands and flags of every command; for the argument of `wt cd`, the `NAME` of every worktree of the repository associated with the shell's working directory whose directory exists, followed by the name of every repository found under the roots in `repos_root` (see the `workspace-discovery` capability) that starts with the word being completed, compared ignoring letter case, leaving out names already offered; and for the argument of `wt shell init`, the supported shell names. Requesting a completion SHALL never write to the terminal, not even a configuration warning: outside a repository, or when git fails, `wt cd` offers no worktree names; when the configuration cannot be read, or no root is configured, it offers no repository names. In zsh, completions SHALL be registered only if the completion system was initialized (`compinit`) before the script was loaded; in bash, only if the `bash-completion` package was loaded before. In both cases, without it the script SHALL still load silently and the function SHALL work.

#### Scenario: Worktree names
- **WHEN** inside a repository with linked worktrees `feat` and `fix` the user requests completions for `wt cd `
- **THEN** `feat` and `fix` are offered

#### Scenario: Subcommands
- **WHEN** the user requests completions for `wt sh`
- **THEN** `shell` is offered

#### Scenario: Shell names
- **WHEN** the user requests completions for `wt shell init `
- **THEN** `zsh`, `bash`, `fish` and `pwsh` are offered

#### Scenario: Outside a repository
- **WHEN** no configuration sets `repos_root` and, outside any repository, the user requests completions for `wt cd `
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

#### Scenario: Repository names outside a repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` and `<root>/web` are repositories, and outside any repository the user requests completions for `wt cd `
- **THEN** `api` and `web` are offered

#### Scenario: Worktrees and repositories together
- **WHEN** `<root>/web` is a repository and, inside a repository with the linked worktree `feat`, the user requests completions for `wt cd `
- **THEN** `feat` and `web` are offered

#### Scenario: Repository prefix ignores case
- **WHEN** `<root>/TrendFisher` and `<root>/api` are repositories and the user requests completions for `wt cd trend`
- **THEN** `TrendFisher` is offered
- **AND** `api` is not

#### Scenario: Invalid configuration
- **WHEN** the user file is not valid TOML and, inside a repository with the linked worktree `feat`, the user requests completions for `wt cd `
- **THEN** `feat` is offered
- **AND** nothing is written to the terminal

#### Scenario: Missing root
- **WHEN** `repos_root` names a directory that does not exist and the user requests completions for `wt cd `
- **THEN** nothing is written to the terminal

#### Scenario: zsh without compinit
- **WHEN** in zsh the script is loaded before `compinit` has run
- **THEN** nothing is written to stdout or stderr
- **AND** `wt cd feat` works

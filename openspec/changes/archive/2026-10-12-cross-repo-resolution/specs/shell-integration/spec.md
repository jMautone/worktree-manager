## MODIFIED Requirements

### Requirement: Completions
Loading the script SHALL also register completions for `wt`. They SHALL offer the subcommands and flags of every command, and for the argument of `wt shell init`, the supported shell names.

For the argument of `wt cd` and of `wt path`, they SHALL offer, each name once and in this order:

1. the `NAME` of every worktree of the repository associated with the shell's working directory whose directory exists;
2. the name of every repository found under the roots in `repos_root` (see the `workspace-discovery` capability) that starts with the word being completed, compared ignoring letter case;
3. the `NAME` of every linked worktree of the other repositories found under the roots whose directory exists (see the `cross-repo-resolution` capability).

Each name SHALL be offered with a description: `worktree` for the first group, `repository` for the second, and `worktree of <display name>` for the third (see the `cross-repo-resolution` capability). When the word being completed contains `:`, they SHALL instead offer, for every worktree whose directory exists of the repository its repository side designates, other than its main worktree, the repository side as typed followed by `:` and the worktree's `NAME`; and nothing when the repository side does not designate exactly one repository.

Requesting a completion SHALL never write to the terminal, not even a configuration warning: outside a repository, or when git fails for it, the first group is empty; when the configuration cannot be read, or no root is configured, the second and third groups are empty; a repository that git cannot read only leaves out its own worktrees. In zsh, completions SHALL be registered only if the completion system was initialized (`compinit`) before the script was loaded; in bash, only if the `bash-completion` package was loaded before. In both cases, without it the script SHALL still load silently and the function SHALL work.

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

#### Scenario: Worktrees of other repositories
- **WHEN** `<root>/cortex` has the linked worktree `angry-jang-12fc4f` and from inside `<root>/web` the user requests completions for `wt cd `
- **THEN** `angry-jang-12fc4f` is offered, with the description `worktree of cortex`
- **AND** `cortex` is offered, with the description `repository`
- **AND** `web`, the main worktree of the current repository, is offered once, with the description `worktree`

#### Scenario: Same worktree name in two repositories
- **WHEN** `<root>/api` and `<root>/web` each have a linked worktree named `feat` and outside any repository the user requests completions for `wt cd `
- **THEN** `feat` is offered once

#### Scenario: Qualified target
- **WHEN** `<root>/cortex` has the linked worktrees `angry-jang-12fc4f` and `sweet-roentgen-4a36e6` and the user requests completions for `wt cd cortex:`
- **THEN** `cortex:angry-jang-12fc4f` and `cortex:sweet-roentgen-4a36e6` are offered
- **AND** the main worktree of `cortex` is not offered

#### Scenario: Repository side as typed
- **WHEN** `<root>/cortex` is the only repository whose name starts with `co` and has the linked worktree `angry-jang-12fc4f`, and the user requests completions for `wt cd co:`
- **THEN** `co:angry-jang-12fc4f` is offered

#### Scenario: Ambiguous repository side
- **WHEN** `<root>/cv-scorer-backend` and `<root>/cv-scorer-front` are repositories and the user requests completions for `wt cd cv:`
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

#### Scenario: Targets of path
- **WHEN** `<root>/cortex` has the linked worktree `angry-jang-12fc4f` and outside any repository the user requests completions for `wt path `
- **THEN** `cortex` and `angry-jang-12fc4f` are offered

#### Scenario: Repository that git cannot read
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` has the linked worktree `feat`, and outside any repository the user requests completions for `wt cd `
- **THEN** `feat`, `api` and `broken` are offered
- **AND** nothing is written to the terminal

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

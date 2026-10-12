## MODIFIED Requirements

### Requirement: Arguments and flags
`wt create` SHALL take exactly one argument, `<name>`, which may also be written `<repo>:<name>` (see "Repository"), and SHALL accept these flags besides the global ones:

| Flag | Meaning |
|---|---|
| `-b`, `--branch <branch>` | the branch to create, used as given |
| `--base <ref>` | the revision the new branch starts at |
| `-x <cmd>` | a command to run in the new worktree |
| `--cd` | move the shell into the new worktree |
| `--no-cd` | do not move the shell |

In the form `<repo>:<name>`, the text before the first `:` is the repository and the rest is the name, and the rules below for `<name>` apply to that name. These SHALL be usage errors, with exit code 2, detected before anything is fetched, created or run: a missing `<name>` or an extra argument; an empty `<name>`, including the empty name of `<repo>:`; an empty repository, as in `:feat`; a `<name>` with a component, separated by `/` or `\`, equal to `.` or `..`; `--cd` together with `--no-cd`; `-x` with an empty command; and `-x` together with `--json`.

#### Scenario: No name
- **WHEN** the user runs `wt create`
- **THEN** stderr reports the missing `<name>` argument
- **AND** the exit code is 2

#### Scenario: Name that escapes its directory
- **WHEN** the user runs `wt create ../x -b fix`
- **THEN** stderr names `../x`
- **AND** nothing is created
- **AND** the exit code is 2

#### Scenario: Contradictory flags
- **WHEN** the user runs `wt create feat --cd --no-cd`
- **THEN** the exit code is 2
- **AND** nothing is created

#### Scenario: Command with JSON
- **WHEN** the user runs `wt create feat -x claude --json`
- **THEN** stderr is a `wt.error.v1` document with `code` 2
- **AND** nothing is created

#### Scenario: Repository without a name
- **WHEN** the user runs `wt create api:`
- **THEN** stderr reports the empty name
- **AND** nothing is created
- **AND** the exit code is 2

#### Scenario: Name without a repository
- **WHEN** the user runs `wt create :feat`
- **THEN** stderr names `:feat`
- **AND** nothing is created
- **AND** the exit code is 2

### Requirement: Repository
`wt create <name>` SHALL create the worktree in the repository associated with the working directory (see the `git-worktrees` capability), whether that directory is in the main worktree, in a linked worktree, in a subdirectory of either, or in a bare repository. Outside a repository it SHALL fail as that capability specifies (exit 3) without creating anything and, when `repos_root` names at least one root, with a hint naming the form `<repo>:<name>`.

`wt create <repo>:<name>` SHALL create the worktree in the repository that `<repo>` designates, resolved as the repository side of a qualified target (see the `cross-repo-resolution` capability), with its errors and exit codes, from any working directory. Everything this capability specifies about the repository SHALL then refer to that repository, as if `wt create <name>` had been started in its main worktree: the default branch, the existing and remote-tracking branches, the remotes fetched, the base, the worktree path and its template variables, and the worktrees already registered. Its repository layer of configuration SHALL be as the `config-layers` capability specifies for a command that operates on another repository.

#### Scenario: Outside a repository
- **WHEN** no configuration sets `repos_root` and the user runs `wt create feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** the exit code is 3

#### Scenario: Outside a repository with roots configured
- **WHEN** `repos_root` names at least one root and the user runs `wt create feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** a `hint:` line contains `<repo>:feat`
- **AND** nothing is created
- **AND** the exit code is 3

#### Scenario: From a bare repository
- **WHEN** the working directory is a bare repository and the user runs `wt create feat`
- **THEN** a linked worktree of that repository is created with the branch `feat` checked out
- **AND** the exit code is 0

#### Scenario: In another repository from outside any repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` is a clone whose default branch is `origin/main`, and outside any repository the user runs `wt create api:feat` through the function with the default `worktree_path`
- **THEN** stdout contains `created worktree <root>/api.worktrees/feat on new branch feat from origin/main`
- **AND** the shell's working directory is `<root>/api.worktrees/feat`
- **AND** the exit code is 0

#### Scenario: In another repository from inside a repository
- **WHEN** the shell is inside `<root>/web` and the user runs `wt create api:feat`
- **THEN** the new worktree belongs to `<root>/api`
- **AND** `<root>/web` has no new worktree and no branch `feat`

#### Scenario: Command in another repository
- **WHEN** outside any repository the user runs `wt create api:feat -x "git branch --show-current"`
- **THEN** stdout contains `feat`
- **AND** the new worktree belongs to `<root>/api`

#### Scenario: Repository by prefix
- **WHEN** `<root>/TrendFisher` is the only repository whose name starts with `trend`, ignoring case, and the user runs `wt create trend:feat`
- **THEN** the new worktree belongs to `<root>/TrendFisher`

#### Scenario: No such repository
- **WHEN** no repository under the roots starts with `nope` and the user runs `wt create nope:feat`
- **THEN** stderr's first line is `wt: no repository named "nope"`
- **AND** nothing is created
- **AND** the exit code is 3

### Requirement: JSON output
`wt create <name> --json` SHALL create the worktree and move the shell like `wt create <name>`, and SHALL print `{"schema":"wt.create.v1","name":...,"path":...,"branch":...,"base":...,"head":...}` instead of the `created worktree` line, where `name` is `<name>` as given, without the repository and its `:` in the form `<repo>:<name>`, `path` the new worktree's absolute path in the operating system's native form, `branch` the new branch, `base` the base as written, and `head` the full commit id the branch starts at.

#### Scenario: Created worktree as JSON
- **WHEN** the user runs `wt create feat --json` in a repository whose default branch is `origin/main`
- **THEN** stdout parses as a JSON object with `schema` `wt.create.v1`, `name` `feat`, `branch` `feat` and `base` `origin/main`
- **AND** `head` is the full commit id of `origin/main`
- **AND** `path` is the path of the new worktree

#### Scenario: Created in another repository as JSON
- **WHEN** the user runs `wt create api:feat --json --no-cd`
- **THEN** stdout parses as a JSON object with `schema` `wt.create.v1` and `name` `feat`
- **AND** `path` is the path of the new worktree of `<root>/api`

### Requirement: Completions
Completions for `wt create` SHALL offer, for `<name>`, the display name of every repository found under the roots (see the `cross-repo-resolution` capability) that starts with the word being completed, ignoring letter case, followed by `:`, without the shell adding a space after it; and nothing when the word already contains `:`, or when no root is configured. For the value of `--base`, they SHALL offer the local branches and the remote-tracking branches, written `<remote>/<branch>`, except `<remote>/HEAD`, of the repository that `<name>` designates when it is written `<repo>:<name>`, and otherwise of the repository associated with the shell's working directory. They SHALL offer nothing for the value of `-b`, and never file names. Requesting a completion SHALL never write to the terminal: when that repository cannot be determined, or when git or the configuration cannot be read, nothing is offered for the value concerned.

#### Scenario: Bases
- **WHEN** in a clone with the local branch `main` and the remote-tracking branch `origin/main`, the user requests completions for `wt create feat --base `
- **THEN** `main` and `origin/main` are offered
- **AND** `origin/HEAD` is not offered

#### Scenario: Bases outside a repository
- **WHEN** outside any repository the user requests completions for `wt create feat --base `
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

#### Scenario: Bases of another repository
- **WHEN** `<root>/api` has the local branch `develop`, the shell is in a directory that is in no repository, and the user requests completions for `wt create api:feat --base `
- **THEN** `develop` is offered

#### Scenario: Repositories for the name
- **WHEN** `<root>/api` and `<root>/web` are repositories and the user requests completions for `wt create a`
- **THEN** `api:` is offered, and the shell adds no space after it
- **AND** `web:` is not offered

#### Scenario: Name after the repository
- **WHEN** the user requests completions for `wt create api:`
- **THEN** nothing is offered

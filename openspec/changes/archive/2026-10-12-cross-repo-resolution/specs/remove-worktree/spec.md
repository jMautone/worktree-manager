## MODIFIED Requirements

### Requirement: Target
`wt remove` SHALL resolve `<target>` as follows:

- `^` and `@` as the `navigate` capability specifies in "Main and current worktree": in the repository associated with the working directory only;
- a target that contains `:` as a qualified target (see the `cross-repo-resolution` capability), whose empty name side designates the repository's main worktree;
- any other target first among the worktrees of the repository associated with the working directory, as the `navigate` capability specifies for the current repository in "Resolving a name": the worktree with that `NAME` or, when none has it, the worktree with that branch, exactly, with exit 4 and nothing else searched when more than one matches; then, when that matched nothing or the working directory is inside no repository, and `repos_root` names at least one root, among the linked worktrees of the other repositories, as the `cross-repo-resolution` capability specifies. Repositories SHALL NOT be matched by their name.

When nothing matches, the command SHALL fail with exit code 3. When the working directory is inside a repository, or `repos_root` names at least one root, the message SHALL be `no worktree named "<target>"`, with a hint naming `wt list --all-repos` when `repos_root` names at least one root and `wt list` otherwise. When the working directory is inside no repository and `repos_root` names no root, the command SHALL fail as the `git-worktrees` capability specifies (`not a git repository: <dir>`), with a hint naming `repos_root`.

When the target resolves to the main worktree, including a bare repository, the command SHALL fail with exit code 2 and the message `cannot remove the main worktree`, and nothing SHALL be removed.

#### Scenario: By branch
- **WHEN** the linked worktree at `<parent>/repo.worktrees/feature-abc1` has the branch `feature/abc1` checked out and the user runs `wt remove feature/abc1`
- **THEN** that worktree is removed

#### Scenario: Current worktree
- **WHEN** the working directory is `<feat>/src`, inside the linked worktree `feat`, and the user runs `wt remove @`
- **THEN** the worktree `feat` is removed

#### Scenario: Main worktree
- **WHEN** the user runs `wt remove ^`
- **THEN** stderr's first line is `wt: cannot remove the main worktree`
- **AND** the exit code is 2

#### Scenario: No such worktree
- **WHEN** no configuration sets `repos_root` and the user runs `wt remove nope` inside a repository with no worktree or branch named `nope`
- **THEN** stderr's first line is `wt: no worktree named "nope"`
- **AND** a `hint:` line names `wt list`
- **AND** the exit code is 3

#### Scenario: Ambiguous name
- **WHEN** two worktrees of the repository live at `/a/x` and `/b/x` and the user runs `wt remove x`
- **THEN** the exit code is 4
- **AND** neither worktree is removed

#### Scenario: Outside a repository
- **WHEN** no configuration sets `repos_root` and the user runs `wt remove feat` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** a `hint:` line names `repos_root`
- **AND** the exit code is 3

#### Scenario: Worktree of another repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` has the clean linked worktree `feat` whose branch is merged, the shell is in the main worktree of `<root>/web`, which has no worktree or branch named `feat`, and the user runs `wt remove feat` through the function
- **THEN** stdout's first line is `removed worktree <path of feat>`
- **AND** the directory of `feat` and the branch `feat` of `api` no longer exist
- **AND** the shell's working directory does not change
- **AND** the exit code is 0

#### Scenario: From outside any repository
- **WHEN** `<root>/api` has the clean linked worktree `feat` and the user runs `wt remove feat` outside any repository
- **THEN** the worktree `feat` of `api` is removed
- **AND** the exit code is 0

#### Scenario: Qualified target
- **WHEN** the shell is inside `<root>/web`, `web` and `<root>/api` both have a clean linked worktree named `feat`, and the user runs `wt remove api:feat`
- **THEN** the worktree `feat` of `api` is removed
- **AND** the worktree `feat` of `web` still exists

#### Scenario: Qualified repository
- **WHEN** the user runs `wt remove api:`
- **THEN** stderr's first line is `wt: cannot remove the main worktree`
- **AND** the exit code is 2

#### Scenario: A repository is not a target
- **WHEN** `<root>/feature-flags` is a repository, no worktree of any repository is named `feature-flags` or has that branch checked out, and from inside `<root>/web` the user runs `wt remove feature-flags`
- **THEN** stderr's first line is `wt: no worktree named "feature-flags"`
- **AND** a `hint:` line names `wt list --all-repos`
- **AND** the exit code is 3

#### Scenario: Same name in two repositories
- **WHEN** `<root>/api` and `<root>/web` each have a clean linked worktree named `feat` and the user runs `wt remove feat` outside any repository
- **THEN** stderr has a `hint:` line containing `api:feat` and another containing `web:feat`
- **AND** both worktrees still exist
- **AND** the exit code is 4

#### Scenario: Lock in another repository
- **WHEN** `<root>/api` has the linked worktree `feat` and from inside `<root>/web` the user runs `wt lock api:feat review`
- **THEN** stdout is `locked worktree <path of feat>`
- **AND** in the output of `wt -C <root>/api list --json`, the element of `feat` has `"locked": true` and `"locked_reason": "review"`

### Requirement: Completions
Completions SHALL offer, for the `<target>` of `wt remove` and of `wt lock`, the `NAME` of every worktree that is neither a main worktree nor locked, including those whose directory no longer exists, among the worktrees of the repository associated with the shell's working directory and the linked worktrees of the other repositories found under the roots (see the `cross-repo-resolution` capability); for the `<target>` of `wt unlock`, the `NAME` of every locked worktree among the same; and nothing for the `<reason>` of `wt lock`. Each name SHALL be offered once, with a description: `worktree` for a worktree of the current repository, and `worktree of <display name>` for a worktree of another repository. When the word being completed contains `:`, they SHALL instead offer, for every worktree of the repository its repository side designates that the same rule accepts, the repository side as typed followed by `:` and the worktree's `NAME`; and nothing when the repository side does not designate exactly one repository. They SHALL never offer file names. Requesting a completion SHALL never write to the terminal: outside a repository with no root configured, nothing is offered; a repository that git cannot read, or a configuration that cannot be read, only leaves out the names it would have offered.

#### Scenario: Targets for remove
- **WHEN** a repository has the linked worktrees `feat`, `gone` (its directory deleted) and `usb` (locked), and the user requests completions for `wt remove `
- **THEN** `feat` and `gone` are offered
- **AND** `usb` and the main worktree's `NAME` are not offered

#### Scenario: Targets for unlock
- **WHEN** in the same repository the user requests completions for `wt unlock `
- **THEN** only `usb` is offered

#### Scenario: Worktrees of other repositories
- **WHEN** `<root>/api` has the linked worktrees `feat` and `usb` (locked), and from inside `<root>/web` the user requests completions for `wt remove `
- **THEN** `feat` is offered, with the description `worktree of api`
- **AND** `usb` is not offered
- **AND** neither `api` nor `web` is offered

#### Scenario: Qualified target
- **WHEN** `<root>/api` has the linked worktrees `feat` and `usb` (locked) and the user requests completions for `wt remove api:`
- **THEN** `api:feat` is offered
- **AND** `api:usb` and the main worktree of `api` are not offered

#### Scenario: Outside a repository
- **WHEN** no configuration sets `repos_root` and outside any repository the user requests completions for `wt remove `
- **THEN** nothing is offered
- **AND** nothing is written to the terminal

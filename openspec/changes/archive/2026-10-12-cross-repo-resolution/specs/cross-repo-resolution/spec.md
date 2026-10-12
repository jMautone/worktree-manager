## Purpose

Defines how `wt` names a worktree that may live in any repository under the configured roots: the qualified form `<repo>:<name>`, the display name that tells repositories with the same name apart, the search among the linked worktrees of the other repositories, and what happens when git cannot read one of them.

## ADDED Requirements

### Requirement: Worktrees of other repositories
When a command searches the worktrees of other repositories (see the `navigate` and `remove-worktree` capabilities, which say when), the candidates SHALL be the linked worktrees of every repository found under the roots (see the `workspace-discovery` capability) other than the repository associated with the working directory, as git reports them, including those whose directory no longer exists. The main worktree of a repository, and a bare repository, SHALL NOT be candidates: they are reached through the repository's name. The target SHALL be resolved among the candidates in two steps:

1. the candidates whose `NAME` (the last component of the path) equals the target;
2. only if step 1 matched none, the candidates whose branch equals the target.

These comparisons SHALL be exact and case-sensitive on every operating system, and each step SHALL consider the candidates of all the other repositories together. A single match SHALL be the result. More than one match in the step that matched SHALL fail with exit code 4, the message `"<target>" matches more than one worktree`, and one `hint:` line per candidate containing its qualified form, `<display name>:<NAME>` (see "Display name of a repository"), and its path. No match SHALL leave the target unresolved, for the command to report.

#### Scenario: From another repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` has the linked worktree `<root>/api.worktrees/feat`, the shell is inside `<root>/web`, which has no worktree or branch named `feat`, and the user runs `wt cd feat`
- **THEN** the shell's working directory is `<root>/api.worktrees/feat`
- **AND** the exit code is 0

#### Scenario: From outside any repository
- **WHEN** `<root>/api` has the linked worktree `feat`, the shell is in a directory that is in no repository, and the user runs `wt cd feat`
- **THEN** the destination is the worktree `feat` of `api`

#### Scenario: By branch
- **WHEN** `<root>/api` has the linked worktree `<root>/api.worktrees/feature-abc1` with the branch `feature/abc1` checked out, and from inside `<root>/web` the user runs `wt cd feature/abc1`
- **THEN** the destination is `<root>/api.worktrees/feature-abc1`

#### Scenario: Name before branch
- **WHEN** `<root>/api` has a linked worktree named `fix`, `<root>/web` has a linked worktree named `x` with the branch `fix` checked out, and from outside any repository the user runs `wt cd fix`
- **THEN** the destination is the worktree `fix` of `api`

#### Scenario: Main worktrees are not candidates
- **WHEN** the main worktrees of `<root>/api` and `<root>/web` have the branch `main` checked out, no linked worktree has it, and from outside any repository the user runs `wt cd main`
- **THEN** stderr's first line is `wt: no worktree or repository named "main"`
- **AND** the exit code is 3

#### Scenario: Same name in two repositories
- **WHEN** `<root>/api` and `<root>/web` each have a linked worktree named `feat`, and from outside any repository the user runs `wt cd feat`
- **THEN** stderr's first line is `wt: "feat" matches more than one worktree`
- **AND** stderr has a `hint:` line containing `api:feat` and the path of the `feat` of `api`, and another containing `web:feat` and the path of the `feat` of `web`
- **AND** the shell's working directory does not change
- **AND** the exit code is 4

#### Scenario: Directory gone
- **WHEN** the directory of the linked worktree `feat` of `<root>/api` was deleted without `git worktree remove`, and from outside any repository the user runs `wt remove feat`
- **THEN** `wt -C <root>/api list` does not show `feat`
- **AND** the exit code is 0

### Requirement: Qualified targets
A target that contains `:` SHALL be a qualified target: the text before the first `:` is its repository side, and the rest, which may itself contain `:`, is its name side. A qualified target SHALL be resolved only as this capability specifies, and never as a name of the current repository, of a repository or of a worktree of another repository. An empty repository side, as in `:feat` or `:`, SHALL be a usage error (exit 2) with a message naming the target. The commands that accept a qualified target say what its name side designates (see "Name side").

#### Scenario: Name side with a colon (macOS, Linux)
- **WHEN** on macOS or Linux `<root>/api` has a linked worktree whose directory is named `a:b`, and the user runs `wt cd api:a:b`
- **THEN** the destination is that worktree

#### Scenario: Missing repository side
- **WHEN** the user runs `wt cd :feat`
- **THEN** stderr's first line names `:feat`
- **AND** the shell's working directory does not change
- **AND** the exit code is 2

#### Scenario: Only a colon
- **WHEN** the user runs `wt cd :`
- **THEN** the exit code is 2

#### Scenario: Current repository is not searched
- **WHEN** `<root>/api` is a repository, the shell is inside `<root>/web`, `web` has a linked worktree named `api`, and the user runs `wt cd api:`
- **THEN** the shell's working directory is `<root>/api`

### Requirement: Repository side
When `repos_root` names no root, a qualified target SHALL fail with exit code 1, the message `no repository roots configured`, and a hint naming the `repos_root` key and the path of the user configuration file, as `wt repos` does (see the `workspace-discovery` capability).

A repository side with no path separator SHALL be resolved among the repositories found under the roots in the three steps that the `navigate` capability specifies for repositories: the exact name, the name ignoring letter case, and the names that start with it ignoring letter case.

A repository side with a path separator (`/` on every operating system; `/` or `\` on Windows) SHALL be a path suffix: it SHALL match the repositories whose path, as `wt repos` lists it, ends with its components, each compared as a whole component and ignoring letter case on every operating system. Separators at its start or end, and repeated separators, SHALL be ignored. A path suffix SHALL NOT match by prefix.

A single matching repository SHALL be the result. No match SHALL fail with exit code 3, the message `no repository named "<repository side>"`, and a hint naming `wt repos`. More than one match SHALL fail with exit code 4, the message `"<repository side>" matches more than one repository`, and one `hint:` line per candidate containing its display name followed by `:` and its path.

#### Scenario: Prefix of a repository
- **WHEN** `<root>/TrendFisher` is the only repository whose name starts with `trend`, ignoring case, it has the linked worktree `feat`, and the user runs `wt cd trend:feat`
- **THEN** the destination is the worktree `feat` of `TrendFisher`

#### Scenario: Repositories with the same name
- **WHEN** `repos_root` is `["~/Repos", "~/Documents/GIT"]`, `~/Repos/api` and `~/Documents/GIT/api` are repositories, the second has the linked worktree `feat`, and the user runs `wt cd GIT/api:feat`
- **THEN** the destination is the worktree `feat` of `~/Documents/GIT/api`

#### Scenario: Path suffix ignores case
- **WHEN** `~/Documents/GIT/api` and `~/Repos/api` are repositories and the user runs `wt cd git/API:`
- **THEN** the destination is `~/Documents/GIT/api`

#### Scenario: Path suffix is not a prefix
- **WHEN** `~/Documents/GIT/api` is a repository, no repository path ends with the components `GIT` and `ap`, and the user runs `wt cd GIT/ap:`
- **THEN** stderr's first line is `wt: no repository named "GIT/ap"`
- **AND** the exit code is 3

#### Scenario: Backslash on Windows
- **WHEN** on Windows `C:\Repos\api` and `D:\GIT\api` are repositories and the user runs `wt cd GIT\api:`
- **THEN** the destination is `D:\GIT\api`

#### Scenario: Backslash on macOS or Linux
- **WHEN** on macOS or Linux `<root>/GIT/api` is a repository, no repository is named `GIT\api`, and the user runs `wt cd 'GIT\api:'`
- **THEN** stderr's first line is `wt: no repository named "GIT\api"`
- **AND** the exit code is 3

#### Scenario: Ambiguous repository side
- **WHEN** `HOME` is `/Users/me`, `repos_root` is `["~/Repos", "~/Documents/GIT"]`, `~/Repos/api` and `~/Documents/GIT/api` are repositories, and the user runs `wt cd api:feat`
- **THEN** stderr's first line is `wt: "api" matches more than one repository`
- **AND** stderr has a `hint:` line containing `Repos/api:` and `/Users/me/Repos/api`, and another containing `GIT/api:` and `/Users/me/Documents/GIT/api`
- **AND** the exit code is 4

#### Scenario: No such repository
- **WHEN** no repository under the roots starts with `nope` and the user runs `wt cd nope:feat`
- **THEN** stderr's first line is `wt: no repository named "nope"`
- **AND** a `hint:` line names `wt repos`
- **AND** the exit code is 3

#### Scenario: No roots configured
- **WHEN** no configuration sets `repos_root` and the user runs `wt cd api:feat`
- **THEN** stderr's first line is `wt: no repository roots configured`
- **AND** a `hint:` line names `repos_root` and the user configuration file
- **AND** the exit code is 1

### Requirement: Name side
For a command that acts on an existing worktree (`wt cd`, `wt path`, `wt remove`, `wt lock`, `wt unlock`), the name side SHALL be resolved among all the worktrees of the repository the repository side designates, including its main worktree:

- an empty name side SHALL designate the repository itself: for `wt cd` and `wt path`, its path as `wt repos` lists it; for a command that acts on a worktree, its main worktree;
- `^` SHALL designate its main worktree, and, for a bare repository, the bare repository itself;
- `@` and `-` SHALL be usage errors (exit 2) with a message naming the target, because they refer to the shell, not to a repository;
- any other name side SHALL be resolved as the `navigate` capability resolves a name in the current repository: the worktrees whose `NAME` equals it and, only if none does, the worktree whose branch equals it, exactly and case-sensitively, with the same exit 4 and hints when more than one matches.

When nothing matches, the command SHALL fail with exit code 3, the message `no worktree named "<name side>" in <display name>`, and a hint naming `wt list --all-repos`. When git fails while reading the worktrees of that repository, the command SHALL fail as the `git-worktrees` capability specifies for git failures (exit 1): a repository named by a qualified target is never skipped.

#### Scenario: Worktree of another repository
- **WHEN** `<root>/api` has the linked worktree `feat`, the shell is inside `<root>/web`, and the user runs `wt cd api:feat`
- **THEN** the destination is the worktree `feat` of `api`

#### Scenario: Same name as a local worktree
- **WHEN** the shell is inside `<root>/web`, `web` and `<root>/api` both have a linked worktree named `feat`, and the user runs `wt cd api:feat`
- **THEN** the destination is the worktree `feat` of `api`

#### Scenario: By branch
- **WHEN** `<root>/api` has the linked worktree `feature-abc1` with the branch `feature/abc1` checked out, and the user runs `wt cd api:feature/abc1`
- **THEN** the destination is that worktree

#### Scenario: Main worktree by its branch
- **WHEN** the main worktree of `<root>/api` has the branch `main` checked out and the user runs `wt cd api:main`
- **THEN** the destination is `<root>/api`

#### Scenario: Main worktree
- **WHEN** the user runs `wt cd api:^`
- **THEN** the destination is the main worktree of `<root>/api`

#### Scenario: Repository itself in the .bare layout
- **WHEN** `<root>/proj/.bare` is a bare repository, `<root>/proj/.git` is a file containing `gitdir: ./.bare`, and the user runs `wt cd proj:` and then `wt path proj:^`
- **THEN** the destination of `wt cd proj:` is `<root>/proj`
- **AND** `wt path proj:^` prints `<root>/proj/.bare`

#### Scenario: Current worktree cannot be qualified
- **WHEN** the user runs `wt cd api:@`
- **THEN** stderr's first line names `api:@`
- **AND** the exit code is 2

#### Scenario: No such worktree in the repository
- **WHEN** `<root>/api` has no worktree or branch named `nope` and the user runs `wt cd api:nope`
- **THEN** stderr's first line is `wt: no worktree named "nope" in api`
- **AND** a `hint:` line names `wt list --all-repos`
- **AND** the exit code is 3

#### Scenario: Repository that git cannot read
- **WHEN** git cannot read the worktrees of `<root>/broken` and the user runs `wt cd broken:feat`
- **THEN** stderr contains git's error message
- **AND** the exit code is 1

### Requirement: Display name of a repository
Wherever `wt` names a repository found under the roots for the user to type it back (in hints, in the `REPO` column of `wt list --all-repos`, in completions, and in messages), it SHALL use the repository's display name. The display name SHALL be the repository's name when no other repository found under the roots has exactly the same name. Otherwise it SHALL be the shortest suffix of its path, of at least two whole components joined with `/` on every operating system, that as a repository side (see "Repository side") matches only that repository. When no suffix matches only that repository, its display name SHALL be its path, which cannot be used as a repository side.

#### Scenario: Unique name
- **WHEN** `<root>/api` is the only repository named `api`
- **THEN** its display name is `api`

#### Scenario: Same name under two roots
- **WHEN** `repos_root` is `["~/Repos", "~/Documents/GIT"]` and `~/Repos/api` and `~/Documents/GIT/api` are repositories
- **THEN** their display names are `Repos/api` and `GIT/api`

#### Scenario: Organisation folders
- **WHEN** `repos_depth` is `2` and `<root>/work/api` and `<root>/oss/api` are repositories
- **THEN** their display names are `work/api` and `oss/api`

#### Scenario: Names that differ only in case
- **WHEN** `<one>/Api` and `<two>/api` are repositories
- **THEN** their display names are `Api` and `api`

#### Scenario: Forward slashes on Windows
- **WHEN** on Windows `C:\Repos\api` and `D:\GIT\api` are repositories
- **THEN** their display names are `Repos/api` and `GIT/api`

#### Scenario: No distinguishing suffix
- **WHEN** `repos_root` is `["/x", "/y"]`, `repos_depth` is `2`, and `/x/api` and `/y/x/api` are repositories
- **THEN** the display name of `/y/x/api` is `y/x/api`
- **AND** the display name of `/x/api` is `/x/api`

### Requirement: Repositories git cannot read
When a command reads the worktrees of the repositories under the roots without one of them being named by a qualified target (to search the worktrees of other repositories, for `wt list --all-repos`, or for a completion), and git fails for one of them for any reason other than git not being found, that repository SHALL be skipped: its worktrees SHALL count as absent, the command SHALL write the warning `cannot read the worktrees of <display name>: <git's message>`, and its exit code SHALL NOT change because of it. With `--json` the warning SHALL NOT be written, as the `cli-contract` capability specifies, and a completion SHALL never write it. When git is not found, the command SHALL fail as the `git-worktrees` capability specifies (exit 1). A command whose earlier steps resolve the target SHALL NOT read the worktrees of the other repositories.

#### Scenario: Found elsewhere
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` has the linked worktree `feat`, and from outside any repository the user runs `wt cd feat`
- **THEN** stderr contains a warning naming `broken`
- **AND** the destination is the worktree `feat` of `api`
- **AND** the exit code is 0

#### Scenario: Found nowhere
- **WHEN** git cannot read the worktrees of `<root>/broken` and from outside any repository the user runs `wt cd nope`
- **THEN** stderr contains a warning naming `broken`
- **AND** stderr has the line `wt: no worktree or repository named "nope"`
- **AND** the exit code is 3

#### Scenario: Not read when not needed
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` is a repository, and from outside any repository the user runs `wt cd api`
- **THEN** stderr is empty
- **AND** the destination is `<root>/api`

#### Scenario: Warning not written as JSON
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` has the linked worktree `feat`, and the user runs `wt path feat --json`
- **THEN** stderr is empty
- **AND** the exit code is 0

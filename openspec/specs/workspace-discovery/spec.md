# workspace-discovery Specification

## Purpose

Defines how `wt` finds the repositories that live under the roots the user configures, wherever the shell is, and `wt repos`, the command that lists them for people and as a versioned JSON document for scripts.

## Requirements

### Requirement: Repository roots
The roots SHALL be the paths in the `repos_root` configuration key (see the `config-layers` capability), in the order they are given. A root written as `~`, or starting with `~/`, SHALL have that `~` replaced by the user's home directory: the `HOME` environment variable on macOS and Linux, and `USERPROFILE` on Windows, where a root starting with `~\` SHALL be expanded the same way. A root that does not exist or is not a directory SHALL produce a warning naming the expanded path, SHALL be skipped, and SHALL NOT change the exit code. When the home directory is not set, a root that needs `~` expanded SHALL produce a warning naming the root and the variable (`HOME`, or `USERPROFILE` on Windows), SHALL be skipped, and SHALL NOT change the exit code. When `repos_root` is empty, there are no roots, and no repository is found.

#### Scenario: Home directory on macOS or Linux
- **WHEN** on macOS or Linux `HOME` is `/Users/me`, `repos_root` is `["~/GIT"]`, and `/Users/me/GIT/api` is a repository
- **THEN** `wt repos` lists `api` with path `/Users/me/GIT/api`

#### Scenario: Home directory on Windows
- **WHEN** on Windows `USERPROFILE` is `C:\Users\me`, `repos_root` is `["~\\GIT"]`, and `C:\Users\me\GIT\api` is a repository
- **THEN** `wt repos` lists `api` with path `C:\Users\me\GIT\api`

#### Scenario: Missing root
- **WHEN** `repos_root` is `["/Volumes/usb/repos", "<dir>"]`, the first does not exist, and `<dir>/api` is a repository
- **THEN** stderr contains a warning naming `/Volumes/usb/repos`
- **AND** `wt repos` lists `api`
- **AND** the exit code is 0

### Requirement: What counts as a repository
A directory found under a root SHALL be a repository when it contains an entry named `.git` and it is not a linked worktree. A linked worktree is a directory created by `git worktree add`: its `.git` is a file that points into the area where another repository keeps the records of its worktrees. A directory whose `.git` is a file pointing to a repository directory of its own, such as one cloned with `git clone --separate-git-dir` or a bare repository with a `.git` file beside it, SHALL be a repository. A directory with no `.git` entry, including a bare repository on its own, SHALL NOT be a repository. A root itself SHALL NOT be a repository.

#### Scenario: Clone
- **WHEN** `<root>/api` is a clone and `repos_root` is `["<root>"]`
- **THEN** `wt repos` lists `api`

#### Scenario: Worktrees beside the repository
- **WHEN** `repos_depth` is `2`, `<root>/api` is a repository, and `wt create feat` created its linked worktree at `<root>/api.worktrees/feat`
- **THEN** `wt repos` lists `api`
- **AND** does not list `feat`

#### Scenario: Bare repository with a .git file
- **WHEN** `<root>/proj/.bare` is a bare repository and `<root>/proj/.git` is a file containing `gitdir: ./.bare`
- **THEN** `wt repos` lists `proj` with path `<root>/proj`

#### Scenario: Bare repository on its own
- **WHEN** `<root>/api.git` is a bare repository with no `.git` entry inside it
- **THEN** `wt repos` does not list `api.git`

#### Scenario: Plain directory
- **WHEN** `<root>/notes` contains files but no `.git` entry
- **THEN** `wt repos` does not list `notes`

### Requirement: Search depth
The immediate subdirectories of a root SHALL be at depth 1, their subdirectories at depth 2, and so on. Directories SHALL be examined down to the depth given by `repos_depth`. A directory that contains a `.git` entry, whether it is a repository or a linked worktree, SHALL NOT be searched further. Directories whose name starts with `.` SHALL be treated like any other, on every operating system. A subdirectory that cannot be read SHALL be skipped without a warning.

#### Scenario: One level by default
- **WHEN** `repos_depth` is not set and `<root>/org/api` is a repository
- **THEN** `wt repos` does not list `api`

#### Scenario: Organisation folders
- **WHEN** `repos_depth` is `2` and `<root>/org/api` is a repository
- **THEN** `wt repos` lists `api` with path `<root>/org/api`

#### Scenario: Repository inside a repository
- **WHEN** `repos_depth` is `3`, `<root>/api` is a repository, and `<root>/api/vendor/lib` is another repository
- **THEN** `wt repos` lists `api`
- **AND** does not list `lib`

#### Scenario: Hidden directory
- **WHEN** `<root>/.dotfiles` is a repository
- **THEN** `wt repos` lists `.dotfiles`

#### Scenario: Unreadable directory (macOS, Linux)
- **WHEN** on macOS or Linux `repos_depth` is `2`, `<root>/locked` cannot be read, and `<root>/api` is a repository
- **THEN** `wt repos` lists `api`
- **AND** stderr is empty
- **AND** the exit code is 0

### Requirement: Repository name and path
A repository's path SHALL be the path through which it was found under its root, in the operating system's native absolute form, without resolving symbolic links. Its name SHALL be the last component of that path. Symbolic links to directories SHALL be followed like directories; on Windows, so SHALL directory junctions.

#### Scenario: Native path on Windows
- **WHEN** on Windows `repos_root` is `["C:/Repos"]` and `C:\Repos\api` is a repository
- **THEN** `wt repos` lists `api` with path `C:\Repos\api`

#### Scenario: Linked repository (macOS, Linux)
- **WHEN** on macOS or Linux `<root>/web` is a symbolic link to a repository at `/elsewhere/frontend`
- **THEN** `wt repos` lists `web` with path `<root>/web`

#### Scenario: Junction (Windows)
- **WHEN** on Windows `<root>\web` is a directory junction to a repository at `D:\elsewhere\frontend`
- **THEN** `wt repos` lists `web` with path `<root>\web`

### Requirement: Same repository reached twice
When the same directory is reached through more than one path, because two roots overlap, a root is given twice, or a link points to a repository already found, it SHALL be listed once. The path kept SHALL be the one found under the root that comes first in `repos_root` and, within that root, the one that sorts first when paths are compared component by component (so `<root>/a/b` sorts before `<root>/a-c`, and `<root>/links/web` before `<root>/web`). Two paths SHALL be the same directory when they lead to the same directory on disk, whatever links, junctions or letter case they go through, on every operating system. Repositories that are different directories SHALL all be listed, even when they share a name.

#### Scenario: Overlapping roots
- **WHEN** `repos_depth` is `2`, `repos_root` is `["<dir>/work", "<dir>"]`, and `<dir>/work/api` is a repository
- **THEN** `wt repos` lists `api` once, with path `<dir>/work/api`

#### Scenario: Link to a repository already found (macOS, Linux)
- **WHEN** on macOS or Linux `<root>/api` is a repository and `<root>/zz-api` is a symbolic link to it
- **THEN** `wt repos` lists `api` once, with path `<root>/api`, and does not list `zz-api`

#### Scenario: Junction to a repository already found (Windows)
- **WHEN** on Windows `<root>\api` is a repository and `<root>\zz-api` is a directory junction to it
- **THEN** `wt repos` lists `api` once, with path `<root>\api`, and does not list `zz-api`

#### Scenario: Same name, different repositories
- **WHEN** `repos_depth` is `2` and `<root>/work/api` and `<root>/oss/api` are two different repositories
- **THEN** `wt repos` lists both, each with its own path

### Requirement: Listing repositories
`wt repos` SHALL list every repository found under the roots and SHALL exit 0. It SHALL accept no positional arguments. Without `--json`, it SHALL print a header row followed by one row per repository, with these columns in this order:

| Column | Content |
|---|---|
| (markers) | `@` if the working directory belongs to the repository; blank otherwise |
| `NAME` | the repository's name |
| `PATH` | the repository's path |

Columns SHALL be aligned by their visible width, whether or not color is enabled. Rows SHALL be ordered by `NAME`, ignoring letter case, with ties broken by path. The working directory SHALL belong to a repository when it is the one git associates with the working directory (see the `git-worktrees` capability), whether the working directory is inside its main worktree, inside one of its linked worktrees, or inside its repository directory; when the working directory is in no repository, or in one that was not found under the roots, no row SHALL be marked. When roots are configured but no repository is found, stdout SHALL be empty and the exit code SHALL be 0.

#### Scenario: Order ignores case
- **WHEN** the repositories found are `zeta`, `Alpha` and `beta`
- **THEN** the rows of `wt repos` are `Alpha`, `beta`, `zeta`, in that order

#### Scenario: Marker from a linked worktree
- **WHEN** `<root>/api` is a repository, its linked worktree lives at `/tmp/x/feat`, and the user runs `wt repos` from `/tmp/x/feat/src`
- **THEN** the row of `api` starts with `@`

#### Scenario: Outside any repository
- **WHEN** the user runs `wt repos` from a directory that is in no repository
- **THEN** no row starts with `@`
- **AND** the exit code is 0

#### Scenario: Working directory override
- **WHEN** the user runs `wt -C <root>/web repos` from inside `<root>/api`
- **THEN** the row of `web` starts with `@`
- **AND** the row of `api` does not

#### Scenario: No repositories found
- **WHEN** `repos_root` names an existing empty directory and the user runs `wt repos`
- **THEN** stdout is empty
- **AND** the exit code is 0

#### Scenario: Extra argument
- **WHEN** the user runs `wt repos api`
- **THEN** stderr reports the unexpected argument `api`
- **AND** the exit code is 2

### Requirement: No roots configured
When `repos_root` is empty, `wt repos` SHALL fail with exit code 1, the message `no repository roots configured`, and a hint naming the `repos_root` key and the path of the user configuration file (see the `config-layers` capability).

#### Scenario: Nothing configured
- **WHEN** no configuration sets `repos_root` and the user runs `wt repos`
- **THEN** stderr's first line is `wt: no repository roots configured`
- **AND** a `hint:` line names `repos_root` and the user configuration file
- **AND** stdout is empty
- **AND** the exit code is 1

#### Scenario: Nothing configured, as JSON
- **WHEN** no configuration sets `repos_root` and the user runs `wt repos --json`
- **THEN** stdout is empty
- **AND** stderr parses as a JSON object with `schema` `wt.error.v1` and `code` 1

### Requirement: JSON output
`wt repos --json` SHALL print a document with `schema` `wt.repos.v1` and a `repos` array in the same order as the table, empty when no repository is found. Each element SHALL have the fields `name` and `path` (strings, as in the table), `root` (the root it was found under, expanded and in native absolute form) and `current` (a boolean, true for the row the table marks with `@`). Warnings, such as a missing root, SHALL NOT be written with `--json`.

#### Scenario: Repositories as JSON (macOS, Linux)
- **WHEN** on macOS or Linux `repos_root` is `["~/GIT"]` with `HOME` set to `/Users/me`, `/Users/me/GIT/api` is a repository, and the user runs `wt repos --json` from inside it
- **THEN** stdout is a JSON object with `schema` `wt.repos.v1`
- **AND** its `repos` array has an element with `name` `api`, `path` `/Users/me/GIT/api`, `root` `/Users/me/GIT` and `current` true

#### Scenario: Repositories as JSON (Windows)
- **WHEN** on Windows `repos_root` is `["C:\\Repos"]`, `C:\Repos\api` is a repository, and the user runs `wt repos --json`
- **THEN** the element of `api` has `path` `C:\Repos\api` and `root` `C:\Repos`

#### Scenario: Missing root as JSON
- **WHEN** one of the roots does not exist and the user runs `wt repos --json`
- **THEN** stderr is empty
- **AND** the exit code is 0

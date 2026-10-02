# Spec Delta

## Purpose

Defines the template language `wt` uses to build paths from configuration, such as `worktree_path`: its syntax, the variables and filters it offers, how `sanitize` turns a branch or a name into a valid directory name on every operating system, and how a rendered template becomes an absolute, native path.

## ADDED Requirements

### Requirement: Template syntax
A template SHALL be literal text with zero or more expressions, each written `{<variable>}` or `{<variable>|<filter>|<filter>...}`. Whitespace around the variable and filter names inside the braces SHALL be ignored. An expression SHALL be replaced by the variable's value with the filters applied from left to right; literal text SHALL be copied unchanged. There is no way to write a literal `{` or `}`.

A template SHALL be invalid when it contains a `{` with no closing `}`, a `}` with no opening `{`, an expression with no variable name, an empty filter name, a variable that is not available where the template is used, or a filter that does not exist. A configuration key holding an invalid template has an invalid value (see the `config-layers` capability): every command that reads the configuration SHALL fail with exit code 1 and a message naming the key, the file or environment variable that set it, and the offending part of the template.

#### Scenario: Filters applied in order
- **WHEN** `worktree_path` is `{repo_parent}/{branch|sanitize|lower}` and the branch is `Feature/ABC`
- **THEN** the last component of the path is `feature-abc`

#### Scenario: Whitespace inside an expression
- **WHEN** `worktree_path` is `{repo_parent}/{ name | sanitize }`
- **THEN** it renders exactly as `{repo_parent}/{name|sanitize}`

#### Scenario: Unknown variable
- **WHEN** the user file sets `worktree_path = "{repo_parent}/{nope}"` and the user runs `wt list`
- **THEN** stderr names `worktree_path`, the user file and `nope`
- **AND** the exit code is 1

#### Scenario: Unknown filter
- **WHEN** `.wt.toml` sets `worktree_path = "{repo_parent}/{name|upper}"`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `worktree_path`, `.wt.toml` and `upper`

#### Scenario: Unclosed brace
- **WHEN** `WT_WORKTREE_PATH` is `{repo_parent/x`
- **THEN** every command that reads the configuration fails with exit code 1
- **AND** stderr names `WT_WORKTREE_PATH`

### Requirement: Variables of worktree_path
A template in `worktree_path` SHALL be able to use exactly these variables:

| Variable | Value |
|---|---|
| `{repo}` | the last component of the main worktree's path; for a bare repository, without a trailing `.git` |
| `{repo_parent}` | the directory that contains the main worktree |
| `{repo_path}` | the main worktree's path |
| `{name}` | the `<name>` given to `wt create`, as typed |
| `{branch}` | the branch the new worktree checks out |

The main worktree SHALL be the one the `git-worktrees` capability defines, whichever worktree of the repository the command runs from. Path values SHALL be absolute and in the operating system's native form.

#### Scenario: Run from a linked worktree
- **WHEN** the main worktree is `/src/repo`, the user runs `wt create x` from inside the linked worktree `/src/repo.worktrees/feat`, and `worktree_path` has its default value
- **THEN** the new worktree's path is `/src/repo.worktrees/x`

#### Scenario: Bare repository
- **WHEN** the main worktree is the bare repository `/src/proj.git` and the user runs `wt create feat` with the default `worktree_path`
- **THEN** the new worktree's path is `/src/proj.worktrees/feat`

### Requirement: sanitize filter
`sanitize` SHALL turn any text into a name usable as one directory name on macOS, Windows and Linux, and SHALL produce the same result on every operating system. It SHALL apply these rules in order:

1. each `/`, `\`, `<`, `>`, `:`, `"`, `|`, `?` and `*`, and each control character (U+0000 to U+001F, and U+007F), is replaced by `-`;
2. each `.` and each space at the end of the text is replaced by `-`;
3. when the text, or its part before the first `.`, is a reserved device name of Windows (`CON`, `PRN`, `AUX`, `NUL`, `COM1` to `COM9`, `LPT1` to `LPT9`), compared ignoring case, a `-` is inserted right after that part.

Every other character, including letters outside ASCII and spaces that are not at the end, SHALL be kept.

| Input | Output |
|---|---|
| `feature/abc1` | `feature-abc1` |
| `fix\x` | `fix-x` |
| `a:b*c` | `a-b-c` |
| `feat<x>` | `feat-x-` |
| `v1.` | `v1-` |
| `..` | `--` |
| `nul` | `nul-` |
| `Con.txt` | `Con-.txt` |
| `my task` | `my task` |
| `café` | `café` |

#### Scenario: Branch with a slash
- **WHEN** with the default `worktree_path` the user runs `wt create feature/abc1` in a repository whose main worktree is `/src/repo`
- **THEN** the new worktree's path is `/src/repo.worktrees/feature-abc1`

#### Scenario: Reserved name on every OS
- **WHEN** with the default `worktree_path` the user runs `wt create nul -b reserved`, on macOS, Windows or Linux
- **THEN** the last component of the new worktree's path is `nul-`

### Requirement: lower filter
`lower` SHALL convert every letter of the text to lower case, following Unicode case mapping, and SHALL keep every other character.

#### Scenario: Lower-case directory
- **WHEN** `worktree_path` is `{repo_parent}/{name|lower}` and the user runs `wt create Feat`
- **THEN** the last component of the new worktree's path is `feat`

### Requirement: From template to path
The rendered template SHALL become a path as follows:

1. On Windows, both `/` and `\` separate components; on macOS and Linux, only `/` does.
2. A `~` at the start, alone or followed by a separator, SHALL be replaced by the user's home directory: `HOME` on macOS and Linux, `USERPROFILE` on Windows.
3. A relative path SHALL be resolved against `{repo_path}`.
4. `.` and `..` components SHALL be resolved without reading the filesystem, and repeated separators SHALL count as one.
5. The result SHALL be reported in the operating system's native form.

#### Scenario: Default template (macOS, Linux)
- **WHEN** on macOS or Linux the main worktree is `/src/repo` and the user runs `wt create feat` with the default `worktree_path`
- **THEN** the new worktree's path is `/src/repo.worktrees/feat`

#### Scenario: Default template (Windows)
- **WHEN** on Windows the main worktree is `C:\src\repo` and the user runs `wt create feat` with the default `worktree_path`
- **THEN** the new worktree's path is `C:\src\repo.worktrees\feat`

#### Scenario: Home directory
- **WHEN** `worktree_path` is `~/wt/{repo}/{name|sanitize}` and the user runs `wt create feat` in the repository `repo`
- **THEN** the new worktree's path is `<home>/wt/repo/feat`, in native form

#### Scenario: Relative to the repository
- **WHEN** `worktree_path` is `.worktrees/{name|sanitize}`, the main worktree is `/src/repo`, and the user runs `wt create feat`
- **THEN** the new worktree's path is `/src/repo/.worktrees/feat`

#### Scenario: Parent components
- **WHEN** `worktree_path` is `{repo_path}/../{repo}-{name|sanitize}`, the main worktree is `/src/repo`, and the user runs `wt create feat`
- **THEN** the new worktree's path is `/src/repo-feat`

### Requirement: Valid path on Windows
On Windows, when a component of the resulting path, other than the drive or the share of a UNC path, is a reserved device name (as listed for `sanitize`, with or without an extension), contains `<`, `>`, `:`, `"`, `|`, `?`, `*` or a control character, or ends with `.` or a space, the command SHALL fail with exit code 2, a message naming the path, and a hint to apply the `sanitize` filter. Nothing SHALL be created. On macOS and Linux this check SHALL NOT apply.

#### Scenario: Reserved name without sanitize (Windows)
- **WHEN** on Windows `worktree_path` is `{repo_parent}/{name}` and the user runs `wt create nul`
- **THEN** stderr names the path and mentions `sanitize`
- **AND** no branch, directory or worktree is created
- **AND** the exit code is 2

#### Scenario: Same template on macOS
- **WHEN** on macOS `worktree_path` is `{repo_parent}/{name}`, the main worktree is `/src/repo`, and the user runs `wt create nul`
- **THEN** the new worktree's path is `/src/nul`
- **AND** the exit code is 0

## MODIFIED Requirements

### Requirement: Template syntax
A template SHALL be literal text with zero or more expressions, each written `{<variable>}` or `{<variable>|<filter>|<filter>...}`. Whitespace around the variable and filter names inside the braces SHALL be ignored. An expression SHALL be replaced by the variable's value with the filters applied from left to right; literal text SHALL be copied unchanged. `{{` SHALL be a literal `{` and `}}` a literal `}`. A template SHALL be read from left to right, so that in `{{{name}}}` the first two braces are a literal `{`, `{name}` is an expression, and the last two braces are a literal `}`.

A template SHALL be invalid when it contains a `{` with no closing `}`, a `}` with no opening `{`, an expression with no variable name, an empty filter name, a variable that is not available where the template is used, or a filter that does not exist. A configuration key holding an invalid template has an invalid value (see the `config-layers` capability): every command that reads the configuration SHALL fail with exit code 1 and a message naming the key, the file or environment variable that set it, and the offending part of the template.

#### Scenario: Filters applied in order
- **WHEN** `worktree_path` is `{repo_parent}/{branch|sanitize|lower}` and the branch is `Feature/ABC`
- **THEN** the last component of the path is `feature-abc`

#### Scenario: Whitespace inside an expression
- **WHEN** `worktree_path` is `{repo_parent}/{ name | sanitize }`
- **THEN** it renders exactly as `{repo_parent}/{name|sanitize}`

#### Scenario: Literal braces
- **WHEN** `worktree_path` is `{repo_parent}/{{x}}-{name}` and the user runs `wt create feat` in a repository whose main worktree is `/src/repo`
- **THEN** the new worktree's path is `/src/{x}-feat`

#### Scenario: Literal brace around an expression
- **WHEN** `worktree_path` is `{repo_parent}/{{{name}}}` and the user runs `wt create feat` in a repository whose main worktree is `/src/repo`
- **THEN** the new worktree's path is `/src/{feat}`

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

#### Scenario: Lone closing brace after a literal one
- **WHEN** `worktree_path` is `{repo_parent}/x}}}`
- **THEN** `wt config list` fails with exit code 1
- **AND** stderr names `worktree_path`

## MODIFIED Requirements

### Requirement: Running the binary through the function
Running `wt <args>` SHALL run the `git-wt` executable found on the PATH with the same arguments, unchanged and in the same order, including arguments that contain spaces and empty arguments. The binary SHALL run with the shell's own standard input, output and error, which the function SHALL NOT capture or redirect. The binary's exit code SHALL be the function's result: its return status in zsh, bash and fish, and `$LASTEXITCODE` in PowerShell.

PowerShell is the one exception to "the same arguments": it removes an argument `--` written without quotes before the function receives its arguments, so that argument SHALL NOT reach the binary. `'--'`, in quotes, SHALL reach it. No command of `wt` SHALL need a `--` to work (see the `batch-execution` capability).

#### Scenario: Output is not captured
- **WHEN** the user runs `wt list` through the function inside a repository
- **THEN** stdout is identical to the output of `git-wt list`

#### Scenario: Argument with spaces
- **WHEN** the user runs `wt config get "a b"` through the function
- **THEN** stderr names the key `a b` as unknown
- **AND** the exit status is 2

#### Scenario: Exit code (zsh, bash, fish)
- **WHEN** in zsh, bash or fish the user runs `wt lsit`
- **THEN** the function's return status is 2

#### Scenario: Exit code (PowerShell)
- **WHEN** in PowerShell the user runs `wt lsit`
- **THEN** `$LASTEXITCODE` is 2 after the function returns

#### Scenario: Double dash (zsh, bash, fish)
- **WHEN** in zsh, bash or fish the user runs `wt config get -- --x` through the function
- **THEN** stderr names the key `--x` as unknown
- **AND** the exit status is 2

#### Scenario: Double dash (PowerShell)
- **WHEN** in PowerShell the user runs `wt config get -- --x` through the function
- **THEN** stderr names `--x` as an unknown flag
- **AND** `$LASTEXITCODE` is 2

#### Scenario: Quoted double dash (PowerShell)
- **WHEN** in PowerShell the user runs `wt config get '--' --x` through the function
- **THEN** stderr names the key `--x` as unknown
- **AND** `$LASTEXITCODE` is 2

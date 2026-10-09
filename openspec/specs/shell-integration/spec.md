# shell-integration Specification

## Purpose

Defines how a `wt` binary, which cannot change the working directory of the shell that started it, moves that shell anyway: the `wt` shell function that `wt shell init` prints for each supported shell, the directive file through which the binary asks it to change directory, and the completions it registers.

## Requirements

### Requirement: Supported shells
`wt shell init` SHALL support the shells `zsh`, `bash`, `fish` and `pwsh` (PowerShell 7). The requirements of this capability SHALL hold for zsh 5.8 or later, bash 3.2 or later, fish 3.3 or later and PowerShell 7.4 or later on macOS and Linux, and for PowerShell 7.4 or later on Windows. On Windows, `wt shell init` SHALL still print the script for `zsh`, `bash` and `fish`, but no other requirement of this capability applies to those shells there.

#### Scenario: PowerShell on Windows
- **WHEN** on Windows the user loads the output of `wt shell init pwsh` in PowerShell 7
- **THEN** every requirement of this capability holds in that session

#### Scenario: bash on Windows
- **WHEN** on Windows the user runs `wt shell init bash`
- **THEN** the script is printed to stdout
- **AND** the exit code is 0

### Requirement: Printing the shell script
`wt shell init <shell>` SHALL print to stdout the script for `<shell>` and exit 0. It SHALL work outside any git repository and SHALL NOT read the configuration, so it SHALL succeed even when the configuration is invalid. A missing `<shell>`, a `<shell>` that is not one of the supported names, or an extra argument SHALL be a usage error (exit 2); for an unsupported name, the diagnostic SHALL list the supported ones. The command never writes, so `--dry-run` SHALL NOT change its output.

#### Scenario: Script for zsh
- **WHEN** the user runs `wt shell init zsh` outside any git repository
- **THEN** stdout contains a zsh script that defines the function `wt`
- **AND** the exit code is 0

#### Scenario: Broken configuration
- **WHEN** the user configuration file contains invalid TOML and the user runs `wt shell init bash`
- **THEN** the script is printed
- **AND** the exit code is 0

#### Scenario: Unsupported shell
- **WHEN** the user runs `wt shell init tcsh`
- **THEN** stderr names `tcsh` and lists `zsh`, `bash`, `fish` and `pwsh`
- **AND** stdout is empty
- **AND** the exit code is 2

#### Scenario: Missing shell
- **WHEN** the user runs `wt shell init`
- **THEN** stderr reports the missing `<shell>` argument
- **AND** the exit code is 2

### Requirement: Installation line
The help of `wt shell init` SHALL show, for each supported shell, the line that loads the script and the file it goes in:

| Shell | File | Line |
|---|---|---|
| zsh | `~/.zshrc` | `eval "$(git-wt shell init zsh)"` |
| bash | `~/.bashrc` | `eval "$(git-wt shell init bash)"` |
| fish | `~/.config/fish/config.fish` | `git-wt shell init fish \| source` |
| pwsh | `$PROFILE` | `Invoke-Expression (& git-wt shell init pwsh \| Out-String)` |

Loading the script with that line SHALL define the function `wt` and SHALL write nothing to stdout or stderr. Loading it more than once in the same session SHALL behave as loading it once.

#### Scenario: Help shows the lines
- **WHEN** the user runs `wt shell init --help`
- **THEN** stdout contains the line and the file for zsh, bash, fish and pwsh

#### Scenario: Silent load
- **WHEN** the user loads the script in any supported shell with its installation line
- **THEN** nothing is written to stdout or stderr
- **AND** `wt` is defined as a shell function

#### Scenario: Loaded twice
- **WHEN** the script is loaded twice in the same session and the user runs `wt cd feat`
- **THEN** the shell changes directory once, to the worktree `feat`

### Requirement: Machine-readable script
`wt shell init <shell> --json` SHALL print `{"schema":"wt.shell.init.v1","shell":<shell>,"script":<script>}`, where `script` is exactly what the command prints without `--json`.

#### Scenario: Script as JSON
- **WHEN** the user runs `wt shell init fish --json`
- **THEN** stdout parses as a JSON object with `schema` `wt.shell.init.v1` and `shell` `fish`
- **AND** its `script` equals the output of `wt shell init fish`

### Requirement: Running the binary through the function
Running `wt <args>` SHALL run the `git-wt` executable found on the PATH with the same arguments, unchanged and in the same order, including arguments that contain spaces and empty arguments. The binary SHALL run with the shell's own standard input, output and error, which the function SHALL NOT capture or redirect. The binary's exit code SHALL be the function's result: its return status in zsh, bash and fish, and `$LASTEXITCODE` in PowerShell.

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

### Requirement: Directive file
Before each run of the binary, the function SHALL create a new, empty file in the system's temporary directory and pass its absolute path to that run, and only to that run, in the environment variable `WT_DIRECTIVE_CD_FILE`. When the binary exits, whatever its exit code, if the file is not empty the function SHALL change the shell's working directory to the path the file contains. The function SHALL then delete the file. After the function returns, `WT_DIRECTIVE_CD_FILE` SHALL NOT be set in the shell. If changing directory fails, the shell SHALL report the failure and the function's result SHALL be non-zero.

#### Scenario: The binary moves the shell
- **WHEN** the binary, run through the function, writes the path of an existing directory to the directive file
- **THEN** after `wt` returns, the shell's working directory is that path

#### Scenario: Commands that do not move the shell
- **WHEN** the user runs `wt list` through the function
- **THEN** the shell's working directory does not change

#### Scenario: No file left behind
- **WHEN** the user runs `wt list` and then `wt cd feat` through the function
- **THEN** no file created by the function remains in the temporary directory

#### Scenario: Variable not left set
- **WHEN** the user runs `wt cd feat` through the function
- **THEN** `WT_DIRECTIVE_CD_FILE` is not set in the shell afterwards

### Requirement: Writing the directive
The shell integration SHALL be active for a run of the binary when `WT_DIRECTIVE_CD_FILE` is set and not empty. A command that moves the shell SHALL replace the contents of that file with the destination's absolute path in the operating system's native form, and nothing else. The binary SHALL NOT create that file: if it does not exist, the command SHALL fail with exit code 1 and leave no file at that path. Under `--dry-run`, the binary SHALL NOT write the file.

#### Scenario: Directive contents (macOS, Linux)
- **WHEN** on macOS or Linux `wt cd feat` writes the directive for a worktree at `/src/repo.worktrees/feat`
- **THEN** the file contains exactly `/src/repo.worktrees/feat`

#### Scenario: Directive contents (Windows)
- **WHEN** on Windows `wt cd feat` writes the directive for a worktree at `C:\src\repo.worktrees\feat`
- **THEN** the file contains exactly `C:\src\repo.worktrees\feat`

#### Scenario: Missing directive file
- **WHEN** `WT_DIRECTIVE_CD_FILE` names a file that does not exist and the user runs `git-wt cd feat`
- **THEN** the exit code is 1
- **AND** no file exists at that path afterwards

### Requirement: Previous directory
Whenever the function changes the shell's working directory, it SHALL remember the directory the shell was in just before, and SHALL pass it to every later run of the binary in that shell session in the environment variable `WT_PREVIOUS_DIR`. A shell session in which the function has not changed directory SHALL have no previous directory, even when it was started from a session that had one.

#### Scenario: Going back and forth
- **WHEN** the shell is in `/a`, the user runs `wt cd feat`, and then `wt cd -`
- **THEN** the shell is back in `/a`
- **AND** running `wt cd -` again moves it to the worktree `feat`

#### Scenario: New session
- **WHEN** a shell session where `wt cd feat` was run starts a new interactive shell that loads the script, and the user runs `wt cd -` in it
- **THEN** the command fails because there is no previous directory

### Requirement: Processes started by wt
Processes that `wt` starts, such as git, SHALL NOT receive `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR` in their environment, so that a program they run cannot write to the shell's directive file.

#### Scenario: Git does not see the protocol
- **WHEN** the `git` on the PATH records its environment and the user runs `wt list` through the function
- **THEN** the recorded environment contains neither `WT_DIRECTIVE_CD_FILE` nor `WT_PREVIOUS_DIR`

### Requirement: Shadowing other programs named wt
Once the script is loaded, `wt` SHALL run the function even when an executable named `wt` is on the PATH.

#### Scenario: Windows Terminal installed (Windows)
- **WHEN** on Windows `wt.exe` from Windows Terminal is on the PATH and the user runs `wt list` in a PowerShell session that loaded the script
- **THEN** `git-wt list` runs, not `wt.exe`

#### Scenario: Another wt on the PATH (macOS, Linux)
- **WHEN** on macOS or Linux an executable named `wt` is on the PATH and the user runs `wt list` in a session that loaded the script
- **THEN** `git-wt list` runs, not that executable

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

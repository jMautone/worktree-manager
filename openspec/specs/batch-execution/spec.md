# batch-execution Specification

## Purpose

Defines `wt each`, the command that runs one command in every worktree of the current repository, or with `--all-repos` of every repository under the configured roots: which worktrees it visits and in what order, how the command's words become a command line on each operating system, how each run is shown, and what happens when a command fails or the user interrupts the batch.

## Requirements

### Requirement: Worktrees visited
`wt each <cmd>...` SHALL run the command (see "Command line") once in each worktree that `wt list` lists for the repository associated with the working directory, including the main worktree, in the order `wt list` lists them. With `--all-repos`, it SHALL run it, from any working directory, once in each worktree that `wt list --all-repos` lists, in that order; a repository that git cannot read SHALL be skipped as the `cross-repo-resolution` capability specifies, with its warning written before any command runs.

Outside a repository and without `--all-repos`, `wt each` SHALL fail as `wt list` does (exit 3, `not a git repository: <dir>`), with a hint naming `wt each --all-repos` when `repos_root` names at least one root. With `--all-repos` and no root in `repos_root`, it SHALL fail as `wt repos` does (exit 1, `no repository roots configured`). When git is not found, or git fails for the repository associated with the working directory, it SHALL fail as the `git-worktrees` capability specifies (exit 1). In every one of these cases no command SHALL run. When there is no worktree to run the command in, `wt each` SHALL write nothing to stdout and SHALL exit 0.

#### Scenario: Every worktree of the current repository
- **WHEN** a repository has its main worktree and the linked worktrees `feat` and `fix`, and from inside `feat` the user runs `wt each git branch --show-current`
- **THEN** the command runs three times: in the main worktree, in `feat` and in `fix`, in that order
- **AND** the exit code is 0

#### Scenario: Every repository
- **WHEN** `repos_root` is `["<root>"]`, `<root>/api` has the linked worktree `feat`, `<root>/web` has no linked worktree, and from outside any repository the user runs `wt each --all-repos git status --short`
- **THEN** the command runs in the main worktree of `api`, in `feat`, and in the main worktree of `web`, in that order
- **AND** the exit code is 0

#### Scenario: Outside a repository
- **WHEN** `repos_root` names at least one root and the user runs `wt each git status` outside any repository
- **THEN** stderr's first line is `wt: not a git repository: <dir>`
- **AND** a `hint:` line names `wt each --all-repos`
- **AND** stdout is empty
- **AND** the exit code is 3

#### Scenario: No roots configured
- **WHEN** no configuration sets `repos_root` and the user runs `wt each --all-repos git status`
- **THEN** stderr's first line is `wt: no repository roots configured`
- **AND** the exit code is 1

#### Scenario: Repository that git cannot read
- **WHEN** git cannot read the worktrees of `<root>/broken`, `<root>/api` is a repository, and the user runs `wt each --all-repos git status`
- **THEN** stderr contains a warning naming `broken`, written before the first header
- **AND** the command runs in the main worktree of `api`
- **AND** the exit code is 0

#### Scenario: Working directory override
- **WHEN** from outside any repository the user runs `wt -C /path/to/repo each git status`
- **THEN** the command runs in the worktrees of `/path/to/repo`

### Requirement: Worktrees skipped
A bare repository listed among the worktrees SHALL be skipped without a message, because it has no working tree. A worktree whose directory does not exist, whether git reports it prunable or it is locked, SHALL be skipped with the warning `skipping <label>: <path> does not exist`, where `<label>` is its label (see "Headers") and `<path>` its path; these warnings SHALL be written before any command runs. A skipped worktree SHALL NOT count as a command in the messages of "Failures" and "Interrupt", and skipping SHALL NOT change the exit code.

#### Scenario: Bare repository
- **WHEN** `<root>/proj/.bare` is a bare repository with the linked worktree `<root>/proj/feat`, and from inside `feat` the user runs `wt each git status`
- **THEN** the command runs only in `feat`
- **AND** stderr is empty
- **AND** the exit code is 0

#### Scenario: Directory gone
- **WHEN** the directory of the linked worktree `old` was deleted without `git worktree remove`, the repository also has the linked worktree `feat`, and the user runs `wt each git status`
- **THEN** stderr contains `wt: warning: skipping old: <path of old> does not exist`, written before the first header
- **AND** the command runs in the main worktree and in `feat`
- **AND** the exit code is 0

### Requirement: Start of the command
`wt` SHALL recognize its own flags, those of `wt each` and the global ones, only before the command's first word: the first argument after `each` that is neither a flag of `wt` nor the value of one. That argument and every argument after it SHALL be the words of the command, unchanged, even when they look like flags of `wt`. An argument `--` before the first word SHALL end `wt`'s flags and SHALL NOT be a word of the command; it is optional.

A command with no words, or whose only word is empty or contains only whitespace, SHALL be a usage error (exit 2), and so SHALL an unknown flag before the first word. Usage errors SHALL be reported before any repository is read, and no command SHALL run.

#### Scenario: Flags of the command
- **WHEN** the user runs `wt each git log --oneline -1` inside a repository
- **THEN** `--oneline` and `-1` reach `git`
- **AND** the exit code is 0

#### Scenario: A flag of wt after the first word
- **WHEN** the user runs `wt each <a program that prints each of its arguments on its own line> --dry-run` inside a repository
- **THEN** stdout contains a line `--dry-run` for each worktree
- **AND** the program ran in every worktree

#### Scenario: Double dash
- **WHEN** the user runs `wt each -- git log --oneline -1` inside a repository
- **THEN** the command run in each worktree is `git log --oneline -1`

#### Scenario: Through the function in PowerShell
- **WHEN** in PowerShell, through the function, the user runs `wt each -- git rev-parse --show-toplevel` inside a repository with the linked worktree `feat`
- **THEN** stdout contains the path of the main worktree and the path of `feat`
- **AND** `$LASTEXITCODE` is 0

#### Scenario: No command
- **WHEN** the user runs `wt each` inside a repository
- **THEN** stderr reports the missing command
- **AND** the exit code is 2

#### Scenario: Empty command
- **WHEN** the user runs `wt each ' '` inside a repository
- **THEN** stderr reports the empty command
- **AND** no command runs
- **AND** the exit code is 2

#### Scenario: Unknown flag before the command
- **WHEN** the user runs `wt each --bogus git status`
- **THEN** stderr names `--bogus` as an unknown flag
- **AND** no command runs
- **AND** the exit code is 2

### Requirement: Command line
The command line SHALL be built from the command's words. A single word SHALL be the command line, unchanged, so that the interpreter reads it as written, operators included. Several words SHALL be joined with single spaces, each written so that it reaches the program as one argument:

- on macOS and Linux, a word made only of letters, digits and the characters `@%+=:,./_-` SHALL be written as is, and any other word, including an empty one, between single quotes, with each `'` in it written as `'\''`; every such word SHALL reach the program unchanged;
- on Windows, a word SHALL be written as the Microsoft C runtime reads one argument from a command line, between double quotes when it is empty or contains a space, a tab, a double quote or any of `&|<>^()`; it SHALL reach a program that splits its command line that way as one argument, unchanged, except that `cmd.exe` SHALL replace each reference `%NAME%` to a defined environment variable with its value, in any word, quoted or not. A command built into `cmd.exe`, such as `echo`, SHALL receive the words as written in the command line, quotes included.

#### Scenario: Interpreter operators in a single word (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt each 'echo a && echo b'` in a repository with no linked worktree
- **THEN** stdout contains `a` and `b`, each on its own line

#### Scenario: Interpreter operators in a single word (Windows)
- **WHEN** on Windows the user runs `wt each "echo a& echo b"` in a repository with no linked worktree
- **THEN** stdout contains `a` and `b`, each on its own line

#### Scenario: Words kept whole (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt each <a program that prints each of its arguments on its own line> 'a b' 'a;b' '$HOME' "it's" ''` in a repository with no linked worktree
- **THEN** the program prints the lines `a b`, `a;b`, `$HOME`, `it's` and an empty line, in that order

#### Scenario: Words kept whole (Windows)
- **WHEN** on Windows the user runs `wt each <a program that prints each of its arguments on its own line> 'a b' 'a&b' 'say "hi"' 'C:\dir\' ''` in a repository with no linked worktree
- **THEN** the program prints the lines `a b`, `a&b`, `say "hi"`, `C:\dir\` and an empty line, in that order

#### Scenario: Environment variable reference (Windows)
- **WHEN** on Windows the environment variable `WT_TEST` is `x`, and the user runs `wt each <a program that prints each of its arguments on its own line> 'a %WT_TEST% b'` in a repository with no linked worktree
- **THEN** the program prints the line `a x b`

#### Scenario: Line shown by a dry run (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt each --dry-run git commit -m "fix bug"` in a repository with no linked worktree at `/src/repo`
- **THEN** stdout is `would run git commit -m 'fix bug' in /src/repo`

#### Scenario: Line shown by a dry run (Windows)
- **WHEN** on Windows the user runs `wt each --dry-run git commit -m "fix bug"` in a repository with no linked worktree at `C:\src\repo`
- **THEN** stdout is `would run git commit -m "fix bug" in C:\src\repo`

### Requirement: Running the commands
`wt each` SHALL run the command line once per worktree, one at a time, with the worktree's root as its working directory, even when `wt` was started in a subdirectory: as `sh -c <line>` on macOS and Linux, and as a command line of `cmd.exe` on Windows. Each command SHALL use `wt`'s own standard input, output and error, and `wt` SHALL wait for it to exit before starting the next one. The commands SHALL NOT receive `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR` (see the `shell-integration` capability). `wt each` SHALL never change the shell's working directory. A command that cannot be started SHALL count as failed (see "Failures"), and `wt each` SHALL go on with the next worktree.

#### Scenario: Working directory of each command
- **WHEN** a repository at `/src/repo` has the linked worktree `/src/repo.worktrees/feat`, and from `/src/repo/docs` the user runs `wt each pwd` on macOS or Linux
- **THEN** stdout contains `/src/repo` and `/src/repo.worktrees/feat` as output of the command

#### Scenario: Shell stays where it is
- **WHEN** through the function, from inside `feat`, the user runs `wt each git status`
- **THEN** the shell's working directory is still `feat` afterwards

#### Scenario: Protocol variables not passed
- **WHEN** through the function the user runs `wt each <a command that records its environment>`
- **THEN** no recorded environment contains `WT_DIRECTIVE_CD_FILE` or `WT_PREVIOUS_DIR`

### Requirement: Headers
Before running the command in a worktree, `wt each` SHALL write to stdout a line made of `==> `, the worktree's label, two spaces, and the worktree's path in the operating system's native form. The label SHALL be the worktree as `wt cd` takes it: without `--all-repos`, `^` for the main worktree and the worktree's `NAME` for any other; with `--all-repos`, the display name of its repository (see the `cross-repo-resolution` capability), `:`, and `^` or the `NAME`. The header SHALL be written before the command starts, so that it precedes everything the command writes. Headers SHALL be colored only as the `cli-contract` capability allows.

#### Scenario: Headers in the current repository
- **WHEN** a repository at `/src/repo` has the linked worktree `/src/repo.worktrees/feat`, and the user runs `wt each true` on macOS or Linux, with stdout piped
- **THEN** stdout is the two lines `==> ^  /src/repo` and `==> feat  /src/repo.worktrees/feat`

#### Scenario: Headers of every repository
- **WHEN** `<root>/api` has the linked worktree `feat`, and the user runs `wt each --all-repos true` on macOS or Linux, with stdout piped
- **THEN** stdout contains the lines `==> api:^  <root>/api` and `==> api:feat  <path of feat>`

#### Scenario: Header before the output
- **WHEN** the user runs `wt each git branch --show-current` in a repository with the linked worktree `feat`
- **THEN** in stdout, the line `==> feat  <path of feat>` comes right before the line `feat`

### Requirement: Failures
A command that exits with a code other than 0, that on macOS or Linux is terminated by a signal, or that cannot be started SHALL count as failed, and `wt each` SHALL go on with the next worktree. When every command exits with 0, `wt each` SHALL write nothing after the last command's output and SHALL exit 0. When at least one command failed, after the last command `wt each` SHALL write to stderr the line `wt: <f> of <m> commands failed`, where `<f>` is the number of failed commands and `<m>` the number of commands run, followed by one `hint:` line per failed command, in the order they ran: `<label> exited with <code>`, where `<code>` is the command's exit code or, for a command terminated by a signal, 128 plus the signal number; or `<label> could not start: <reason>`. The exit code SHALL then be 1, whatever the commands' exit codes.

#### Scenario: One command fails
- **WHEN** a repository has its main worktree and the linked worktrees `feat` and `fix`, and the user runs `wt each <a command that exits with 3 in feat and with 0 elsewhere>`
- **THEN** the command also runs in `fix`
- **AND** stderr contains `wt: 1 of 3 commands failed` and the line `hint: feat exited with 3`
- **AND** the exit code is 1

#### Scenario: Every command succeeds
- **WHEN** the user runs `wt each true` in a repository with two linked worktrees, on macOS or Linux
- **THEN** stderr is empty
- **AND** the exit code is 0

#### Scenario: Terminated by a signal (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt each 'kill -TERM $$'` in a repository with no linked worktree
- **THEN** stderr contains `hint: ^ exited with 143`
- **AND** the exit code is 1

### Requirement: Interrupt
An interrupt from the terminal (Ctrl-C) while `wt each` runs SHALL reach the command that is running and SHALL NOT end `wt` before that command exits. Once it exits, `wt each` SHALL NOT start another command, and SHALL write to stderr the line `wt: interrupted: <k> of <m> commands did not run`, where `<k>` is the number of commands not started and `<m>` the number of commands it would have run, followed by the `hint:` lines of "Failures" for the commands that failed. The exit code SHALL be 130 on every operating system, whatever the commands' exit codes.

#### Scenario: Interrupt while a command runs (macOS, Linux)
- **WHEN** on macOS or Linux the user runs `wt each 'touch started; sleep 1; touch finished'` in a repository with the linked worktree `feat`, and `wt` receives SIGINT while the command runs in the main worktree
- **THEN** `wt` exits only after that command does
- **AND** the main worktree has the file `finished`
- **AND** `feat` has no file `started`
- **AND** stderr contains `wt: interrupted: 1 of 2 commands did not run`
- **AND** the exit code is 130

#### Scenario: Ctrl-C in the console (Windows)
- **WHEN** on Windows the user runs `wt each <a command that waits for a key>` in a repository with the linked worktree `feat` and presses Ctrl-C while it runs in the main worktree
- **THEN** the command does not run in `feat`
- **AND** stderr contains `wt: interrupted: 1 of 2 commands did not run`
- **AND** `$LASTEXITCODE` is 130

### Requirement: Dry run
`wt each --dry-run` SHALL perform every check and read of `wt each`, with the same errors, warnings and exit codes, and SHALL NOT run any command. Instead of the headers and the commands, it SHALL print to stdout, for each worktree the command would run in and in that order, the line `would run <command line> in <path>`, where `<command line>` is built as "Command line" specifies and `<path>` is the worktree's path in the operating system's native form. It SHALL then exit 0.

#### Scenario: Preview
- **WHEN** a repository at `/src/repo` has the linked worktree `/src/repo.worktrees/feat`, and the user runs `wt each --dry-run 'touch ran'` on macOS or Linux
- **THEN** stdout is `would run touch ran in /src/repo` and `would run touch ran in /src/repo.worktrees/feat`, in that order
- **AND** no file `ran` exists in either worktree
- **AND** the exit code is 0

### Requirement: No JSON output
`wt each --json` SHALL be a usage error (exit 2), reported before any repository is read and as the `cli-contract` capability specifies for failures with `--json`, with a hint naming `wt list --json`; no command SHALL run. A `--json` after the command's first word SHALL be a word of the command (see "Start of the command").

#### Scenario: JSON requested
- **WHEN** the user runs `wt each --json git status` inside a repository
- **THEN** stdout is empty
- **AND** stderr is a `wt.error.v1` document with `code` 2 whose `hints` name `wt list --json`
- **AND** no command runs
- **AND** the exit code is 2

### Requirement: Completions
Completions of `wt each` SHALL offer its flags and the global ones while no word of the command has been typed. For the words of the command, they SHALL offer the shell's own completion of file names, and SHALL never offer a flag of `wt`. Requesting a completion SHALL never write to the terminal.

#### Scenario: Flags before the command
- **WHEN** the user requests completions for `wt each --a`
- **THEN** `--all-repos` is offered

#### Scenario: Flags after the first word
- **WHEN** the user requests completions for `wt each git log --a`
- **THEN** no flag of `wt` is offered

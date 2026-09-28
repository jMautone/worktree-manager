package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Runner runs git. It is the only edge through which wt touches git, so the
// code that decides can be tested without a repository.
type Runner interface {
	// Run runs git with args in dir and returns its stdout.
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// ErrNotInstalled means no git executable was found on the PATH.
var ErrNotInstalled = errors.New("git not found")

// NotRepoError means git associates no repository with Dir.
type NotRepoError struct {
	Dir string
}

func (e *NotRepoError) Error() string { return "not a git repository: " + e.Dir }

// CommandError is any other git failure. Stderr is git's own message.
type CommandError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	msg := e.Stderr
	if msg == "" {
		msg = e.Err.Error()
	}
	return "git " + strings.Join(e.Args, " ") + ": " + msg
}

func (e *CommandError) Unwrap() error { return e.Err }

// Exec runs the git executable found on the PATH.
type Exec struct {
	// Env is the environment for git; nil means the current process's.
	Env []string
}

// Run implements Runner. Git runs with LC_ALL=C: wt tells "not a git
// repository" apart from other failures by git's message, which is
// translated in other locales.
func (x Exec) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	env := x.Env
	if env == nil {
		env = os.Environ()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(env[:len(env):len(env)], "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrNotInstalled
	}
	msg := strings.TrimSpace(stderr.String())
	if strings.Contains(msg, "not a git repository") {
		return nil, &NotRepoError{Dir: dir}
	}
	return nil, &CommandError{Args: args, Stderr: msg, Err: err}
}

// ListWorktrees returns the worktrees of the repository associated with dir.
// Unlike `rev-parse --show-toplevel`, `worktree list` works from anywhere in
// the repository, including inside a bare repository.
func ListWorktrees(ctx context.Context, r Runner, dir string) ([]WorktreeEntry, error) {
	out, err := r.Run(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktreeList(out)
}

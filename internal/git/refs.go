package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// ErrNoRevision means a revision does not resolve to a commit.
var ErrNoRevision = errors.New("no such revision")

// InvalidBranchError means git does not accept Name as the name of a new
// branch, as written.
type InvalidBranchError struct {
	Name string
}

func (e *InvalidBranchError) Error() string { return "invalid branch name " + quote(e.Name) }

func quote(s string) string { return `"` + s + `"` }

// exitCode is git's exit code in a *CommandError, or -1.
func exitCode(err error) int {
	var ce *CommandError
	var ee *exec.ExitError
	if errors.As(err, &ce) && errors.As(ce.Err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// lines splits git's output into its non-empty lines.
func lines(out []byte) []string {
	var ls []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// CheckBranchName checks with git that name is valid for a new branch. git
// expands some names (@{-1} is the previous branch); wt requires the result
// to be name itself, so that nothing is expanded.
func CheckBranchName(ctx context.Context, r Runner, dir, name string) error {
	out, err := r.Run(ctx, dir, "check-ref-format", "--branch", name)
	var ce *CommandError
	switch {
	case errors.As(err, &ce):
		return &InvalidBranchError{Name: name}
	case err != nil:
		return err
	case strings.TrimRight(string(out), "\r\n") != name:
		return &InvalidBranchError{Name: name}
	}
	return nil
}

// Refs returns the full names of the references that match patterns, as in
// `git for-each-ref` (refs/heads matches every local branch).
func Refs(ctx context.Context, r Runner, dir string, patterns ...string) ([]string, error) {
	out, err := r.Run(ctx, dir, append([]string{"for-each-ref", "--format=%(refname)"}, patterns...)...)
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

// Remotes returns the names of the configured remotes.
func Remotes(ctx context.Context, r Runner, dir string) ([]string, error) {
	out, err := r.Run(ctx, dir, "remote")
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

// OriginHEAD returns the remote-tracking branch origin/HEAD points to, as
// origin/<branch>, or "" when origin/HEAD does not exist.
func OriginHEAD(ctx context.Context, r Runner, dir string) (string, error) {
	out, err := r.Run(ctx, dir, "for-each-ref", "--format=%(symref:short)", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SymbolicHEAD returns the branch HEAD points to in dir, or "" when HEAD is
// detached. For a bare repository it is the branch of the bare repository.
func SymbolicHEAD(ctx context.Context, r Runner, dir string) (string, error) {
	out, err := r.Run(ctx, dir, "symbolic-ref", "-q", "--short", "HEAD")
	if exitCode(err) == 1 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveCommit returns the full id of the commit rev resolves to, or
// ErrNoRevision. --end-of-options keeps a rev that starts with - from being
// read as an option.
func ResolveCommit(ctx context.Context, r Runner, dir, rev string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	var ce *CommandError
	if exitCode(err) == 1 && errors.As(err, &ce) && ce.Stderr == "" {
		return "", ErrNoRevision
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Fetch fetches remote with git's own configuration for it (refspecs,
// fetch.prune). Its output is captured, like every git command wt runs.
func Fetch(ctx context.Context, r Runner, dir, remote string) error {
	_, err := r.Run(ctx, dir, "fetch", "--end-of-options", remote)
	return err
}

// CreateBranch creates branch at commit, with no upstream whatever commit
// is. branch must have passed CheckBranchName, which rejects a leading -.
func CreateBranch(ctx context.Context, r Runner, dir, branch, commit string) error {
	_, err := r.Run(ctx, dir, "branch", "--no-track", branch, commit)
	return err
}

// DeleteBranch deletes branch, merged or not.
func DeleteBranch(ctx context.Context, r Runner, dir, branch string) error {
	_, err := r.Run(ctx, dir, "branch", "-D", branch)
	return err
}

// AddWorktree creates a worktree at path, an absolute path, with the
// existing branch checked out. git creates the missing parent directories
// of path.
func AddWorktree(ctx context.Context, r Runner, dir, path, branch string) error {
	_, err := r.Run(ctx, dir, "worktree", "add", path, branch)
	return err
}

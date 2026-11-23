package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// TrackedChanges reports whether the worktree at dir has changes to tracked
// files, staged or not. Untracked and ignored files do not count: unlike
// git worktree remove, a rebase does not lose them.
func TrackedChanges(ctx context.Context, r Runner, dir string) (bool, error) {
	out, err := r.Run(ctx, dir, "status", "--porcelain", "-z", "--untracked-files=no", "--ignore-submodules=none")
	if err != nil {
		return false, err
	}
	return len(strings.Trim(string(out), "\x00")) > 0, nil
}

// inProgress is what git leaves in a worktree's own git directory while an
// operation is stopped, in the order InProgress checks them.
var inProgress = []struct{ path, op string }{
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
	{"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
}

// InProgress returns the operation stopped in the worktree at dir:
// "rebase", "merge", "cherry-pick", "revert", or "" when there is none.
// --git-path names the worktree's own git directory, so a linked worktree
// is told apart from the main one.
func InProgress(ctx context.Context, r Runner, dir string) (string, error) {
	args := []string{"rev-parse"}
	for _, p := range inProgress {
		args = append(args, "--git-path", p.path)
	}
	out, err := r.Run(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	paths := lines(out)
	for i, p := range inProgress {
		if i >= len(paths) {
			break
		}
		path := paths[i]
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			return p.op, nil
		}
	}
	return "", nil
}

// Rebase rebases the branch checked out in the worktree at dir onto commit,
// without stashing and without moving other branches, whatever git's
// configuration says: the flag beats rebase.autoStash, and -c covers a
// wrapper that would drop the flag.
func Rebase(ctx context.Context, r Runner, dir, commit string) error {
	_, err := r.Run(ctx, dir, "-c", "rebase.autoStash=false", "-c", "rebase.updateRefs=false",
		"rebase", "--no-autostash", commit)
	return err
}

// RebaseAbort aborts the rebase in progress in the worktree at dir, leaving
// its branch, HEAD and files as they were before it started.
func RebaseAbort(ctx context.Context, r Runner, dir string) error {
	_, err := r.Run(ctx, dir, "rebase", "--abort")
	return err
}

// Unmerged reports whether the worktree at dir has unmerged paths: what a
// rebase stopped on a conflict leaves.
func Unmerged(ctx context.Context, r Runner, dir string) (bool, error) {
	out, err := r.Run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

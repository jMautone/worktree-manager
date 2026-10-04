package git

import (
	"context"
	"strings"
)

// Status returns the entries of `git status --porcelain -z` in dir: one per
// change to a tracked file, staged or not, and one per untracked file or
// directory; none for ignored files. It is the check git worktree remove
// makes, with --untracked-files=normal so that status.showUntrackedFiles =
// no does not hide files that would be lost.
func Status(ctx context.Context, r Runner, dir string) ([]string, error) {
	out, err := r.Run(ctx, dir, "status", "--porcelain", "-z", "--ignore-submodules=none", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	var entries []string
	for _, e := range strings.Split(string(out), "\x00") {
		if e != "" {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// Upstream returns the upstream of the local branch as configured, as in
// origin/feat, or "" when it has none. The upstream is reported even when
// its reference no longer exists: ResolveCommit tells.
func Upstream(ctx context.Context, r Runner, dir, branch string) (string, error) {
	out, err := r.Run(ctx, dir, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor reports whether commit a is commit b or one of its ancestors.
// Both are expected to be full commit ids, already resolved.
func IsAncestor(ctx context.Context, r Runner, dir, a, b string) (bool, error) {
	_, err := r.Run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	if exitCode(err) == 1 {
		return false, nil
	}
	return err == nil, err
}

// RemoveWorktree deletes the linked worktree at path, an absolute path, and
// git's record of it; when the directory no longer exists it only removes
// the record. Without force git refuses when the worktree has modified or
// untracked files. When git cannot delete the directory it still removes
// the record, and fails.
func RemoveWorktree(ctx context.Context, r Runner, dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := r.Run(ctx, dir, append(args, path)...)
	return err
}

// LockWorktree locks the worktree at path; an empty reason locks it
// without one.
func LockWorktree(ctx context.Context, r Runner, dir, path, reason string) error {
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	_, err := r.Run(ctx, dir, append(args, path)...)
	return err
}

// UnlockWorktree unlocks the worktree at path.
func UnlockWorktree(ctx context.Context, r Runner, dir, path string) error {
	_, err := r.Run(ctx, dir, "worktree", "unlock", path)
	return err
}

// PruneWorktrees removes git's record of every worktree whose directory no
// longer exists, except locked ones.
func PruneWorktrees(ctx context.Context, r Runner, dir string) error {
	_, err := r.Run(ctx, dir, "worktree", "prune")
	return err
}

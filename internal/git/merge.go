package git

import (
	"context"
	"fmt"
)

// FastForward moves the local branch from old to commit, which must contain
// old. When checkedOut is the path of the worktree that has branch checked
// out, it runs git merge --ff-only there, so that its index and files move
// with it; git refuses before overwriting an untracked file. Otherwise it
// runs git update-ref with old, so that a branch moved meanwhile is not
// overwritten.
func FastForward(ctx context.Context, r Runner, dir, branch, old, commit, checkedOut string) error {
	ref := "refs/heads/" + branch
	if checkedOut == "" {
		_, err := r.Run(ctx, dir, "update-ref", "-m", "wt merge", ref, commit, old)
		return err
	}
	// merge --ff-only takes no old value: the branch is checked first.
	cur, err := ResolveCommit(ctx, r, checkedOut, ref)
	if err != nil {
		return err
	}
	if cur != old {
		return fmt.Errorf("%s moved while merging: it is at %s, not %s", branch, cur, old)
	}
	_, err = r.Run(ctx, checkedOut, "-c", "merge.autoStash=false", "merge", "--ff-only", "--no-autostash", "--quiet", "--end-of-options", commit)
	return err
}

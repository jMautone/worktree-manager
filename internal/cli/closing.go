package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// closing is what wt merge and wt step read of the current worktree and of
// its target before deciding anything.
type closing struct {
	res       resolved
	cfg       *config.Config
	w         worktree.Worktree
	main      worktree.Worktree
	gitDir    string
	target    string
	old       string // the target's commit
	targetDir string // the worktree that has the target checked out, or ""
	state     worktree.MergeState
	tracked   bool // changes to tracked files
	inTarget  bool // HEAD is the target's commit or an ancestor of it
}

// readClosing reads the current worktree, which cannot be the main one
// (mainMsg is the error), and, withTarget, its target: target, or the base
// without its remote. early runs once the worktree is known, before the
// target is read. The pending changes are read unless an operation is in
// progress or HEAD is detached; the target's state only once it is found
// and is not the worktree's own branch. Nothing is checked but the main
// worktree: the checks are the caller's, on what is read.
func (a *app) readClosing(ctx context.Context, target string, withTarget bool, mainMsg string, early func(resolved) error) (closing, error) {
	var c closing
	dir, err := a.workdir()
	if err != nil {
		return c, err
	}
	res, cfg, err := a.resolve(ctx, dir, "@", modeWorktree)
	if err != nil {
		return c, err
	}
	w := *res.Worktree
	if w.Main || w.Bare {
		return c, &Error{Code: ExitUsage, Msg: mainMsg}
	}
	if early != nil {
		if err := early(res); err != nil {
			return c, err
		}
	}
	c.res, c.cfg, c.w, c.main, c.gitDir = res, cfg, w, mainWorktree(res.Repo), res.GitDir

	if withTarget && target == "" {
		base := configString(cfg, "default_base")
		if base == "" {
			if base, err = a.defaultBranch(ctx, res.GitDir, c.main); err != nil {
				return c, err
			}
		}
		remotes, err := git.Remotes(ctx, a.git, res.GitDir)
		if err != nil {
			return c, gitError(err)
		}
		target = worktree.MergeTarget(base, remotes)
	}
	c.target = target

	s := worktree.MergeState{Detached: w.Detached || w.Branch == ""}
	if s.InProgress, err = git.InProgress(ctx, a.git, w.Path); err != nil {
		return c, gitError(err)
	}
	if withTarget && !strings.HasPrefix(target, "-") {
		c.old, err = git.ResolveCommit(ctx, a.git, res.GitDir, "refs/heads/"+target)
		switch {
		case errors.Is(err, git.ErrNoRevision):
		case err != nil:
			return c, gitError(err)
		default:
			s.TargetFound = true
		}
	}
	s.TargetIsOwn = s.TargetFound && target == w.Branch
	if s.InProgress == "" && !s.Detached {
		entries, err := git.Status(ctx, a.git, w.Path)
		if err != nil {
			return c, gitError(err)
		}
		s.Changes = len(entries) > 0
		if s.Changes {
			if c.tracked, err = git.TrackedChanges(ctx, a.git, w.Path); err != nil {
				return c, gitError(err)
			}
		}
	}
	if s.InProgress == "" && !s.Detached && s.TargetFound && !s.TargetIsOwn {
		for _, o := range res.Repo.worktrees {
			if o.Bare || o.Detached || o.Branch != target {
				continue
			}
			c.targetDir = o.Path
			if info, err := os.Stat(o.Path); err != nil || !info.IsDir() {
				break
			}
			dirty, err := git.TrackedChanges(ctx, a.git, o.Path)
			if err != nil {
				return c, gitError(err)
			}
			if dirty {
				s.TargetDirty = o.Path
			}
		}
		if s.Contains, err = git.IsAncestor(ctx, a.git, w.Path, c.old, w.Head); err != nil {
			return c, gitError(err)
		}
		if c.inTarget, err = git.IsAncestor(ctx, a.git, w.Path, w.Head, c.old); err != nil {
			return c, gitError(err)
		}
	}
	c.state = s
	return c, nil
}

// moveTarget fast-forwards target from old to head, as the fourth step of
// wt merge and wt step push do, and prints its line. When the target
// already contains head, it says so and moves nothing. It returns the
// target's commit afterwards.
func (a *app) moveTarget(ctx context.Context, c closing, head string) (string, error) {
	branch := c.w.Branch
	already, err := git.IsAncestor(ctx, a.git, c.w.Path, head, c.old)
	if err != nil {
		return "", gitError(err)
	}
	if already {
		return c.old, a.printLine(worktree.MergeLine(worktree.MergeAlready, branch, c.target, false))
	}
	if err := git.FastForward(ctx, a.git, c.gitDir, c.target, c.old, head, c.targetDir); err != nil {
		return "", &Error{Code: ExitError, Msg: fmt.Sprintf("cannot move %s to %s: %s", c.target, branch, firstGitLine(err))}
	}
	return head, a.printLine(worktree.MergeLine(worktree.MergeMerged, branch, c.target, false))
}

// printLine prints line on stdout, except under --json.
func (a *app) printLine(line string) error {
	if a.json {
		return nil
	}
	_, err := fmt.Fprintln(a.env.Stdout, line)
	return err
}

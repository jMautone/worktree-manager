package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const lockLong = `Lock a worktree of the current repository, with an optional reason.

<target> is resolved as in "wt cd". A locked worktree is not removed by
"wt remove", even with --force, and not forgotten by "wt prune", even when
its directory is gone: lock a worktree on a drive that is not always
mounted. "wt unlock" unlocks it.`

const unlockLong = `Unlock a worktree of the current repository locked with "wt lock".

<target> is resolved as in "wt cd".`

func (a *app) lockCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lock <target> [<reason>]",
		Short: "Lock a worktree so that it is not removed or pruned",
		Long:  lockLong,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 2 {
				return usageError(cmd, fmt.Sprintf("unexpected argument %q for %q", args[2], cmd.CommandPath()))
			}
			return exactArgs("target")(cmd, args[:min(len(args), 1)])
		},
		// The second argument, the reason, is free text: nothing is offered.
		ValidArgsFunction: a.completeNames(func(w worktree.Worktree) bool { return !w.Main && !w.Locked }),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			var reason string
			if len(args) > 1 {
				reason = args[1]
			}
			return a.changeLock(cmd.Context(), args[0], true, reason)
		}),
	}
}

func (a *app) unlockCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "unlock <target>",
		Short:             "Unlock a worktree",
		Long:              unlockLong,
		Args:              exactArgs("target"),
		ValidArgsFunction: a.completeNames(func(w worktree.Worktree) bool { return w.Locked }),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return a.changeLock(cmd.Context(), args[0], false, "")
		}),
	}
}

// changeLock locks (lock) or unlocks the worktree target. Every check runs
// before git, which would fail on the same cases with less to say.
func (a *app) changeLock(ctx context.Context, target string, lock bool, reason string) error {
	dir, err := a.workdir()
	if err != nil {
		return err
	}
	repo, err := a.requireRepository(ctx, dir)
	if err != nil {
		return err
	}
	if _, err := a.loadConfig(repo.root()); err != nil {
		return err
	}
	w, err := a.resolveTarget(repo, target)
	if err != nil {
		return err
	}
	verb, check := "unlock", worktree.CheckUnlockable
	if lock {
		verb, check = "lock", worktree.CheckLockable
	}
	if err := lockBlocked(check(w)); err != nil {
		return err
	}

	if !a.dryRun {
		if lock {
			err = git.LockWorktree(ctx, a.git, dir, w.Path, reason)
		} else {
			err = git.UnlockWorktree(ctx, a.git, dir, w.Path)
		}
		if err != nil {
			return gitError(err)
		}
	}
	if a.json {
		if !lock {
			return a.writeJSON(struct {
				Schema string `json:"schema"`
				Name   string `json:"name"`
				Path   string `json:"path"`
			}{"wt.unlock.v1", w.Name, w.Path})
		}
		doc := struct {
			Schema string  `json:"schema"`
			Name   string  `json:"name"`
			Path   string  `json:"path"`
			Reason *string `json:"reason"`
		}{Schema: "wt.lock.v1", Name: w.Name, Path: w.Path}
		if reason != "" {
			doc.Reason = &reason
		}
		return a.writeJSON(doc)
	}
	line := fmt.Sprintf("%sed worktree %s\n", verb, w.Path)
	if a.dryRun {
		line = fmt.Sprintf("would %s worktree %s\n", verb, w.Path)
	}
	_, err = fmt.Fprint(a.env.Stdout, line)
	return err
}

// lockBlocked maps the errors of CheckLockable and CheckUnlockable to the
// exit-code contract.
func lockBlocked(err error) error {
	var me *worktree.MainWorktreeError
	var le *worktree.LockedError
	var nle *worktree.NotLockedError
	switch {
	case errors.As(err, &me):
		return &Error{Code: ExitUsage, Msg: me.Error()}
	case errors.As(err, &le):
		return &Error{Code: ExitBlocked, Msg: le.Error()}
	case errors.As(err, &nle):
		return &Error{Code: ExitBlocked, Msg: nle.Error()}
	}
	return err
}

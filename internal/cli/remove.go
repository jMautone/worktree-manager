package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const removeLong = `Remove a worktree of the current repository, and its branch when it is
merged.

<target> is resolved as in "wt cd": the worktree with that NAME or, if none
has it, the one with that branch checked out; @ is the current worktree.
The main worktree is never removed.

Nothing is removed while the worktree has modified or untracked files
(-f removes it with them), is locked (see "wt unlock"), or contains another
worktree. Files that git ignores, such as node_modules or an ignored .env,
are not checked: they are deleted with the worktree, as git does.

The branch is deleted when it is merged: when its last commit is in the
base (default_base, else the repository's default branch, as in "wt
create") or in the branch's upstream. Only the references already here
count; nothing is fetched. A squash merge does not count, because the
branch's commits are not in the base: such a branch is kept, and -D deletes
it. A branch pushed to its upstream counts as merged even while its pull
request is open; its commits stay on the remote, and --keep-branch keeps
the local branch.

-f and -D are independent: -f never deletes a branch that is not merged,
and -D never removes a worktree with modified or untracked files.

When the shell is inside the worktree, it ends up in the main worktree,
through the wt shell function (see "wt shell init --help").`

// removeFlags are the flags of wt remove.
type removeFlags struct {
	keepBranch  bool
	forceDelete bool
	force       bool
}

func (a *app) removeCommand() *cobra.Command {
	var f removeFlags
	cmd := &cobra.Command{
		Use:   "remove <target>",
		Short: "Remove a worktree, and its branch when it is merged",
		Long:  removeLong,
		Args:  exactArgs("target"),
		// The worktrees that could be removed, including those whose
		// directory is gone.
		ValidArgsFunction: a.completeNames(func(w worktree.Worktree) bool { return !w.Main && !w.Locked }),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			if f.keepBranch && f.forceDelete {
				return usageError(cmd, "--keep-branch and -D cannot be used together")
			}
			plan, main, err := a.planRemove(cmd.Context(), args[0], f)
			if err != nil {
				return err
			}
			if a.dryRun {
				return a.previewRemove(plan)
			}
			return a.remove(cmd.Context(), plan, main)
		}),
	}
	fs := cmd.Flags()
	fs.BoolVar(&f.keepBranch, "keep-branch", false, "keep the branch, even if it is merged")
	fs.BoolVarP(&f.forceDelete, "force-delete-branch", "D", false, "delete the branch, even if it is not merged")
	fs.BoolVarP(&f.force, "force", "f", false, "remove the worktree even if it has modified or untracked files")
	return cmd
}

// planRemove reads everything wt remove needs and decides what it would do.
// It only reads: every check that can fail by the state of the repository
// happens here, before anything is removed. It also returns the main
// worktree, where the effects run.
func (a *app) planRemove(ctx context.Context, target string, f removeFlags) (worktree.RemovePlan, worktree.Worktree, error) {
	var plan worktree.RemovePlan
	dir, err := a.workdir()
	if err != nil {
		return plan, worktree.Worktree{}, err
	}
	repo, err := a.requireRepository(ctx, dir)
	if err != nil {
		return plan, worktree.Worktree{}, err
	}
	cfg, err := a.loadConfig(repo.root())
	if err != nil {
		return plan, worktree.Worktree{}, err
	}
	w, err := a.resolveTarget(repo, target)
	if err != nil {
		return plan, worktree.Worktree{}, err
	}
	if err := removeBlocked(worktree.CheckRemovable(w, repo.worktrees, a.env.GOOS)); err != nil {
		return plan, worktree.Worktree{}, err
	}
	if info, err := os.Stat(w.Path); err == nil && info.IsDir() && !f.force {
		entries, err := git.Status(ctx, a.git, w.Path)
		if err != nil {
			return plan, worktree.Worktree{}, gitError(err)
		}
		if len(entries) > 0 {
			return plan, worktree.Worktree{}, &Error{
				Code: ExitBlocked,
				Msg:  fmt.Sprintf("worktree %q has modified or untracked files", w.Name),
				Hints: []string{
					fmt.Sprintf("see them with 'git -C %s status'", w.Path),
					"--force removes the worktree with them; they are lost",
				},
			}
		}
	}
	main := mainWorktree(repo)

	var m worktree.Merged
	if w.Branch != "" && !f.keepBranch && !f.forceDelete {
		if m, err = a.readMerged(ctx, dir, cfg, main, w); err != nil {
			return plan, worktree.Worktree{}, err
		}
	}
	del, keepReason := worktree.BranchOutcome(w.Branch, f.keepBranch, f.forceDelete, m)

	// The shell moves when wt started inside the worktree; -C does not
	// count, since it does not move the shell.
	var cd string
	cwd, err := a.env.Getwd()
	if err != nil {
		return plan, worktree.Worktree{}, fmt.Errorf("cannot determine the working directory: %w", err)
	}
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = real
	}
	if worktree.Contains(w.Path, cwd, a.env.GOOS) {
		cd = main.Path
	}

	return worktree.RemovePlan{
		Worktree:     w,
		Force:        f.force,
		Branch:       w.Branch,
		DeleteBranch: del,
		KeepReason:   keepReason,
		CD:           cd,
	}, main, nil
}

// removeBlocked maps the errors of CheckRemovable to the exit-code
// contract.
func removeBlocked(err error) error {
	var me *worktree.MainWorktreeError
	var le *worktree.LockedError
	var ce *worktree.ContainsError
	switch {
	case errors.As(err, &me):
		return &Error{Code: ExitUsage, Msg: me.Error()}
	case errors.As(err, &le):
		return &Error{Code: ExitBlocked, Msg: le.Error(), Hints: []string{fmt.Sprintf("run 'wt unlock %s' first", le.Name)}}
	case errors.As(err, &ce):
		return &Error{Code: ExitBlocked, Msg: ce.Error(), Hints: []string{fmt.Sprintf("remove %q first", ce.Inner.Name)}}
	}
	return err
}

func mainWorktree(repo *repository) worktree.Worktree {
	for _, w := range repo.worktrees {
		if w.Main {
			return w
		}
	}
	return worktree.Worktree{}
}

// readMerged reads what BranchOutcome needs to know whether the branch of w
// is merged: whether its tip is in the base and in its upstream. A base
// that cannot be used is a warning, not an error: only the upstream counts
// then. An upstream that no longer exists does not count, without a
// warning: a forge deletes the branch of a merged pull request.
func (a *app) readMerged(ctx context.Context, dir string, cfg *config.Config, main, w worktree.Worktree) (worktree.Merged, error) {
	var m worktree.Merged
	cannotTell := func(why string) {
		a.warn(fmt.Sprintf("cannot tell whether branch %s is merged: %s", w.Branch, why))
	}
	in := func(rev string) (bool, error) {
		commit, err := git.ResolveCommit(ctx, a.git, dir, rev)
		if errors.Is(err, git.ErrNoRevision) {
			return false, err
		}
		if err != nil {
			return false, gitError(err)
		}
		ok, err := git.IsAncestor(ctx, a.git, dir, w.Head, commit)
		if err != nil {
			return false, gitError(err)
		}
		return ok, nil
	}

	base := configString(cfg, "default_base")
	if base == "" {
		branch, ok, err := a.readDefaultBranch(ctx, dir, main)
		if err != nil {
			return m, err
		}
		if !ok {
			cannotTell("the repository has no default branch")
		}
		base = branch
	}
	if base != "" {
		ok, err := in(base)
		switch {
		case errors.Is(err, git.ErrNoRevision):
			cannotTell("base not found: " + base)
		case err != nil:
			return m, err
		default:
			m.Base, m.InBase = base, ok
		}
	}

	up, err := git.Upstream(ctx, a.git, dir, w.Branch)
	if err != nil {
		return m, gitError(err)
	}
	if up != "" {
		ok, err := in(up)
		if err != nil && !errors.Is(err, git.ErrNoRevision) {
			return m, err
		}
		m.InUpstream = ok
	}
	return m, nil
}

// previewRemove prints what wt remove would do, from the same plan.
func (a *app) previewRemove(plan worktree.RemovePlan) error {
	active := shell.Active(a.env.Getenv)
	if plan.CD != "" && !active {
		a.warnShellGone()
	}
	if a.json {
		return a.writeJSON(removeDocument(plan))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "would remove worktree %s\n", plan.Worktree.Path)
	if plan.Branch != "" {
		b.WriteString(branchLine(plan, true) + "\n")
	}
	if plan.CD != "" && active {
		fmt.Fprintf(&b, "would change directory to %s\n", plan.CD)
	}
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

// branchLine says what happens to the branch: what would happen under
// --dry-run, what happened otherwise.
func branchLine(plan worktree.RemovePlan, dryRun bool) string {
	deleted, kept := "deleted", "kept"
	if dryRun {
		deleted, kept = "would delete", "would keep"
	}
	switch {
	case plan.DeleteBranch:
		return deleted + " branch " + plan.Branch
	case plan.KeepReason != "":
		return fmt.Sprintf("%s branch %s: %s", kept, plan.Branch, plan.KeepReason)
	}
	return kept + " branch " + plan.Branch
}

// remove carries out the plan from the main worktree: the worktree, then
// the branch, then the shell, and the output last, so that stdout stays
// empty on any error. An error after the worktree is gone says so.
func (a *app) remove(ctx context.Context, plan worktree.RemovePlan, main worktree.Worktree) error {
	path := plan.Worktree.Path
	// On Windows a directory that is a process's working directory cannot
	// be deleted, and wt may have started inside the worktree. Elsewhere it
	// is not needed, but one code path is tested on every OS. A failure is
	// not fatal: git's own error explains it, on Windows.
	if a.env.Chdir != nil {
		_ = a.env.Chdir(main.Path)
	}
	if err := git.RemoveWorktree(ctx, a.git, main.Path, path, plan.Force); err != nil {
		e := &Error{Code: ExitError, Msg: err.Error()}
		if ge, ok := gitError(err).(*Error); ok {
			e = ge
		}
		if _, err := os.Lstat(path); err == nil {
			e.Hints = append(e.Hints, "the directory is still there: "+path)
		}
		return e
	}

	var failed error
	if plan.DeleteBranch {
		if err := git.DeleteBranch(ctx, a.git, main.Path, plan.Branch); err != nil {
			failed = &Error{Code: ExitError, Msg: fmt.Sprintf("removed worktree %s, but cannot delete branch %s: %s", path, plan.Branch, gitMessage(err))}
		}
	}
	// The directory is gone whatever happened to the branch: the shell must
	// not stay in it.
	if plan.CD != "" {
		if !shell.Active(a.env.Getenv) {
			a.warnShellGone()
		} else if err := shell.WriteDirective(a.env.Getenv(shell.DirectiveVar), plan.CD); err != nil && failed == nil {
			failed = &Error{Code: ExitError, Msg: fmt.Sprintf("removed worktree %s, but cannot write the directive file: %v", path, err)}
		}
	}
	if failed != nil {
		return failed
	}

	if a.json {
		return a.writeJSON(removeDocument(plan))
	}
	out := fmt.Sprintf("removed worktree %s\n", path)
	if plan.Branch != "" {
		out += branchLine(plan, false) + "\n"
	}
	_, err := a.env.Stdout.Write([]byte(out))
	return err
}

// gitMessage is git's own message in err, or err itself.
func gitMessage(err error) string {
	var ce *git.CommandError
	if errors.As(err, &ce) && ce.Stderr != "" {
		return ce.Stderr
	}
	return err.Error()
}

func (a *app) warnShellGone() {
	a.warn("shell integration is not active, so the shell stays in a directory that no longer exists; see 'wt shell init --help'")
}

// removeJSON is the wt.remove.v1 document.
type removeJSON struct {
	Schema        string  `json:"schema"`
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	Branch        *string `json:"branch"`
	BranchDeleted bool    `json:"branch_deleted"`
}

func removeDocument(plan worktree.RemovePlan) removeJSON {
	doc := removeJSON{Schema: "wt.remove.v1", Name: plan.Worktree.Name, Path: plan.Worktree.Path, BranchDeleted: plan.DeleteBranch}
	if plan.Branch != "" {
		doc.Branch = &plan.Branch
	}
	return doc
}

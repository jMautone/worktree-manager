package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/hook"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const mergeLong = `Merge the current worktree into a branch, and remove it.

The worktree is the one the shell is in, or the one -C names; the main
worktree cannot be merged. The target is <target> when given, otherwise
the repository's base (default_base, or its default branch; see
"wt config --help") without its remote: origin/main gives main. It must be
a local branch. wt merge neither fetches nor pushes.

In order, stopping at the first step that fails:

  1. check that the worktree has no modified or untracked files and no
     operation in progress, that the target, if checked out in another
     worktree, has no uncommitted changes there, and that the repository's
     hooks that would run are approved;
  2. rebase the branch onto the target, unless it already contains it. A
     conflict aborts the rebase, leaving the worktree as it was;
  3. run the pre-merge hook in the worktree;
  4. fast-forward the target to the branch; where the target is checked
     out, its files are updated;
  5. remove the worktree and delete its branch, as "wt remove" does, with
     its hooks, and move the shell to the main worktree;
  6. run the post-merge hook, in the background, in the main worktree.

When pre-merge fails, the branch stays rebased and the target does not
move. When a step after the target moved fails, the target stays moved.

--no-rebase does not rebase: the branch must already contain the target.
--no-remove keeps the worktree and its branch.`

type mergeFlags struct {
	noRebase bool
	noRemove bool
	noHooks  bool
}

func (a *app) mergeCommand() *cobra.Command {
	var f mergeFlags
	cmd := &cobra.Command{
		Use:   "merge [<target>]",
		Short: "Merge the current worktree into a branch, and remove it",
		Long:  mergeLong,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError(cmd, fmt.Sprintf("unexpected argument %q for %q", args[1], cmd.CommandPath()))
			}
			return nil
		},
		ValidArgsFunction: a.completeMergeTargets,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			// A Ctrl-C does not end wt: it reaches git, and wt moves
			// nothing once git ends.
			var interrupts chan os.Signal
			if !a.dryRun {
				interrupts = make(chan os.Signal, 1)
				signal.Notify(interrupts, os.Interrupt)
				defer signal.Stop(interrupts)
			}
			p, err := a.planMerge(cmd.Context(), target, f)
			if err != nil {
				return err
			}
			if a.dryRun {
				return a.previewMerge(cmd.Context(), p)
			}
			return a.merge(cmd.Context(), p, interrupts)
		}),
	}
	fs := cmd.Flags()
	fs.BoolVar(&f.noRebase, "no-rebase", false, "do not rebase; the branch must already contain the target")
	fs.BoolVar(&f.noRemove, "no-remove", false, "keep the worktree and its branch once merged")
	fs.BoolVar(&f.noHooks, "no-hooks", false, "run no hook")
	return cmd
}

// mergePlan is everything wt merge read and decided before its first
// effect.
type mergePlan struct {
	w         worktree.Worktree
	main      worktree.Worktree
	gitDir    string
	target    string
	old       string // the target's commit
	targetDir string // the worktree that has the target checked out, or ""
	rebase    bool
	pre, post hook.Run
	remove    *worktree.RemovePlan // nil with --no-remove
}

// planMerge reads everything wt merge needs and makes every check, in the
// order of the spec: nothing changes until it returns without error.
func (a *app) planMerge(ctx context.Context, target string, f mergeFlags) (mergePlan, error) {
	var p mergePlan
	dir, err := a.workdir()
	if err != nil {
		return p, err
	}
	res, cfg, err := a.resolve(ctx, dir, "@", modeWorktree)
	if err != nil {
		return p, err
	}
	w := *res.Worktree
	if w.Main || w.Bare {
		return p, &Error{Code: ExitUsage, Msg: "cannot merge the main worktree"}
	}
	if !f.noRemove {
		if err := removeBlocked(worktree.CheckRemovable(w, res.Repo.worktrees, a.env.GOOS)); err != nil {
			return p, err
		}
	}
	main := mainWorktree(res.Repo)
	p.w, p.main, p.gitDir = w, main, res.GitDir

	if target == "" {
		base := configString(cfg, "default_base")
		if base == "" {
			if base, err = a.defaultBranch(ctx, res.GitDir, main); err != nil {
				return p, err
			}
		}
		remotes, err := git.Remotes(ctx, a.git, res.GitDir)
		if err != nil {
			return p, gitError(err)
		}
		target = worktree.MergeTarget(base, remotes)
	}
	p.target = target

	s := worktree.MergeState{Detached: w.Detached || w.Branch == "", NoRebase: f.noRebase}
	if s.InProgress, err = git.InProgress(ctx, a.git, w.Path); err != nil {
		return p, gitError(err)
	}
	if !strings.HasPrefix(target, "-") {
		p.old, err = git.ResolveCommit(ctx, a.git, res.GitDir, "refs/heads/"+target)
		switch {
		case errors.Is(err, git.ErrNoRevision):
		case err != nil:
			return p, gitError(err)
		default:
			s.TargetFound = true
		}
	}
	s.TargetIsOwn = s.TargetFound && target == w.Branch
	if s.InProgress == "" && !s.Detached && s.TargetFound && !s.TargetIsOwn {
		entries, err := git.Status(ctx, a.git, w.Path)
		if err != nil {
			return p, gitError(err)
		}
		s.Changes = len(entries) > 0
		for _, o := range res.Repo.worktrees {
			if o.Bare || o.Detached || o.Branch != target {
				continue
			}
			p.targetDir = o.Path
			if info, err := os.Stat(o.Path); err != nil || !info.IsDir() {
				break
			}
			dirty, err := git.TrackedChanges(ctx, a.git, o.Path)
			if err != nil {
				return p, gitError(err)
			}
			if dirty {
				s.TargetDirty = o.Path
			}
		}
		if s.Contains, err = git.IsAncestor(ctx, a.git, w.Path, p.old, w.Head); err != nil {
			return p, gitError(err)
		}
	}
	if b := worktree.CheckMerge(w.Branch, target, s); b != nil {
		return p, &Error{Code: b.Code, Msg: b.Msg, Hints: b.Hints}
	}
	p.rebase = !s.Contains

	// Every hook that would run is approved before anything is done, so
	// that a hook of the removal does not stop wt with the target moved.
	hooks := &hookPlanner{a: a, ctx: ctx, cfg: cfg, noHooks: f.noHooks, gitDir: main.Path}
	events := []hook.Event{hook.PreMerge, hook.PostMerge}
	if !f.noRemove {
		events = append(events, hook.PreRemove, hook.PostRemove)
	}
	if err := hooks.check(events...); err != nil {
		return p, err
	}
	values := worktree.HookVars(main, w, "", a.env.GOOS)
	values["target"] = target
	if p.pre, err = hooks.plan(hook.PreMerge, values, w.Path); err != nil {
		return p, err
	}
	// post-merge runs in the main worktree: by then the worktree may be gone.
	if p.post, err = hooks.plan(hook.PostMerge, values, main.Path); err != nil {
		return p, err
	}
	if !f.noRemove {
		plan, _, err := a.planRemoveOf(ctx, res, cfg, removeFlags{forceDelete: true, noHooks: f.noHooks})
		if err != nil {
			return p, err
		}
		p.remove = &plan
	}
	return p, nil
}

// merge carries out the plan: rebase, pre-merge, fast-forward, remove and
// post-merge, printing each step as it is done.
func (a *app) merge(ctx context.Context, p mergePlan, interrupts <-chan os.Signal) error {
	branch, w := p.w.Branch, p.w
	head := w.Head
	rebasedHint := fmt.Sprintf("%s was rebased onto %s, and %s was not moved", branch, p.target, p.target)
	interrupted := func(rebased bool) error {
		e := &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %s was not merged into %s", branch, p.target)}
		if rebased {
			e.Hints = append(e.Hints, rebasedHint)
		}
		return e
	}

	if p.rebase {
		outcome, detail := a.rebaseOnto(ctx, w.Path, p.old)
		// The interrupt first: git stopped because of it.
		if pending(interrupts) {
			return interrupted(outcome == worktree.Rebased)
		}
		switch outcome {
		case worktree.Conflict:
			return &Error{
				Code:  ExitConflict,
				Msg:   fmt.Sprintf("conflict rebasing %s onto %s; rebase aborted", branch, p.target),
				Hints: []string{fmt.Sprintf("run \"git rebase %s\" in %s to resolve it, then wt merge again", p.target, w.Path)},
			}
		case worktree.Failed:
			return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot rebase %s onto %s: %s", branch, p.target, detail)}
		}
		var err error
		if head, err = git.ResolveCommit(ctx, a.git, w.Path, "HEAD"); err != nil {
			return gitError(err)
		}
		if err := a.mergeLine(worktree.MergeRebased, p); err != nil {
			return err
		}
	}

	if err := a.runBlocking(p.pre); err != nil {
		var e *Error
		if p.rebase && errors.As(err, &e) {
			e.Hints = append(e.Hints, rebasedHint)
		}
		return err
	}
	if pending(interrupts) {
		return interrupted(p.rebase)
	}

	already, err := git.IsAncestor(ctx, a.git, w.Path, head, p.old)
	if err != nil {
		return gitError(err)
	}
	if already {
		head = p.old
		if err := a.mergeLine(worktree.MergeAlready, p); err != nil {
			return err
		}
	} else {
		if err := git.FastForward(ctx, a.git, p.gitDir, p.target, p.old, head, p.targetDir); err != nil {
			e := &Error{Code: ExitError, Msg: fmt.Sprintf("cannot move %s to %s: %s", p.target, branch, firstGitLine(err))}
			if p.rebase {
				e.Hints = append(e.Hints, rebasedHint)
			}
			return e
		}
		if err := a.mergeLine(worktree.MergeMerged, p); err != nil {
			return err
		}
	}

	if p.remove != nil {
		report := func(plan worktree.RemovePlan) error {
			if a.json {
				return nil
			}
			_, err := a.env.Stdout.Write([]byte(removedLines(plan)))
			return err
		}
		if err := a.remove(ctx, *p.remove, p.main, report); err != nil {
			var e *Error
			if errors.As(err, &e) {
				e.Hints = append(e.Hints, fmt.Sprintf("%s was merged into %s", branch, p.target))
			}
			return err
		}
	}
	a.background(p.post)
	if a.json {
		return a.writeJSON(mergeDocument(p, head))
	}
	return nil
}

// mergeLine prints the line of a step, except under --json.
func (a *app) mergeLine(step worktree.MergeStep, p mergePlan) error {
	if a.json {
		return nil
	}
	_, err := fmt.Fprintln(a.env.Stdout, worktree.MergeLine(step, p.w.Branch, p.target, a.dryRun))
	return err
}

// previewMerge prints what wt merge would do, from the same plan.
func (a *app) previewMerge(ctx context.Context, p mergePlan) error {
	head := p.w.Head
	already := false
	if !p.rebase {
		var err error
		if already, err = git.IsAncestor(ctx, a.git, p.w.Path, head, p.old); err != nil {
			return gitError(err)
		}
	} else {
		// The rebased commits do not exist yet.
		head = ""
	}
	if already {
		head = p.old
	}
	active := shell.Active(a.env.Getenv)
	if p.remove != nil && p.remove.CD != "" && !active {
		a.warnShellGone()
	}
	if a.json {
		return a.writeJSON(mergeDocument(p, head))
	}
	var b strings.Builder
	if p.rebase {
		b.WriteString(worktree.MergeLine(worktree.MergeRebased, p.w.Branch, p.target, true) + "\n")
	}
	previewHook(&b, p.pre)
	step := worktree.MergeMerged
	if already {
		step = worktree.MergeAlready
	}
	b.WriteString(worktree.MergeLine(step, p.w.Branch, p.target, true) + "\n")
	if p.remove != nil {
		b.WriteString(previewRemoveLines(*p.remove, active))
	}
	previewHook(&b, p.post)
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

type mergeJSON struct {
	Schema  string  `json:"schema"`
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Branch  string  `json:"branch"`
	Target  string  `json:"target"`
	Rebased bool    `json:"rebased"`
	Head    *string `json:"head"`
	Removed bool    `json:"removed"`
}

// mergeDocument is wt.merge.v1 for p, with head the target's commit
// afterwards: null under --dry-run when a rebase would make it.
func mergeDocument(p mergePlan, head string) mergeJSON {
	doc := mergeJSON{
		Schema: "wt.merge.v1", Name: p.w.Name, Path: p.w.Path, Branch: p.w.Branch,
		Target: p.target, Rebased: p.rebase, Removed: p.remove != nil,
	}
	if head != "" {
		doc.Head = &head
	}
	return doc
}

// completeMergeTargets offers the local branches of the working directory's
// repository but the current worktree's own.
func (a *app) completeMergeTargets(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dir, err := a.workdir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	refs, err := git.Refs(cmd.Context(), a.git, dir, "refs/heads")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	own, _ := git.SymbolicHEAD(cmd.Context(), a.git, dir)
	var names []string
	for _, ref := range refs {
		if name := strings.TrimPrefix(ref, "refs/heads/"); name != own {
			names = append(names, name)
		}
	}
	return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}

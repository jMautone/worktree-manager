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

  1. check that the worktree has no operation in progress, that the
     target, if checked out in another worktree, has no uncommitted
     changes there, and that the repository's hooks that would run are
     approved;
  2. commit the pending changes, modified and untracked files, between
     the pre-commit and post-commit hooks; with --squash, replace the
     branch's own commits and the pending changes by one commit instead
     (see "wt step commit --help" for the message);
  3. rebase the branch onto the target, unless it already contains it. A
     conflict aborts the rebase, leaving the worktree as it was;
  4. run the pre-merge hook in the worktree;
  5. fast-forward the target to the branch; where the target is checked
     out, its files are updated;
  6. remove the worktree and delete its branch, as "wt remove" does, with
     its hooks, and move the shell to the main worktree;
  7. run the post-merge hook, in the background, in the main worktree.

The message of the commit is -m <msg>, or what you write in git's
editor; without a terminal, -m is needed. When the commit fails, nothing
changes. When pre-merge fails, the branch stays committed and rebased,
and the target does not move. When a step after the target moved fails,
the target stays moved.

--no-commit does not commit: pending changes block the merge.
--no-rebase does not rebase: the branch must already contain the target.
--no-remove keeps the worktree and its branch.`

type mergeFlags struct {
	noRebase bool
	noRemove bool
	noHooks  bool
	noCommit bool
	squash   bool
	stepFlags
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
			f.hasMessage = cmd.Flags().Changed("message")
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
	fs.BoolVar(&f.squash, "squash", false, "squash the branch into one commit before rebasing")
	fs.StringVarP(&f.message, "message", "m", "", "the message of the commit or the squash; without it, git's editor opens")
	fs.BoolVar(&f.noCommit, "no-commit", false, "do not commit pending changes; they block the merge")
	return cmd
}

// mergePlan is everything wt merge read and decided before its first
// effect.
type mergePlan struct {
	closing
	commit    *commitPlan // nil when nothing is committed or squashed
	noSquash  bool        // --squash with nothing to squash
	rebase    bool
	pre, post hook.Run
	remove    *worktree.RemovePlan // nil with --no-remove
}

// planMerge reads everything wt merge needs and makes every check, in the
// order of the spec: nothing changes until it returns without error.
func (a *app) planMerge(ctx context.Context, target string, f mergeFlags) (mergePlan, error) {
	var p mergePlan
	early := func(res resolved) error {
		if f.noRemove {
			return nil
		}
		return removeBlocked(worktree.CheckRemovable(*res.Worktree, res.Repo.worktrees, a.env.GOOS))
	}
	c, err := a.readClosing(ctx, target, true, "cannot merge the main worktree", early)
	if err != nil {
		return p, err
	}
	p.closing = c
	res, cfg, w, main, target := c.res, c.cfg, c.w, c.main, c.target
	s := c.state
	s.NoRebase = f.noRebase
	s.NoCommit = f.noCommit
	if b := worktree.CheckMerge(w.Branch, target, s); b != nil {
		return p, &Error{Code: b.Code, Msg: b.Msg, Hints: b.Hints}
	}
	p.rebase = !s.Contains

	// What is committed before the rebase: the pending changes, or the
	// squash of the branch, which includes them.
	kind := worktree.StepCommit
	if f.squash {
		kind = worktree.StepSquash
	}
	ss, err := a.stepState(ctx, kind, c, f.hasMessage)
	if err != nil {
		return p, err
	}
	commits := false
	if _, noop := worktree.StepNoop(kind, w.Branch, target, ss); !noop {
		commits = true
	} else if f.squash {
		p.noSquash = true
	}
	if commits {
		if worktree.NeedsMessage(kind, ss) && !ss.Terminal {
			b := worktree.MessageBlock()
			return p, &Error{Code: b.Code, Msg: b.Msg, Hints: b.Hints}
		}
		if f.hasMessage && git.Blank(f.message) {
			return p, emptyMessageError()
		}
	}

	// Every hook that would run is approved before anything is done, so
	// that a hook of the removal does not stop wt with the target moved.
	hooks := &hookPlanner{a: a, ctx: ctx, cfg: cfg, noHooks: f.noHooks, gitDir: main.Path}
	events := []hook.Event{hook.PreMerge, hook.PostMerge}
	if !f.noRemove {
		events = append(events, hook.PreRemove, hook.PostRemove)
	}
	if commits {
		events = append(events, hook.PreCommit, hook.PostCommit)
	}
	if err := hooks.check(events...); err != nil {
		return p, err
	}
	if commits {
		// post-commit runs in the main worktree when the worktree is
		// removed: by then it is gone.
		postDir := w.Path
		if !f.noRemove {
			postDir = main.Path
		}
		f.stepFlags.noHooks = f.noHooks
		cp, err := a.planCommit(ctx, hooks, c, ss, f.squash, f.stepFlags, postDir)
		if err != nil {
			return p, err
		}
		p.commit = &cp
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
		plan, _, err := a.planRemoveOf(ctx, res, cfg, removeFlags{forceDelete: true, noHooks: f.noHooks, committing: commits})
		if err != nil {
			return p, err
		}
		p.remove = &plan
	}
	return p, nil
}

// merge carries out the plan: commit or squash, rebase, pre-merge,
// fast-forward, remove and post-merge, printing each step as it is done.
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

	if p.commit != nil {
		var err error
		if head, err = a.makeCommit(ctx, *p.commit, interrupts); err != nil {
			return err
		}
		if p.remove == nil {
			a.background(p.commit.post)
		}
	} else if p.noSquash {
		if err := a.printLine("nothing to squash"); err != nil {
			return err
		}
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

	moved, err := a.moveTarget(ctx, p.closing, head)
	if err != nil {
		var e *Error
		if p.rebase && errors.As(err, &e) {
			e.Hints = append(e.Hints, rebasedHint)
		}
		return err
	}
	head = moved

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
	if p.commit != nil && p.remove != nil {
		a.background(p.commit.post)
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
	if p.commit != nil {
		previewHook(&b, p.commit.pre)
		b.WriteString(worktree.StepLine(p.commit.kind, p.w.Branch, "", "", p.commit.n, true) + "\n")
		if p.remove == nil {
			previewHook(&b, p.commit.post)
		}
	} else if p.noSquash {
		b.WriteString("nothing to squash\n")
	}
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
		if p.commit != nil {
			previewHook(&b, p.commit.post)
		}
	}
	previewHook(&b, p.post)
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

type mergeJSON struct {
	Schema    string  `json:"schema"`
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	Branch    string  `json:"branch"`
	Target    string  `json:"target"`
	Committed bool    `json:"committed"`
	Squashed  bool    `json:"squashed"`
	Rebased   bool    `json:"rebased"`
	Head      *string `json:"head"`
	Removed   bool    `json:"removed"`
}

// mergeDocument is wt.merge.v1 for p, with head the target's commit
// afterwards: null under --dry-run when a rebase would make it.
func mergeDocument(p mergePlan, head string) mergeJSON {
	doc := mergeJSON{
		Schema: "wt.merge.v1", Name: p.w.Name, Path: p.w.Path, Branch: p.w.Branch,
		Target: p.target, Rebased: p.rebase, Removed: p.remove != nil,
	}
	if p.commit != nil {
		doc.Committed = true
		doc.Squashed = p.commit.kind == worktree.StepSquash
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

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

const stepLong = `Run one step of "wt merge" on the current worktree, and nothing else.

  commit   commit every pending change: modified and untracked files, not
           the ignored ones
  squash   replace the branch's commits that the target does not contain,
           and the pending changes, by one commit on the same merge base
  rebase   rebase the branch onto the target, as "wt merge" does
  push     fast-forward the target to the branch, between the pre-merge and
           post-merge hooks. It moves a local branch: it never runs
           "git push"

The worktree is the one the shell is in, or the one -C names; the main
worktree has no steps. The target of squash, rebase and push is <target>
when given, otherwise the repository's base without its remote, as for
"wt merge". A step with nothing to do says so and exits 0.`

const commitMessageHelp = `The message is -m <msg> when given. Otherwise git opens its editor (the
one "git commit" would open) in the terminal; saving an empty message
commits nothing. Without a terminal, or with --json, -m is needed. The
repository's own git hooks run; --no-hooks skips only wt's pre-commit and
post-commit. When the commit fails, the branch and the index are left as
they were.`

type stepFlags struct {
	message    string
	hasMessage bool
	noHooks    bool
}

func (a *app) stepCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "step",
		Short: "Run one step of wt merge: commit, squash, rebase or push",
		Long:  stepLong,
		Args:  unknownSubcommand,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			cmd.SetOut(a.env.Stderr)
			_ = cmd.Help()
			return &exitStatus{code: ExitUsage}
		}),
	}
	cmd.SuggestionsMinimumDistance = 2
	cmd.AddCommand(
		a.stepSubcommand(worktree.StepCommit, "commit", "Commit every pending change of the current worktree",
			"Commit every pending change of the current worktree: changes to tracked\nfiles and untracked files that are not ignored.\n\n"+commitMessageHelp),
		a.stepSubcommand(worktree.StepSquash, "squash [<target>]", "Squash the branch into one commit",
			"Replace the commits of the branch that the target does not contain, and\nthe pending changes, by one commit whose parent is their merge base. The\neditor opens with the messages of the commits squashed, oldest first.\n\n"+commitMessageHelp+"\n\nIf wt is killed while it squashes, \"git reset --soft ORIG_HEAD\" puts the\nbranch back."),
		a.stepSubcommand(worktree.StepRebase, "rebase [<target>]", "Rebase the branch onto the target",
			"Rebase the branch onto the target, as \"wt merge\" does: never stashing,\nand aborting on a conflict, with the worktree as it was. Changes to\ntracked files block it; untracked files do not."),
		a.stepSubcommand(worktree.StepPush, "push [<target>]", "Fast-forward the target to the branch",
			"Fast-forward the target, a local branch, to the branch, as \"wt merge\"\ndoes, after the pre-merge hook and before the post-merge hook. The branch\nmust contain the target (\"wt step rebase\" first) and have no changes to\ntracked files. It never runs \"git push\" and never contacts a remote."),
	)
	return cmd
}

func (a *app) stepSubcommand(kind worktree.StepKind, use, short, long string) *cobra.Command {
	var f stepFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args: func(cmd *cobra.Command, args []string) error {
			max := 1
			if kind == worktree.StepCommit {
				max = 0
			}
			if len(args) > max {
				return usageError(cmd, fmt.Sprintf("unexpected argument %q for %q", args[max], cmd.CommandPath()))
			}
			return nil
		},
		RunE: action(func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			f.hasMessage = cmd.Flags().Changed("message")
			var interrupts chan os.Signal
			if !a.dryRun {
				interrupts = make(chan os.Signal, 1)
				signal.Notify(interrupts, os.Interrupt)
				defer signal.Stop(interrupts)
			}
			p, err := a.planStep(cmd.Context(), kind, target, f)
			if err != nil {
				return err
			}
			return a.step(cmd.Context(), p, interrupts)
		}),
	}
	if kind != worktree.StepCommit {
		cmd.ValidArgsFunction = a.completeMergeTargets
	} else {
		cmd.ValidArgsFunction = cobra.NoFileCompletions
	}
	fs := cmd.Flags()
	if kind.Commits() {
		fs.StringVarP(&f.message, "message", "m", "", "the commit message; without it, git's editor opens")
	}
	if kind != worktree.StepRebase {
		fs.BoolVar(&f.noHooks, "no-hooks", false, "run no hook")
	}
	return cmd
}

// commitPlan is a commit wt makes: of the pending changes, or a squash of
// the branch onto base.
type commitPlan struct {
	w       worktree.Worktree
	kind    worktree.StepKind // StepCommit or StepSquash
	base    string            // the merge base a squash commits onto
	n       int               // the commits a squash replaces
	initial string            // what the editor of a squash opens with
	message string
	hasMsg  bool
	pre     hook.Run
	post    hook.Run
}

// stepPlan is everything a step read and decided before its first effect.
type stepPlan struct {
	closing
	kind   worktree.StepKind
	noop   string // the line of a step with nothing to do, or ""
	commit commitPlan
	pre    hook.Run // pre-merge, for push
	post   hook.Run // post-merge, for push
}

// planStep reads everything the step needs and makes every check, in the
// order of the spec: nothing changes until it returns without error.
func (a *app) planStep(ctx context.Context, kind worktree.StepKind, target string, f stepFlags) (stepPlan, error) {
	p := stepPlan{kind: kind}
	c, err := a.readClosing(ctx, target, kind != worktree.StepCommit, "cannot run wt step in the main worktree", nil)
	if err != nil {
		return p, err
	}
	p.closing = c
	s, err := a.stepState(ctx, kind, c, f.hasMessage)
	if err != nil {
		return p, err
	}
	if b := worktree.CheckStep(kind, c.w.Branch, c.target, s); b != nil {
		return p, &Error{Code: b.Code, Msg: b.Msg, Hints: b.Hints}
	}
	if line, noop := worktree.StepNoop(kind, c.w.Branch, c.target, s); noop {
		p.noop = line
		return p, nil
	}
	if kind.Commits() && f.hasMessage && git.Blank(f.message) {
		return p, emptyMessageError()
	}

	hooks := &hookPlanner{a: a, ctx: ctx, cfg: c.cfg, noHooks: f.noHooks, gitDir: c.main.Path}
	if kind.Commits() {
		if err := hooks.check(hook.PreCommit, hook.PostCommit); err != nil {
			return p, err
		}
		if p.commit, err = a.planCommit(ctx, hooks, c, s, kind == worktree.StepSquash, f, c.w.Path); err != nil {
			return p, err
		}
		return p, nil
	}
	if kind == worktree.StepPush {
		if err := hooks.check(hook.PreMerge, hook.PostMerge); err != nil {
			return p, err
		}
		values := worktree.HookVars(c.main, c.w, "", a.env.GOOS)
		values["target"] = c.target
		if p.pre, err = hooks.plan(hook.PreMerge, values, c.w.Path); err != nil {
			return p, err
		}
		if p.post, err = hooks.plan(hook.PostMerge, values, c.main.Path); err != nil {
			return p, err
		}
	}
	return p, nil
}

// stepState is the state the pure decision of a step needs, from what was
// read and, for a squash, the commits of the branch.
func (a *app) stepState(ctx context.Context, kind worktree.StepKind, c closing, hasMessage bool) (worktree.StepState, error) {
	s := worktree.StepState{
		MergeState:  c.state,
		Pending:     c.state.Changes,
		Tracked:     c.tracked,
		InTarget:    c.inTarget,
		MessageFlag: hasMessage,
		Terminal:    a.env.StdinTTY && !a.json,
		DryRun:      a.dryRun,
	}
	if kind == worktree.StepSquash && s.TargetFound && !s.TargetIsOwn && s.InProgress == "" && !s.Detached {
		n, err := git.CountCommits(ctx, a.git, c.w.Path, c.old, c.w.Head)
		if err != nil {
			return s, gitError(err)
		}
		s.Own = n
	}
	return s, nil
}

// planCommit plans the commit of c's worktree, or its squash onto the
// target, with its hooks; post-commit runs in postDir.
func (a *app) planCommit(ctx context.Context, hooks *hookPlanner, c closing, s worktree.StepState, squash bool, f stepFlags, postDir string) (commitPlan, error) {
	cp := commitPlan{w: c.w, kind: worktree.StepCommit, message: f.message, hasMsg: f.hasMessage}
	if squash && !worktree.SquashAsCommit(s) {
		cp.kind, cp.n = worktree.StepSquash, s.Own
		base, err := git.MergeBase(ctx, a.git, c.w.Path, c.old, c.w.Head)
		if err != nil {
			return cp, gitError(err)
		}
		cp.base = base
		if !f.hasMessage && !a.dryRun {
			msgs, err := git.Messages(ctx, a.git, c.w.Path, base, c.w.Head)
			if err != nil {
				return cp, gitError(err)
			}
			cp.initial = worktree.SquashMessage(c.w.Branch, msgs)
		}
	}
	values := worktree.HookVars(c.main, c.w, "", a.env.GOOS)
	var err error
	if cp.pre, err = hooks.plan(hook.PreCommit, values, c.w.Path); err != nil {
		return cp, err
	}
	if cp.post, err = hooks.plan(hook.PostCommit, values, postDir); err != nil {
		return cp, err
	}
	return cp, nil
}

func emptyMessageError() *Error {
	return &Error{Code: ExitError, Msg: "empty commit message; nothing committed"}
}

// makeCommit runs pre-commit and makes the commit of cp, printing its
// line, and returns the branch's new HEAD. When the commit fails or is
// interrupted, the branch and the index go back to what they were.
// post-commit is the caller's to start.
func (a *app) makeCommit(ctx context.Context, cp commitPlan, interrupts <-chan os.Signal) (string, error) {
	nothing := fmt.Sprintf("interrupted: nothing was committed to %s", cp.w.Branch)
	if err := a.runBlocking(cp.pre); err != nil {
		return "", err
	}
	if pending(interrupts) {
		return "", &Error{Code: ExitInterrupted, Msg: nothing}
	}
	dir := cp.w.Path
	snap, err := git.TakeSnapshot(ctx, a.git, dir)
	if err != nil {
		return "", gitError(err)
	}
	squash := cp.kind == worktree.StepSquash
	commitErr := func() error {
		if squash {
			if err := git.ResetSoft(ctx, a.git, dir, cp.base); err != nil {
				return err
			}
		}
		if err := git.StageAll(ctx, a.git, dir); err != nil {
			return err
		}
		if cp.hasMsg {
			return git.Commit(ctx, a.git, dir, cp.message, squash)
		}
		t := git.Terminal{Env: shell.ChildEnviron(a.env.Environ, a.env.GOOS), Stdin: a.env.Stdin, Stdout: a.env.Stdout, Stderr: a.env.Stderr}
		return git.CommitEdit(ctx, a.git, t, dir, cp.initial, squash)
	}()
	// The interrupt first: git stopped because of it.
	interrupted := pending(interrupts)
	if commitErr != nil || interrupted {
		if err := snap.Restore(ctx, a.git, dir, squash); err != nil {
			return "", &Error{Code: ExitError, Msg: fmt.Sprintf("cannot restore %s: %s", cp.w.Branch, firstGitLine(err)),
				Hints: []string{fmt.Sprintf("run \"git reset --soft %s\" in %s to put the branch back", snap.Head, dir)}}
		}
		switch {
		case interrupted:
			return "", &Error{Code: ExitInterrupted, Msg: nothing}
		case errors.Is(commitErr, git.ErrEmptyMessage):
			return "", emptyMessageError()
		}
		return "", &Error{Code: ExitError, Msg: "cannot commit: " + firstGitLine(commitErr), Hints: []string{"the branch and the index are as they were"}}
	}
	head, err := git.ResolveCommit(ctx, a.git, dir, "HEAD")
	if err != nil {
		return "", gitError(err)
	}
	short := head
	if len(short) > 7 {
		short = short[:7]
	}
	if out, err := a.git.Run(ctx, dir, "rev-parse", "--short", head); err == nil {
		short = strings.TrimSpace(string(out))
	}
	return head, a.printLine(worktree.StepLine(cp.kind, cp.w.Branch, "", short, cp.n, false))
}

// step carries out the plan of one step.
func (a *app) step(ctx context.Context, p stepPlan, interrupts <-chan os.Signal) error {
	head := p.w.Head
	if p.kind == worktree.StepPush {
		head = p.old
	}
	if p.noop != "" {
		if a.json {
			return a.writeJSON(stepDocument(p, false, &head))
		}
		return a.printLine(p.noop)
	}
	if a.dryRun {
		return a.previewStep(p)
	}
	var err error
	switch p.kind {
	case worktree.StepCommit, worktree.StepSquash:
		if head, err = a.makeCommit(ctx, p.commit, interrupts); err != nil {
			return err
		}
		a.background(p.commit.post)
	case worktree.StepRebase:
		if head, err = a.rebaseStep(ctx, p.closing, interrupts, "wt step rebase"); err != nil {
			return err
		}
	case worktree.StepPush:
		if err := a.runBlocking(p.pre); err != nil {
			return err
		}
		if pending(interrupts) {
			return &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %s was not moved", p.target)}
		}
		if head, err = a.moveTarget(ctx, p.closing, p.w.Head); err != nil {
			return err
		}
		a.background(p.post)
	}
	if a.json {
		return a.writeJSON(stepDocument(p, true, &head))
	}
	return nil
}

// rebaseStep rebases c's branch onto its target and prints its line, as
// the second step of wt merge does, and returns the branch's new HEAD.
// again names the command to run once a conflict is resolved.
func (a *app) rebaseStep(ctx context.Context, c closing, interrupts <-chan os.Signal, again string) (string, error) {
	branch := c.w.Branch
	outcome, detail := a.rebaseOnto(ctx, c.w.Path, c.old)
	// The interrupt first: git stopped because of it.
	if pending(interrupts) {
		return "", &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %s was not rebased onto %s", branch, c.target)}
	}
	switch outcome {
	case worktree.Conflict:
		return "", &Error{
			Code:  ExitConflict,
			Msg:   fmt.Sprintf("conflict rebasing %s onto %s; rebase aborted", branch, c.target),
			Hints: []string{fmt.Sprintf("run \"git rebase %s\" in %s to resolve it, then %s again", c.target, c.w.Path, again)},
		}
	case worktree.Failed:
		return "", &Error{Code: ExitError, Msg: fmt.Sprintf("cannot rebase %s onto %s: %s", branch, c.target, detail)}
	}
	head, err := git.ResolveCommit(ctx, a.git, c.w.Path, "HEAD")
	if err != nil {
		return "", gitError(err)
	}
	return head, a.printLine(worktree.MergeLine(worktree.MergeRebased, branch, c.target, false))
}

// previewStep prints what the step would do, from the same plan.
func (a *app) previewStep(p stepPlan) error {
	if a.json {
		var head *string
		if p.kind == worktree.StepPush {
			h := p.w.Head
			head = &h
		}
		return a.writeJSON(stepDocument(p, true, head))
	}
	var b strings.Builder
	switch p.kind {
	case worktree.StepCommit, worktree.StepSquash:
		previewCommit(&b, p.commit)
	case worktree.StepRebase:
		b.WriteString(worktree.StepLine(p.kind, p.w.Branch, p.target, "", 0, true) + "\n")
	case worktree.StepPush:
		previewHook(&b, p.pre)
		b.WriteString(worktree.StepLine(p.kind, p.w.Branch, p.target, "", 0, true) + "\n")
		previewHook(&b, p.post)
	}
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

// previewCommit writes the --dry-run lines of a commit: pre-commit, the
// commit and post-commit.
func previewCommit(b *strings.Builder, cp commitPlan) {
	previewHook(b, cp.pre)
	b.WriteString(worktree.StepLine(cp.kind, cp.w.Branch, "", "", cp.n, true) + "\n")
	previewHook(b, cp.post)
}

type stepJSON struct {
	Schema  string  `json:"schema"`
	Step    string  `json:"step"`
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Branch  string  `json:"branch"`
	Target  string  `json:"target"`
	Changed bool    `json:"changed"`
	Head    *string `json:"head"`
}

// stepDocument is wt.step.v1 for p: head is the branch's commit afterwards,
// or for push the target's; null under --dry-run when the step would make
// it.
func stepDocument(p stepPlan, changed bool, head *string) stepJSON {
	return stepJSON{
		Schema: "wt.step.v1", Step: p.kind.String(), Name: p.w.Name, Path: p.w.Path,
		Branch: p.w.Branch, Target: p.target, Changed: changed, Head: head,
	}
}

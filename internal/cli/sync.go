package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const syncLong = `Rebase one or more worktrees onto their base.

With no name, the current worktree. With names, those worktrees, named as
"wt remove" takes them: ^ for the main worktree, @ for the current one,
<repo>:<name> for one of another repository. With --all, every linked
worktree of the current repository whose HEAD is not detached; the main
worktree only when named with ^.

The base is --base when given, otherwise the repository's: default_base, or
its default branch (see "wt config --help"). When the base is on a remote,
as origin/main, that remote is fetched once per repository before the
first rebase; --no-fetch fetches nothing.

wt sync never stashes. It skips, without touching them, a worktree with
changes to tracked files, with a detached HEAD, with a rebase, merge,
cherry-pick or revert in progress, or whose directory is gone; untracked
files do not stop a rebase. A worktree that already contains its base is
left as it is. A rebase that stops on a conflict is aborted, so the
worktree is left as it was, and wt sync goes on with the next one.

Rebasing rewrites the branch's commits: a branch already pushed diverges
from its upstream. wt sync never pushes.

Each worktree gets one line: "rebased", "is up to date with", "skipped",
"conflict in" or "failed". When one is not synced, a summary follows on
stderr with a hint for each, and the exit code is 1 when a rebase failed,
otherwise 6 on a conflict, otherwise 5.`

type syncFlags struct {
	all     bool
	base    string
	noFetch bool
}

func (a *app) syncCommand() *cobra.Command {
	var f syncFlags
	cmd := &cobra.Command{
		Use:   "sync [<name>...] [--all]",
		Short: "Rebase worktrees onto their base",
		Long:  syncLong,
		Args: func(cmd *cobra.Command, args []string) error {
			if f.all && len(args) > 0 {
				return usageError(cmd, "--all cannot be used with a <name>")
			}
			if cmd.Flags().Changed("base") && strings.TrimSpace(f.base) == "" {
				return usageError(cmd, "--base cannot be empty")
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if f.all {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			complete := a.completeTargets(modeWorktree, func(w worktree.Worktree) bool {
				return !w.Bare && !w.Detached && !slices.Contains(args, w.Name)
			})
			return complete(cmd, nil, toComplete)
		},
		RunE: action(func(cmd *cobra.Command, args []string) error {
			// A Ctrl-C does not end wt: it reaches the git that is running,
			// and wt starts no other fetch or rebase once that one ends.
			var interrupts chan os.Signal
			if !a.dryRun {
				interrupts = make(chan os.Signal, 1)
				signal.Notify(interrupts, os.Interrupt)
				defer signal.Stop(interrupts)
			}
			return a.sync(cmd.Context(), args, f, interrupts)
		}),
	}
	fs := cmd.Flags()
	fs.BoolVar(&f.all, "all", false, "sync every linked worktree of the current repository")
	fs.StringVar(&f.base, "base", "", "rebase onto `ref` instead of each repository's base")
	fs.BoolVar(&f.noFetch, "no-fetch", false, "fetch nothing")
	_ = cmd.RegisterFlagCompletionFunc("base", a.completeBases)
	return cmd
}

// syncRepo is a repository wt sync works in: its base, resolved once, so
// that all of its worktrees are rebased onto the same commit.
type syncRepo struct {
	gitDir string // where git runs for the repository
	cfg    *config.Config
	main   worktree.Worktree
	name   string // display name, as its worktrees' labels and JSON show it
	other  bool   // not the working directory's repository
	base   string // as given or configured
	commit string
	fetch  string // the remote fetched, or ""
}

// syncTarget is one worktree to sync.
type syncTarget struct {
	label string
	w     worktree.Worktree
	repo  *syncRepo
}

// syncResult is how one worktree ended.
type syncResult struct {
	target  syncTarget
	outcome worktree.SyncOutcome
	detail  string // the reason of a skip, git's message of a failure
	head    string // HEAD afterwards
}

// sync runs wt sync in three phases, so that nothing is rebased until every
// check passed: the targets, then each repository's base (fetched and
// resolved), then the worktrees one at a time.
func (a *app) sync(ctx context.Context, names []string, f syncFlags, interrupts <-chan os.Signal) error {
	dir, err := a.workdir()
	if err != nil {
		return err
	}
	targets, repos, err := a.syncTargets(ctx, dir, names, f.all)
	if err != nil {
		return err
	}
	var fetched []string
	for _, r := range repos {
		if pending(interrupts) {
			return &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %d of %d worktrees not synced", len(targets), len(targets))}
		}
		if err := a.syncBase(ctx, r, f); err != nil {
			return err
		}
		if r.fetch != "" {
			fetched = append(fetched, r.fetch)
		}
	}

	if a.dryRun && !a.json {
		for _, remote := range fetched {
			if _, err := fmt.Fprintf(a.env.Stdout, "would fetch %s\n", remote); err != nil {
				return err
			}
		}
	}
	results := make([]syncResult, 0, len(targets))
	interrupted := false
	for _, t := range targets {
		if interrupted = pending(interrupts); interrupted {
			break
		}
		r := a.syncOne(ctx, t)
		results = append(results, r)
		if !a.json {
			if _, err := fmt.Fprintln(a.env.Stdout, worktree.SyncLine(t.label, t.repo.base, r.outcome, r.detail, a.dryRun)); err != nil {
				return err
			}
		}
	}
	interrupted = interrupted || pending(interrupts)
	if err := syncReport(results, len(targets), interrupted); err != nil {
		return err
	}
	if a.json {
		return a.writeJSON(syncDocument(results, a.dryRun))
	}
	return nil
}

// syncTargets resolves what wt sync syncs: the current worktree, the
// worktrees named, or with all the linked worktrees of the current
// repository. It returns them in order, and their repositories in the order
// they first appear.
func (a *app) syncTargets(ctx context.Context, dir string, names []string, all bool) ([]syncTarget, []*syncRepo, error) {
	var targets []syncTarget
	var repos []*syncRepo
	byMain := map[string]*syncRepo{}
	seen := map[string]bool{}
	var displayNames *workspaceNames
	repoOf := func(main worktree.Worktree, gitDir string, cfg *config.Config, other bool) *syncRepo {
		if r, ok := byMain[main.Path]; ok {
			return r
		}
		if displayNames == nil {
			displayNames = &workspaceNames{}
			if base, err := a.loadConfigQuiet(""); err == nil && len(reposRoot(base)) > 0 {
				*displayNames = newWorkspaceNames(a.discoverRepos(base, func(string) {}), a.env.GOOS)
			}
		}
		r := &syncRepo{
			gitDir: gitDir, cfg: cfg, main: main, other: other,
			name: displayNames.of(workspace.Repo{Path: main.Path, Name: filepath.Base(main.Path)}),
		}
		byMain[main.Path] = r
		repos = append(repos, r)
		return r
	}
	add := func(w worktree.Worktree, r *syncRepo) {
		if seen[w.Path] {
			return
		}
		seen[w.Path] = true
		label := worktree.Label("", w)
		if r.other {
			label = worktree.Label(r.name, w)
		}
		targets = append(targets, syncTarget{label: label, w: w, repo: r})
	}

	if all {
		repo, err := a.loadRepository(ctx, dir)
		if err != nil {
			return nil, nil, gitError(err)
		}
		cfg, err := a.loadConfig(repo.root())
		if err != nil {
			return nil, nil, err
		}
		ws := worktree.SyncTargets(repo.worktrees)
		if len(ws) == 0 {
			return nil, nil, nil
		}
		r := repoOf(mainWorktree(repo), dir, cfg, false)
		for _, w := range ws {
			add(w, r)
		}
		return targets, repos, nil
	}

	if len(names) == 0 {
		names = []string{"@"}
	}
	for _, name := range names {
		res, cfg, err := a.resolve(ctx, dir, name, modeWorktree)
		if err != nil {
			return nil, nil, err
		}
		add(*res.Worktree, repoOf(mainWorktree(res.Repo), res.GitDir, cfg, res.Other))
	}
	return targets, repos, nil
}

// syncBase decides r's base, fetches its remote once, and resolves it to a
// commit. A base that cannot be used fails with exit 3 before any rebase.
func (a *app) syncBase(ctx context.Context, r *syncRepo, f syncFlags) error {
	base := f.base
	if base == "" {
		base = configString(r.cfg, "default_base")
	}
	if base == "" {
		var err error
		if base, err = a.defaultBranch(ctx, r.gitDir, r.main); err != nil {
			return err
		}
	}
	r.base = base
	if !f.noFetch {
		remotes, err := git.Remotes(ctx, a.git, r.gitDir)
		if err != nil {
			return gitError(err)
		}
		r.fetch, _ = worktree.FetchRemote(base, remotes)
	}
	if r.fetch != "" && !a.dryRun {
		if err := git.Fetch(ctx, a.git, r.gitDir, r.fetch); err != nil {
			a.warn(fmt.Sprintf("cannot fetch %s (%s); using the references already here", r.fetch, firstGitLine(err)))
		}
	}
	commit, err := git.ResolveCommit(ctx, a.git, r.gitDir, base)
	if errors.Is(err, git.ErrNoRevision) {
		return &Error{Code: ExitNotFound, Msg: "base not found: " + base}
	}
	if err != nil {
		return gitError(err)
	}
	r.commit = commit
	return nil
}

// syncOne syncs one worktree: it reads its state, decides, and rebases when
// it has to. It never leaves a rebase in progress that it could abort.
func (a *app) syncOne(ctx context.Context, t syncTarget) syncResult {
	res := syncResult{target: t, head: t.w.Head}
	fail := func(err error) syncResult {
		res.outcome, res.detail = worktree.Failed, firstGitLine(err)
		return res
	}

	w := t.w
	state := worktree.SyncState{Path: w.Path, Detached: w.Detached || w.Branch == ""}
	if info, err := os.Stat(w.Path); err != nil || !info.IsDir() {
		state.Missing = true
	}
	if !state.Missing {
		var err error
		if state.InProgress, err = git.InProgress(ctx, a.git, w.Path); err != nil {
			return fail(err)
		}
	}
	if !state.Missing && !state.Detached && state.InProgress == "" {
		var err error
		if state.Dirty, err = git.TrackedChanges(ctx, a.git, w.Path); err != nil {
			return fail(err)
		}
		if state.Contains, err = git.IsAncestor(ctx, a.git, w.Path, t.repo.commit, w.Head); err != nil {
			return fail(err)
		}
	}
	action, reason := worktree.DecideSync(state)
	switch action {
	case worktree.SyncSkip:
		res.outcome, res.detail = worktree.Skipped, reason
		return res
	case worktree.SyncUpToDate:
		res.outcome = worktree.UpToDate
		return res
	}
	if a.dryRun {
		res.outcome = worktree.Rebased
		return res
	}

	res.outcome, res.detail = a.rebaseOnto(ctx, w.Path, t.repo.commit)
	if res.outcome == worktree.Rebased {
		if head, err := git.ResolveCommit(ctx, a.git, w.Path, "HEAD"); err == nil {
			res.head = head
		}
	}
	return res
}

// rebaseOnto rebases the branch checked out in dir onto commit and, when
// git stops, aborts the rebase left in progress, so that the worktree is as
// it was. It returns Rebased, Conflict, or Failed with git's message; a
// rebase that cannot be aborted is a failure that says how to finish it.
func (a *app) rebaseOnto(ctx context.Context, dir, commit string) (worktree.SyncOutcome, string) {
	rebaseErr := git.Rebase(ctx, a.git, dir, commit)
	if rebaseErr == nil {
		return worktree.Rebased, ""
	}
	outcome, detail := worktree.Failed, firstGitLine(rebaseErr)
	if unmerged, err := git.Unmerged(ctx, a.git, dir); err == nil && unmerged {
		outcome, detail = worktree.Conflict, ""
	}
	if op, err := git.InProgress(ctx, a.git, dir); err == nil && op == "rebase" {
		if err := git.RebaseAbort(ctx, a.git, dir); err != nil {
			outcome = worktree.Failed
			detail = fmt.Sprintf("cannot abort the rebase (%s); run \"git rebase --abort\" in %s", firstGitLine(err), dir)
		}
	}
	return outcome, detail
}

// syncReport is the outcome of wt sync once the loop is over: nil when every
// worktree was rebased or up to date; otherwise a summary with one hint per
// worktree that was not synced.
func syncReport(results []syncResult, planned int, interrupted bool) error {
	var hints []string
	outcomes := make([]worktree.SyncOutcome, 0, len(results))
	synced := 0
	for _, r := range results {
		outcomes = append(outcomes, r.outcome)
		switch r.outcome {
		case worktree.Rebased, worktree.UpToDate:
			synced++
		default:
			t := r.target
			hints = append(hints, worktree.SyncHint(t.label, t.repo.base, t.w.Path, r.outcome, r.detail))
		}
	}
	if interrupted {
		return &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %d of %d worktrees not synced", planned-synced, planned), Hints: hints}
	}
	if code := worktree.SyncExit(outcomes); code != 0 {
		return &Error{Code: code, Msg: fmt.Sprintf("%d of %d worktrees not synced", len(results)-synced, len(results)), Hints: hints}
	}
	return nil
}

type syncWorktreeJSON struct {
	Name   string `json:"name"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
	Result string `json:"result"`
	Head   string `json:"head"`
}

type syncJSON struct {
	Schema    string             `json:"schema"`
	Worktrees []syncWorktreeJSON `json:"worktrees"`
}

// syncDocument is wt.sync.v1, for a sync where every worktree was rebased
// or up to date.
func syncDocument(results []syncResult, dryRun bool) syncJSON {
	doc := syncJSON{Schema: "wt.sync.v1", Worktrees: make([]syncWorktreeJSON, 0, len(results))}
	for _, r := range results {
		result := "up_to_date"
		if r.outcome == worktree.Rebased {
			result = "rebased"
			if dryRun {
				result = "would_rebase"
			}
		}
		t := r.target
		doc.Worktrees = append(doc.Worktrees, syncWorktreeJSON{
			Name: t.w.Name, Repo: t.repo.name, Path: t.w.Path, Branch: t.w.Branch,
			Base: t.repo.base, Result: result, Head: r.head,
		})
	}
	return doc
}

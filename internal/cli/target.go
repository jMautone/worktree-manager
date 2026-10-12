package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// worktreeError maps the errors of worktree.Resolve to exit codes.
func worktreeError(err error) error {
	var nf *worktree.NotFoundError
	var amb *worktree.AmbiguousError
	switch {
	case errors.As(err, &nf):
		return &Error{Code: ExitNotFound, Msg: nf.Error(), Hints: []string{hintList}}
	case errors.As(err, &amb):
		var hints []string
		for _, c := range amb.Candidates {
			hints = append(hints, fmt.Sprintf("%s (%s)", c.Path, describeCheckout(c)))
		}
		return &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
	case errors.Is(err, worktree.ErrNoCurrent):
		return &Error{Code: ExitNotFound, Msg: err.Error()}
	}
	return err
}

// workspaceNames is the repositories under the roots with their display
// names, for the messages that name them.
type workspaceNames struct {
	repos []workspace.Repo
	names []string // workspace.DisplayNames of repos
}

func newWorkspaceNames(repos []workspace.Repo, goos string) workspaceNames {
	return workspaceNames{repos, workspace.DisplayNames(repos, goos)}
}

// of is the display name of the repository at path, or its name when it is
// not among the repositories.
func (n workspaceNames) of(r workspace.Repo) string {
	for i, x := range n.repos {
		if x.Path == r.Path {
			return n.names[i]
		}
	}
	return r.Name
}

// repoAmbiguity maps a name that matches more than one repository to exit
// 4, with one hint per candidate: its display name, as the repository side
// of a qualified target, and its path.
func repoAmbiguity(amb *workspace.AmbiguousError, n workspaceNames) *Error {
	hints := make([]string, 0, len(amb.Candidates))
	for _, c := range amb.Candidates {
		hints = append(hints, n.of(c)+":  "+c.Path)
	}
	return &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
}

// othersAmbiguity maps a name that matches linked worktrees of more than
// one other repository to exit 4, with one hint per candidate: its
// qualified form and its path. repos are the repositories the hits' groups
// index.
func othersAmbiguity(amb *worktree.OthersAmbiguousError, repos []workspace.Repo, n workspaceNames) *Error {
	hints := make([]string, 0, len(amb.Hits))
	for _, h := range amb.Hits {
		hints = append(hints, n.of(repos[h.Group])+":"+h.Worktree.Name+"  "+h.Worktree.Path)
	}
	return &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
}

const (
	hintList     = "run 'wt list' to see the worktrees"
	hintListAll  = "run 'wt list --all-repos' to see the worktrees of every repository"
	hintRepos    = "run 'wt repos' to see the repositories"
	hintSetRoots = "set repos_root to reach the repositories from anywhere; see 'wt repos -h'"
)

// nameNotFound is the error when an unqualified name matched nothing.
// notRepo is the reason there is no current repository, nil inside one;
// roots reports whether repos_root names any root. In modeNavigate the
// repositories were searched too.
func nameNotFound(target string, notRepo *git.NotRepoError, roots bool, mode resolveMode) *Error {
	switch {
	case notRepo != nil && !roots:
		return &Error{Code: ExitNotFound, Msg: notRepo.Error(), Hints: []string{hintSetRoots}}
	case !roots:
		return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree named %q", target), Hints: []string{hintList}}
	case mode == modeNavigate:
		return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree or repository named %q", target), Hints: []string{hintListAll, hintRepos}}
	}
	return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree named %q", target), Hints: []string{hintListAll}}
}

// qualifiedNotFound is the error when the name side of a qualified target
// matched no worktree of its repository, named by its display name.
func qualifiedNotFound(name, repo string) *Error {
	return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree named %q in %s", name, repo), Hints: []string{hintListAll}}
}

// repoNotFound is the error when the repository side of a qualified target
// matched no repository.
func repoNotFound(nf *workspace.NotFoundError) *Error {
	return &Error{Code: ExitNotFound, Msg: nf.Error(), Hints: []string{hintRepos}}
}

// qualifiedUsage checks what a qualified target can be told wrong about
// without reading anything: an empty repository side, and @ or - as the
// name side, which refer to the shell, not to a repository.
func qualifiedUsage(target, repo, name string) *Error {
	switch {
	case repo == "":
		return &Error{Code: ExitUsage, Msg: fmt.Sprintf("missing repository before \":\" in %q", target)}
	case name == "@" || name == "-":
		return &Error{Code: ExitUsage, Msg: fmt.Sprintf("%q: @ and - cannot follow a repository", target)}
	}
	return nil
}

// noRootsError is the error of a command that needs repos_root when it
// names no root.
func (a *app) noRootsError() *Error {
	userFile, _ := a.userFile()
	return &Error{
		Code:  ExitError,
		Msg:   "no repository roots configured",
		Hints: []string{fmt.Sprintf(`set repos_root in %s, e.g. repos_root = ["~/src"]`, userFile)},
	}
}

// describeCheckout tells candidates with the same name apart in a hint.
func describeCheckout(w worktree.Worktree) string {
	switch {
	case w.Bare:
		return "bare"
	case w.Detached:
		return "detached"
	}
	return "branch " + w.Branch
}

// Filters of the worktrees a command's completion offers.
var (
	// canEnter: wt cd and wt path, whose destination must exist.
	canEnter = func(w worktree.Worktree) bool { return !w.Prunable }
	// canRemove: wt remove and wt lock, including the worktrees whose
	// directory is gone.
	canRemove = func(w worktree.Worktree) bool { return !w.Main && !w.Locked }
	// isLocked: wt unlock.
	isLocked = func(w worktree.Worktree) bool { return w.Locked }
)

// completeTargets offers, for a command's first argument, what resolve
// would accept in mode, in the order it would try them and each name once:
//
//  1. the NAME of the worktrees of the current repository that filter
//     accepts, described as "worktree";
//  2. in modeNavigate, the name of the repositories under the roots that
//     start with the word, ignoring case, described as "repository";
//  3. the NAME of the linked worktrees of the other repositories that
//     filter accepts, described as "worktree of <display name>".
//
// A word with ":" offers instead <repository side as typed>:<NAME> for the
// worktrees of that repository, other than its main one, that filter
// accepts. Like every completion it never writes to the terminal: warnings
// are dropped, and an error only leaves out the names it would have found.
func (a *app) completeTargets(mode resolveMode, filter func(worktree.Worktree) bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		dir, err := a.workdir()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctx := cmd.Context()
		repo, _ := a.loadRepository(ctx, dir)
		var repos []workspace.Repo
		if base, err := a.loadConfigQuiet(""); err == nil && len(reposRoot(base)) > 0 {
			repos = a.discoverRepos(base, func(string) {})
		}

		if side, _, qualified := workspace.SplitQualified(toComplete); qualified {
			r, err := workspace.MatchSide(repos, side, a.env.GOOS)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			target, _, _, err := a.readRepo(ctx, dir, repo, repos, r)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var names []string
			for _, w := range target.worktrees {
				if !w.Main && filter(w) {
					names = append(names, side+":"+w.Name)
				}
			}
			return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
		}

		var offered []cobra.Completion
		seen := map[string]bool{}
		offer := func(name, desc string) {
			if !seen[name] {
				seen[name] = true
				offered = append(offered, cobra.CompletionWithDesc(name, desc))
			}
		}
		if repo != nil {
			for _, w := range repo.worktrees {
				if filter(w) && strings.HasPrefix(w.Name, toComplete) {
					offer(w.Name, "worktree")
				}
			}
		}
		if len(repos) == 0 {
			return offered, cobra.ShellCompDirectiveNoFileComp
		}
		if mode == modeNavigate {
			for _, r := range repos {
				if workspace.HasPrefixFold(r.Name, toComplete) {
					offer(r.Name, "repository")
				}
			}
		}
		current := currentRepo(repo, repos)
		others := make([]workspace.Repo, 0, len(repos))
		for i, r := range repos {
			if i != current {
				others = append(others, r)
			}
		}
		scanned, err := a.scanWorktrees(ctx, others, -1, "")
		if err != nil {
			return offered, cobra.ShellCompDirectiveNoFileComp
		}
		names := newWorkspaceNames(repos, a.env.GOOS)
		for _, s := range scanned {
			for _, w := range s.Worktrees {
				if !w.Main && !w.Bare && filter(w) && strings.HasPrefix(w.Name, toComplete) {
					offer(w.Name, "worktree of "+names.of(s.Repo))
				}
			}
		}
		return offered, cobra.ShellCompDirectiveNoFileComp
	}
}

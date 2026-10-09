package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// resolveTarget resolves <target> among the worktrees of repo as wt cd
// does: ^, @, a name and then a branch. `-` is wt cd's own; here it is a
// name like any other.
func (a *app) resolveTarget(repo *repository, target string) (worktree.Worktree, error) {
	w, err := worktree.Resolve(repo.worktrees, target)
	return w, worktreeError(err)
}

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

// repoAmbiguity maps a name that matches more than one repository to exit
// 4, with one hint per candidate.
func repoAmbiguity(amb *workspace.AmbiguousError) *Error {
	hints := make([]string, 0, len(amb.Candidates))
	for _, c := range amb.Candidates {
		hints = append(hints, c.Path)
	}
	return &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
}

const (
	hintList  = "run 'wt list' to see the worktrees"
	hintRepos = "run 'wt repos' to see the repositories"
)

// nameNotFound is the error of wt cd when a name matched neither a worktree
// of the current repository nor a repository under the roots. notRepo is
// the reason there is no current repository, nil inside one; roots reports
// whether repos_root names any root.
func nameNotFound(target string, notRepo *git.NotRepoError, roots bool) *Error {
	switch {
	case notRepo == nil && !roots:
		return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree named %q", target), Hints: []string{hintList}}
	case notRepo == nil:
		return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no worktree or repository named %q", target), Hints: []string{hintList, hintRepos}}
	case roots:
		return &Error{Code: ExitNotFound, Msg: fmt.Sprintf("no repository named %q", target), Hints: []string{hintRepos}}
	}
	return &Error{
		Code:  ExitNotFound,
		Msg:   notRepo.Error(),
		Hints: []string{"set repos_root to jump to repositories from anywhere; see 'wt repos -h'"},
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

// completeNames offers, for a command's first argument, the NAME of every
// worktree of the repository that filter accepts, and nothing after it.
// It loads no configuration, so typing never prints a warning, and any
// error offers nothing: a completion never writes to the terminal.
func (a *app) completeNames(filter func(worktree.Worktree) bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		dir, err := a.workdir()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		repo, err := a.loadRepository(cmd.Context(), dir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var names []string
		for _, w := range repo.worktrees {
			if filter(w) && !slices.Contains(names, w.Name) {
				names = append(names, w.Name)
			}
		}
		return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeCdTargets offers, for the argument of wt cd, the NAME of every
// worktree of the current repository whose directory exists and then the
// name of every repository under the roots that starts with the word being
// completed, ignoring letter case, leaving out names already offered. Like
// every completion it never writes to the terminal: warnings are dropped,
// and an error only leaves out the names it would have found.
func (a *app) completeCdTargets(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dir, err := a.workdir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	repo, _ := a.loadRepository(cmd.Context(), dir)
	var names []string
	if repo != nil {
		for _, w := range repo.worktrees {
			if !w.Prunable && !slices.Contains(names, w.Name) {
				names = append(names, w.Name)
			}
		}
	}
	offered := withPrefix(names, toComplete)
	cfg, err := a.loadConfigQuiet(repo.root())
	if err != nil {
		return offered, cobra.ShellCompDirectiveNoFileComp
	}
	for _, r := range a.discoverRepos(cfg, func(string) {}) {
		if workspace.HasPrefixFold(r.Name, toComplete) && !slices.Contains(offered, r.Name) {
			offered = append(offered, r.Name)
		}
	}
	return offered, cobra.ShellCompDirectiveNoFileComp
}

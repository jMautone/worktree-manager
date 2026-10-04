package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/worktree"
)

// resolveTarget resolves <target> among the worktrees of repo as wt cd
// does: ^, @, a name and then a branch. `-` is wt cd's own; here it is a
// name like any other.
func (a *app) resolveTarget(repo *repository, target string) (worktree.Worktree, error) {
	w, err := worktree.Resolve(repo.worktrees, target)
	var nf *worktree.NotFoundError
	var amb *worktree.AmbiguousError
	switch {
	case errors.As(err, &nf):
		return w, &Error{Code: ExitNotFound, Msg: nf.Error(), Hints: []string{"run 'wt list' to see the worktrees"}}
	case errors.As(err, &amb):
		var hints []string
		for _, c := range amb.Candidates {
			hints = append(hints, fmt.Sprintf("%s (%s)", c.Path, describeCheckout(c)))
		}
		return w, &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
	case errors.Is(err, worktree.ErrNoCurrent):
		return w, &Error{Code: ExitNotFound, Msg: err.Error()}
	}
	return w, err
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
// Like wt cd, it loads no configuration, so typing never prints a warning,
// and any error offers nothing: a completion never writes to the terminal.
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

package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const cdLong = `Move the shell to a worktree of the current repository.

<target> is one of:

  <name>   the worktree with that NAME (as in "wt list") or, if none has
           it, the worktree with that branch checked out
  ^        the main worktree
  @        the root of the current worktree
  -        the directory the shell was in before the last jump of wt

A binary cannot change the working directory of its shell, so wt cd only
works through the wt shell function; see "wt shell init --help".

In zsh with the EXTENDED_GLOB option set, ^ is a glob operator: quote it,
as in wt cd '^'. In PowerShell, @ is an operator: quote it, as in wt cd '@'.`

func (a *app) cdCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cd <target>",
		Short: "Move the shell to a worktree",
		Long:  cdLong,
		Args:  exactArgs("target"),
		// The names come from git only: no configuration is loaded, so
		// typing never prints a warning.
		ValidArgsFunction: a.completeWorktreeNames,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			if !shell.Active(a.env.Getenv) {
				return &Error{
					Code:  ExitError,
					Msg:   "shell integration is not active",
					Hints: []string{"load it with the line for your shell from 'wt shell init --help'"},
				}
			}
			dest, err := a.cdDestination(cmd, dir, args[0])
			if err != nil {
				return err
			}
			if info, err := os.Stat(dest); err != nil || !info.IsDir() {
				return &Error{Code: ExitNotFound, Msg: "directory does not exist: " + dest}
			}
			switch {
			case a.dryRun && !a.json:
				_, err := fmt.Fprintf(a.env.Stdout, "would change directory to %s\n", dest)
				return err
			case !a.dryRun:
				if err := shell.WriteDirective(a.env.Getenv(shell.DirectiveVar), dest); err != nil {
					return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot write the directive file: %v", err)}
				}
			}
			if a.json {
				return a.writeJSON(struct {
					Schema string `json:"schema"`
					Path   string `json:"path"`
				}{"wt.cd.v1", dest})
			}
			return nil
		}),
	}
}

// cdDestination resolves target to a directory. `-` is the shell session's
// previous directory and needs no repository, so -C does not change it.
func (a *app) cdDestination(cmd *cobra.Command, dir, target string) (string, error) {
	if target == "-" {
		if _, err := a.loadConfig(""); err != nil {
			return "", err
		}
		prev := a.env.Getenv(shell.PreviousVar)
		if prev == "" {
			return "", &Error{Code: ExitNotFound, Msg: "no previous directory"}
		}
		return prev, nil
	}
	repo, err := a.requireRepository(cmd.Context(), dir)
	if err != nil {
		return "", err
	}
	if _, err := a.loadConfig(repo.root()); err != nil {
		return "", err
	}
	w, err := worktree.Resolve(repo.worktrees, target)
	var nf *worktree.NotFoundError
	var amb *worktree.AmbiguousError
	switch {
	case errors.As(err, &nf):
		return "", &Error{Code: ExitNotFound, Msg: nf.Error(), Hints: []string{"run 'wt list' to see the worktrees"}}
	case errors.As(err, &amb):
		var hints []string
		for _, c := range amb.Candidates {
			hints = append(hints, fmt.Sprintf("%s (%s)", c.Path, describeCheckout(c)))
		}
		return "", &Error{Code: ExitAmbiguous, Msg: amb.Error(), Hints: hints}
	case errors.Is(err, worktree.ErrNoCurrent):
		return "", &Error{Code: ExitNotFound, Msg: err.Error()}
	case err != nil:
		return "", err
	}
	return w.Path, nil
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

// completeWorktreeNames offers the NAME of every worktree of the repository
// whose directory exists. Any error offers nothing: a completion never
// writes to the terminal.
func (a *app) completeWorktreeNames(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
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
		if !w.Prunable && !slices.Contains(names, w.Name) {
			names = append(names, w.Name)
		}
	}
	return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/shell"
)

const cdLong = `Move the shell to a worktree or a repository, from anywhere.

<target> is one of:

  <name>          the worktree of the current repository with that NAME
                  (as in "wt list") or, if none has it, with that branch
                  checked out; if neither exists, or outside a repository,
                  the repository with that name under the roots in
                  repos_root (as in "wt repos"); if none, the linked
                  worktree of another repository with that NAME or branch
                  (as in "wt list --all-repos")
  <repo>:<name>   the worktree <name> of the repository <repo>, searched
                  nowhere else: api:feat, api:^ for its main worktree, and
                  api: for the repository itself
  ^               the main worktree
  @               the root of the current worktree
  -               the directory the shell was in before the last jump of wt

Worktree names and branches match exactly. A repository name is matched
in three steps, each only if the previous one found nothing: the exact
name; the name ignoring letter case; the names that start with <name>,
ignoring letter case. A step that finds more than one is an error that
lists them (exit 4), each in the qualified form that names it. Scripts
should use full names: a prefix that is unique today may not be after the
next clone.

When two repositories have the same name, write <repo> as the end of its
path, as in GIT/api:feat; the hints of the error name each one this way.

A binary cannot change the working directory of its shell, so wt cd only
works through the wt shell function; see "wt shell init --help". "wt path"
prints the destination instead.

In zsh with the EXTENDED_GLOB option set, ^ is a glob operator: quote it,
as in wt cd '^' and wt cd 'api:^'. In PowerShell, @ is an operator: quote
it, as in wt cd '@'.`

func (a *app) cdCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "cd <target>",
		Short:             "Move the shell to a worktree or a repository",
		Long:              cdLong,
		Args:              exactArgs("target"),
		ValidArgsFunction: a.completeTargets(modeNavigate, canEnter),
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
			dest, err := a.cdDestination(cmd.Context(), dir, args[0])
			if err != nil {
				return err
			}
			if err := mustExist(dest); err != nil {
				return err
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
// Anything else is resolved as resolve does in modeNavigate.
func (a *app) cdDestination(ctx context.Context, dir, target string) (string, error) {
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
	res, _, err := a.resolve(ctx, dir, target, modeNavigate)
	return res.Path, err
}

// mustExist fails with exit 3 when dest is not a directory: a worktree
// whose directory was deleted is still listed by git.
func mustExist(dest string) error {
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		return &Error{Code: ExitNotFound, Msg: "directory does not exist: " + dest}
	}
	return nil
}

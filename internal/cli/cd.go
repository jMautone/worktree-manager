package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const cdLong = `Move the shell to a worktree of the current repository, or to one of
the repositories under the roots in repos_root.

<target> is one of:

  <name>   the worktree with that NAME (as in "wt list") or, if none has
           it, the worktree with that branch checked out; if neither
           exists, or outside a repository, the repository with that name
           under the roots in repos_root (as in "wt repos")
  ^        the main worktree
  @        the root of the current worktree
  -        the directory the shell was in before the last jump of wt

Worktree names and branches match exactly. A repository name is matched
in three steps, each only if the previous one found nothing: the exact
name; the name ignoring letter case; the names that start with <name>,
ignoring letter case. A step that finds more than one is an error that
lists them (exit 4). Scripts should use full names: a prefix that is
unique today may not be after the next clone.

A binary cannot change the working directory of its shell, so wt cd only
works through the wt shell function; see "wt shell init --help".

In zsh with the EXTENDED_GLOB option set, ^ is a glob operator: quote it,
as in wt cd '^'. In PowerShell, @ is an operator: quote it, as in wt cd '@'.`

func (a *app) cdCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "cd <target>",
		Short:             "Move the shell to a worktree or a repository",
		Long:              cdLong,
		Args:              exactArgs("target"),
		ValidArgsFunction: a.completeCdTargets,
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
//
// Any other target is resolved in the current repository first, exactly as
// in a repository alone; only when that finds nothing, or there is no
// current repository, is a name looked for among the repositories under the
// roots. `^` and `@` are never looked for there.
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
	repo, err := a.loadRepository(cmd.Context(), dir)
	var notRepo *git.NotRepoError
	if err != nil && !errors.As(err, &notRepo) {
		return "", gitError(err)
	}
	cfg, err := a.loadConfig(repo.root())
	if err != nil {
		return "", err
	}
	var local error // why the current repository did not resolve target
	if repo != nil {
		w, err := worktree.Resolve(repo.worktrees, target)
		var amb *worktree.AmbiguousError
		switch {
		case err == nil:
			return w.Path, nil
		case errors.As(err, &amb):
			// An ambiguity among the worktrees is not resolved elsewhere.
			return "", worktreeError(err)
		}
		local = err
	}
	if target == "^" || target == "@" {
		if repo == nil {
			return "", gitError(notRepo)
		}
		return "", worktreeError(local)
	}
	if len(reposRoot(cfg)) == 0 {
		return "", nameNotFound(target, notRepo, false)
	}
	r, err := workspace.Match(a.discoverRepos(cfg, a.warn), target)
	var amb *workspace.AmbiguousError
	switch {
	case err == nil:
		return r.Path, nil
	case errors.As(err, &amb):
		return "", repoAmbiguity(amb)
	}
	return "", nameNotFound(target, notRepo, true)
}

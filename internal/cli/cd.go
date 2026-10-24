package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/hook"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/worktree"
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
it, as in wt cd '@'.

Hooks: wt cd runs the commands of pre-cd at the root of the destination's
worktree, and waits for them, before the shell moves; when one fails, the
shell stays and wt exits with 7. Once the shell moves, it starts those of
post-cd in the background and does not wait for them: their output goes to
a log under the repository's .git/wt/logs, whose path wt prints. A
destination inside no worktree runs neither, and --no-hooks runs neither;
see "wt config --help" for how to set them.`

func (a *app) cdCommand() *cobra.Command {
	var noHooks bool
	cmd := &cobra.Command{
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
			t, err := a.cdDestination(cmd.Context(), dir, args[0])
			if err != nil {
				return err
			}
			if err := mustExist(t.dest); err != nil {
				return err
			}
			pre, post, err := a.planCD(&hookPlanner{a: a, ctx: cmd.Context(), cfg: t.cfg, noHooks: noHooks}, t)
			if err != nil {
				return err
			}
			switch {
			case a.dryRun && !a.json:
				var b strings.Builder
				previewHook(&b, pre)
				fmt.Fprintf(&b, "would change directory to %s\n", t.dest)
				previewHook(&b, post)
				_, err := a.env.Stdout.Write([]byte(b.String()))
				return err
			case !a.dryRun:
				if err := a.runBlocking(pre); err != nil {
					return err
				}
				if err := shell.WriteDirective(a.env.Getenv(shell.DirectiveVar), t.dest); err != nil {
					return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot write the directive file: %v", err)}
				}
			}
			if a.json {
				if err := a.writeJSON(struct {
					Schema string `json:"schema"`
					Path   string `json:"path"`
				}{"wt.cd.v1", t.dest}); err != nil {
					return err
				}
			}
			if !a.dryRun {
				a.background(post)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&noHooks, "no-hooks", false, "run no hook")
	return cmd
}

// cdTarget is where wt cd goes: the directory, the configuration, and,
// when resolving the target already read them, the worktree it is and its
// repository.
type cdTarget struct {
	dest string
	cfg  *config.Config
	w    *worktree.Worktree // nil when the target is a repository or -
	repo *repository        // w's repository
}

// cdDestination resolves target to a directory. `-` is the shell session's
// previous directory and needs no repository, so -C does not change it.
// Anything else is resolved as resolve does in modeNavigate.
func (a *app) cdDestination(ctx context.Context, dir, target string) (cdTarget, error) {
	if target == "-" {
		cfg, err := a.loadConfig("")
		if err != nil {
			return cdTarget{}, err
		}
		prev := a.env.Getenv(shell.PreviousVar)
		if prev == "" {
			return cdTarget{}, &Error{Code: ExitNotFound, Msg: "no previous directory"}
		}
		return cdTarget{dest: prev, cfg: cfg}, nil
	}
	res, cfg, err := a.resolve(ctx, dir, target, modeNavigate)
	return cdTarget{dest: res.Path, cfg: cfg, w: res.Worktree, repo: res.Repo}, err
}

// planCD decides the events of wt cd, which run at the root of the
// worktree that contains the destination: none when no cd hook would run,
// or when the destination is inside no worktree, as the previous directory
// of wt cd - can be. Only when a cd hook would run does it read the
// repository of a destination that resolving did not read.
func (a *app) planCD(hooks *hookPlanner, t cdTarget) (pre, post hook.Run, err error) {
	pre, post = hook.Run{Event: hook.PreCD}, hook.Run{Event: hook.PostCD}
	if !hooks.configured(hook.PreCD, hook.PostCD) {
		return pre, post, nil
	}
	w, repo := t.w, t.repo
	if w == nil {
		r, err := a.loadRepository(hooks.ctx, t.dest)
		var notRepo *git.NotRepoError
		switch {
		case errors.As(err, &notRepo):
			return pre, post, nil
		case err != nil:
			return pre, post, gitError(err)
		case r.current == nil:
			return pre, post, nil
		}
		w, repo = r.current, r
	}
	main := repo.main()
	hooks.gitDir = main.Path
	values := worktree.HookVars(*main, *w, "", a.env.GOOS)
	if pre, err = hooks.plan(hook.PreCD, values, w.Path); err != nil {
		return pre, post, err
	}
	if post, err = hooks.plan(hook.PostCD, values, w.Path); err != nil {
		return pre, post, err
	}
	return pre, post, nil
}

// mustExist fails with exit 3 when dest is not a directory: a worktree
// whose directory was deleted is still listed by git.
func mustExist(dest string) error {
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		return &Error{Code: ExitNotFound, Msg: "directory does not exist: " + dest}
	}
	return nil
}

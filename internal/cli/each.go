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
	"github.com/jMautone/worktree-manager/internal/process"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const eachLong = `Run a command in every worktree of the current repository, one after the
other, in the order of "wt list". --all-repos runs it, from anywhere, in
every worktree that "wt list --all-repos" lists.

The command starts at the first word that is not a flag of wt: from there
on, every word is the command's, even one that looks like a flag of wt, as
--json in wt each npm test --json. Flags of wt go before the command. A --
before it is allowed but never needed (PowerShell drops it anyway).

A single word is the command line as written, run by sh -c on macOS and
Linux and by cmd.exe on Windows, as -x of "wt create", operators included:

  wt each 'npm ci && npm test'

Several words reach the program each one whole, as you typed them:

  wt each git commit -m "fix bug"

Your shell expands a glob without quotes before wt sees it, in the
current directory: write wt each 'ls *.go' to expand it in each worktree.
On Windows, cmd.exe expands %NAME% in any word, quoted or not.

Each command runs in the root of its worktree, with this terminal. Before
it, a line "==> <worktree>  <path>" names the worktree as "wt cd" takes
it: ^ and feat, or api:^ and api:feat with --all-repos. A bare repository
is skipped, and so is, with a warning, a worktree whose directory does not
exist.

When a command fails, wt each goes on with the next one; at the end it
lists the ones that failed and exits with 1. Ctrl-C reaches the command
that is running; once that command ends, wt each runs no other and exits
with 130.`

const hintEachAll = "run 'wt each --all-repos' to run it in the worktrees of every repository"

func (a *app) eachCommand() *cobra.Command {
	var allRepos bool
	cmd := &cobra.Command{
		Use:   "each [--all-repos] [--] <cmd>...",
		Short: "Run a command in every worktree",
		Long:  eachLong,
		Args:  cobra.ArbitraryArgs,
		// The words of the command are the shell's to complete; cobra
		// offers no flag of wt after the first one, since flags are not
		// interspersed.
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: action(func(cmd *cobra.Command, args []string) error {
			if err := validateEach(cmd, args, a.json); err != nil {
				return err
			}
			// From here on, a Ctrl-C does not end wt: it reaches the
			// command that is running, and wt runs no other once that
			// one ends. Go stops terminating the process on an interrupt
			// while a channel is registered for it, so one that arrives
			// between commands or while reading is caught too.
			var interrupts chan os.Signal
			if !a.dryRun {
				interrupts = make(chan os.Signal, 1)
				signal.Notify(interrupts, os.Interrupt)
				defer signal.Stop(interrupts)
			}
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			groups, err := a.eachGroups(cmd.Context(), dir, allRepos)
			if err != nil {
				return err
			}
			plan := worktree.PlanEach(groups, goneDirs(groups))
			for _, t := range plan.Gone {
				a.warn(fmt.Sprintf("skipping %s: %s does not exist", t.Label, t.Worktree.Path))
			}
			line := process.Line(a.env.GOOS, args)
			if a.dryRun {
				for _, t := range plan.Run {
					if _, err := fmt.Fprintf(a.env.Stdout, "would run %s in %s\n", line, t.Worktree.Path); err != nil {
						return err
					}
				}
				return nil
			}
			return a.each(plan, line, interrupts)
		}),
	}
	// The first word of the command ends the flags of wt: everything after
	// it is the command's.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "run the command in the worktrees of every repository under the roots")
	return cmd
}

// validateEach rejects the usage errors of wt each, before anything is read.
func validateEach(cmd *cobra.Command, args []string, asJSON bool) error {
	switch {
	case asJSON:
		e := usageError(cmd, `"wt each" has no JSON output`)
		e.Hints = append([]string{"run 'wt list --json' to read the worktrees"}, e.Hints...)
		return e
	case len(args) == 0:
		return usageError(cmd, `missing <cmd> for "wt each"`)
	case len(args) == 1 && strings.TrimSpace(args[0]) == "":
		return usageError(cmd, `empty command for "wt each"`)
	}
	return nil
}

// eachGroups reads the worktrees wt each visits: those of the repository
// of dir, as wt list reads them, or with allRepos those of every repository
// under the roots, as wt list --all-repos does, with its warnings.
func (a *app) eachGroups(ctx context.Context, dir string, allRepos bool) ([]worktree.EachGroup, error) {
	if allRepos {
		scanned, names, err := a.workspaceWorktrees(ctx, dir)
		if err != nil {
			return nil, err
		}
		var groups []worktree.EachGroup
		for i, s := range scanned {
			if s.Err == nil {
				groups = append(groups, worktree.EachGroup{Repo: names.names[i], Worktrees: s.Worktrees})
			}
		}
		return groups, nil
	}
	repo, err := a.loadRepository(ctx, dir)
	var notRepo *git.NotRepoError
	if errors.As(err, &notRepo) {
		e := gitError(err).(*Error)
		if base, err := a.loadConfigQuiet(""); err == nil && len(reposRoot(base)) > 0 {
			e.Hints = append(e.Hints, hintEachAll)
		}
		return nil, e
	}
	if err != nil {
		return nil, gitError(err)
	}
	if _, err := a.loadConfig(repo.root()); err != nil {
		return nil, err
	}
	return []worktree.EachGroup{{Worktrees: repo.worktrees}}, nil
}

// goneDirs is the set of paths of the worktrees of groups whose directory
// cannot be used: any error of Stat counts, not only a missing one, since
// no command can run there.
func goneDirs(groups []worktree.EachGroup) map[string]bool {
	gone := map[string]bool{}
	for _, g := range groups {
		for _, w := range g.Worktrees {
			if w.Bare {
				continue
			}
			if info, err := os.Stat(w.Path); err != nil || !info.IsDir() {
				gone[w.Path] = true
			}
		}
	}
	return gone
}

// each runs line in the worktrees of plan, one at a time, each after its
// header, and reports how they ended. An interrupt received on interrupts
// stops it before the next command; the command that is running gets the
// Ctrl-C itself, from the terminal, and wt waits for it.
func (a *app) each(plan worktree.EachPlan, line string, interrupts <-chan os.Signal) error {
	env := shell.ChildEnviron(a.env.Environ, a.env.GOOS)
	color := a.color()
	results := make([]eachResult, 0, len(plan.Run))
	interrupted := false
	for _, t := range plan.Run {
		if interrupted = pending(interrupts); interrupted {
			break
		}
		a.eachHeader(t, color)
		code, err := runCommand(t.Worktree.Path, line, env, a.env.Stdin, a.env.Stdout, a.env.Stderr)
		results = append(results, eachResult{Label: t.Label, Code: code, Err: err})
	}
	// Also when the interrupt came while the last command ran.
	interrupted = interrupted || pending(interrupts)
	return eachReport(results, len(plan.Run), interrupted)
}

// pending reports, without waiting, whether an interrupt was received.
func pending(interrupts <-chan os.Signal) bool {
	select {
	case <-interrupts:
		return true
	default:
		return false
	}
}

// eachHeader writes the line that names the worktree t before its command
// runs. stdout has no buffer, so the line comes before anything the
// command writes to the same file.
func (a *app) eachHeader(t worktree.EachTarget, color bool) {
	arrow, label := "==>", t.Label
	if color {
		arrow = "\x1b[" + styleBold + "m" + arrow + "\x1b[0m"
		label = "\x1b[" + styleBold + "m" + label + "\x1b[0m"
	}
	_, _ = fmt.Fprintf(a.env.Stdout, "%s %s  %s\n", arrow, label, t.Worktree.Path)
}

// eachResult is how the command of wt each ended in one worktree: its exit
// code, or err when it could not be started.
type eachResult struct {
	Label string
	Code  int
	Err   error
}

// eachReport is the outcome of wt each once the loop is over: results are
// the commands that ran, planned how many it would have run, and
// interrupted whether a Ctrl-C stopped it. It is nil when every command
// ran and exited with 0; otherwise its hints name each failed command, in
// the order they ran.
func eachReport(results []eachResult, planned int, interrupted bool) error {
	var hints []string
	for _, r := range results {
		switch {
		case r.Err != nil:
			hints = append(hints, fmt.Sprintf("%s could not start: %v", r.Label, r.Err))
		case r.Code != 0:
			hints = append(hints, fmt.Sprintf("%s exited with %d", r.Label, r.Code))
		}
	}
	switch {
	case interrupted:
		return &Error{Code: ExitInterrupted, Msg: fmt.Sprintf("interrupted: %d of %d commands did not run", planned-len(results), planned), Hints: hints}
	case len(hints) > 0:
		return &Error{Code: ExitError, Msg: fmt.Sprintf("%d of %d commands failed", len(hints), len(results)), Hints: hints}
	}
	return nil
}

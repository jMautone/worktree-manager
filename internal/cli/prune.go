package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const pruneLong = `Forget the worktrees of the current repository whose directory no longer
exists: those "wt list" shows as prunable. Locked worktrees are kept.

Only git's record of each worktree is removed; branches are not touched.
"wt remove" also forgets such a worktree, and deletes its branch when it is
merged.`

func (a *app) pruneCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "prune",
		Short:             "Forget worktrees whose directory is gone",
		Long:              pruneLong,
		Args:              noArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			repo, err := a.requireRepository(ctx, dir)
			if err != nil {
				return err
			}
			if _, err := a.loadConfig(repo.root()); err != nil {
				return err
			}
			// git prunes by the same rule it reports prunable worktrees
			// with, so what is listed here is what it prunes.
			pruned := worktree.Prunable(repo.worktrees)
			if len(pruned) > 0 && !a.dryRun {
				if err := git.PruneWorktrees(ctx, a.git, dir); err != nil {
					return gitError(err)
				}
			}
			return a.printPruned(pruned)
		}),
	}
}

// pruneJSON is one element of the wt.prune.v1 document.
type pruneJSON struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (a *app) printPruned(pruned []worktree.Worktree) error {
	if a.json {
		doc := struct {
			Schema string      `json:"schema"`
			Pruned []pruneJSON `json:"pruned"`
		}{"wt.prune.v1", []pruneJSON{}}
		for _, w := range pruned {
			doc.Pruned = append(doc.Pruned, pruneJSON{w.Name, w.Path, w.PrunableReason})
		}
		return a.writeJSON(doc)
	}
	if len(pruned) == 0 {
		_, err := fmt.Fprintln(a.env.Stdout, "nothing to prune")
		return err
	}
	verb := "pruned"
	if a.dryRun {
		verb = "would prune"
	}
	var b strings.Builder
	for _, w := range pruned {
		fmt.Fprintf(&b, "%s worktree %s\n", verb, w.Path)
	}
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

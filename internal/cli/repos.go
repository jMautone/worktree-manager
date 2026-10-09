package cli

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/workspace"
)

const reposLong = `List the repositories found under the roots in repos_root.

wt looks repos_depth levels below each root (1 by default, at most 3) for
directories with a .git entry. It does not look inside a repository or a
worktree, and a linked worktree is not a repository. Set the roots in the
user configuration file ("wt config path" shows where), for example:

  repos_root = ["~/src", "/Volumes/work"]

or in WT_REPOS_ROOT, separated by : (by ; on Windows).

The first column marks with @ the repository of the working directory.
"wt cd <name>" jumps to any of these repositories from anywhere.`

func (a *app) reposCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "List the repositories under the configured roots",
		Long:  reposLong,
		Args:  noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			// wt repos needs no repository: outside one, or when git fails,
			// it only loses the @ marker.
			repo, _ := a.loadRepository(cmd.Context(), dir)
			cfg, err := a.loadConfig(repo.root())
			if err != nil {
				return err
			}
			if len(reposRoot(cfg)) == 0 {
				userFile, _ := a.userFile()
				return &Error{
					Code:  ExitError,
					Msg:   "no repository roots configured",
					Hints: []string{fmt.Sprintf(`set repos_root in %s, e.g. repos_root = ["~/src"]`, userFile)},
				}
			}
			repos := a.discoverRepos(cfg, a.warn)
			current := -1
			if main := repo.main(); main != nil {
				if id, err := workspace.IDOf(main.Path); err == nil {
					current = workspace.Current(repos, id)
				}
			}
			if a.json {
				return a.writeJSON(reposDocument(repos, current))
			}
			if len(repos) == 0 {
				return nil
			}
			var buf bytes.Buffer
			if err := renderTable(&buf, reposRows(repos, current), a.color()); err != nil {
				return err
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}

// reposRoot is the effective repos_root.
func reposRoot(cfg *config.Config) []string {
	v, _ := cfg.Get("repos_root")
	roots, _ := v.Value.([]string)
	return roots
}

// discoverRepos finds the repositories under the roots of cfg, as wt repos
// lists them. Warnings, such as a missing root, go to warn.
func (a *app) discoverRepos(cfg *config.Config, warn func(string)) []workspace.Repo {
	v, _ := cfg.Get("repos_depth")
	depth, _ := v.Value.(int)
	roots, warnings := workspace.Roots(reposRoot(cfg), a.env.GOOS, a.home())
	found, walkWarnings := workspace.Walk(roots, depth)
	for _, w := range append(warnings, walkWarnings...) {
		warn(w)
	}
	return workspace.List(found, a.env.GOOS)
}

// reposRows builds the table of `wt repos`: marker, NAME, PATH.
func reposRows(repos []workspace.Repo, current int) [][]cell {
	rows := [][]cell{{
		{},
		{text: "NAME", style: styleBold},
		{text: "PATH", style: styleBold},
	}}
	for i, r := range repos {
		marker, name := cell{}, cell{text: r.Name}
		if i == current {
			marker = cell{text: "@", style: styleCurrent}
			name.style = styleBold
		}
		rows = append(rows, []cell{marker, name, {text: r.Path}})
	}
	return rows
}

// reposJSON is the wt.repos.v1 document.
type reposJSON struct {
	Schema string     `json:"schema"`
	Repos  []repoJSON `json:"repos"`
}

type repoJSON struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Root    string `json:"root"`
	Current bool   `json:"current"`
}

func reposDocument(repos []workspace.Repo, current int) reposJSON {
	doc := reposJSON{Schema: "wt.repos.v1", Repos: make([]repoJSON, 0, len(repos))}
	for i, r := range repos {
		doc.Repos = append(doc.Repos, repoJSON{Name: r.Name, Path: r.Path, Root: r.Root, Current: i == current})
	}
	return doc
}

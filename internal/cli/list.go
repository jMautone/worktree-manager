package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const listLong = `List every worktree of the repository that contains the working directory.

The first column marks the current worktree with @ and the main one with ^.

--all-repos lists, from anywhere, the worktrees of every repository under
the roots in repos_root (as in "wt repos"), with the REPO column naming
each one's repository as "wt cd <repo>:<name>" takes it. A repository
that git cannot read is left out with a warning.`

func (a *app) listCommand() *cobra.Command {
	var allRepos bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the worktrees of the current repository",
		Long:  listLong,
		Args:  noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			if allRepos {
				return a.listAll(cmd.Context(), dir)
			}
			repo, err := a.loadRepository(cmd.Context(), dir)
			var notRepo *git.NotRepoError
			if errors.As(err, &notRepo) {
				e := gitError(err).(*Error)
				if base, err := a.loadConfigQuiet(""); err == nil && len(reposRoot(base)) > 0 {
					e.Hints = append(e.Hints, hintListAll)
				}
				return e
			}
			if err != nil {
				return gitError(err)
			}
			if _, err := a.loadConfig(repo.root()); err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(listDocument(repo.worktrees))
			}
			return a.writeTable(listRows(repo.worktrees, nil))
		}),
	}
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "list the worktrees of every repository under the roots")
	return cmd
}

// listAll is wt list --all-repos: the worktrees of every repository under
// the roots, which need no current repository.
func (a *app) listAll(ctx context.Context, dir string) error {
	scanned, names, err := a.workspaceWorktrees(ctx, dir)
	if err != nil {
		return err
	}
	if a.json {
		return a.writeJSON(listAllDocument(scanned))
	}
	var ws []worktree.Worktree
	var repoNames []string
	for i, s := range scanned {
		for _, w := range s.Worktrees {
			ws = append(ws, w)
			repoNames = append(repoNames, names.names[i])
		}
	}
	if len(ws) == 0 {
		return nil
	}
	return a.writeTable(listRows(ws, repoNames))
}

// workspaceWorktrees reads the worktrees of every repository under the
// roots, as wt list --all-repos lists them, with the repository of dir
// marking its current worktree. It warns about each repository git cannot
// read, which keeps its error in Err. The configuration has no repository
// layer: the roots never come from one.
func (a *app) workspaceWorktrees(ctx context.Context, dir string) ([]repoWorktrees, workspaceNames, error) {
	repo, _ := a.loadRepository(ctx, dir)
	cfg, err := a.loadConfig("")
	if err != nil {
		return nil, workspaceNames{}, err
	}
	if len(reposRoot(cfg)) == 0 {
		return nil, workspaceNames{}, a.noRootsError()
	}
	repos := a.discoverRepos(cfg, a.warn)
	cwdReal := dir
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		cwdReal = real
	}
	scanned, err := a.scanWorktrees(ctx, repos, currentRepo(repo, repos), cwdReal)
	if err != nil {
		return nil, workspaceNames{}, gitError(err)
	}
	names := newWorkspaceNames(repos, a.env.GOOS)
	a.readable(scanned, names, a.warn)
	return scanned, names, nil
}

// writeTable writes rows to stdout as an aligned table.
func (a *app) writeTable(rows [][]cell) error {
	var buf bytes.Buffer
	if err := renderTable(&buf, rows, a.color()); err != nil {
		return err
	}
	_, err := a.env.Stdout.Write(buf.Bytes())
	return err
}

// listRows builds the table of `wt list`: markers, NAME, BRANCH, HEAD,
// STATE, PATH. With repos, the REPO of each worktree, aligned with ws, it
// is the table of --all-repos, with REPO after the markers.
func listRows(ws []worktree.Worktree, repos []string) [][]cell {
	header := []cell{{}}
	if repos != nil {
		header = append(header, cell{text: "REPO", style: styleBold})
	}
	rows := [][]cell{append(header, []cell{
		{text: "NAME", style: styleBold},
		{text: "BRANCH", style: styleBold},
		{text: "HEAD", style: styleBold},
		{text: "STATE", style: styleBold},
		{text: "PATH", style: styleBold},
	}...)}
	for i, w := range ws {
		markers := cell{}
		switch {
		case w.Current && w.Main:
			markers = cell{text: "@^", style: styleCurrent}
		case w.Current:
			markers = cell{text: "@", style: styleCurrent}
		case w.Main:
			markers = cell{text: "^", style: styleMain}
		}

		name := cell{text: w.Name}
		if w.Current {
			name.style = styleBold
		}

		branch := cell{text: w.Branch, style: styleBranch}
		switch {
		case w.Bare:
			branch = cell{text: "(bare)", style: styleDim}
		case w.Detached:
			branch = cell{text: "(detached)", style: styleWarn}
		}

		head := cell{text: "-", style: styleDim}
		if w.Head != "" {
			head.text = w.Head[:min(7, len(w.Head))]
		}

		state := cell{}
		switch {
		case w.Locked && w.Prunable:
			state = cell{text: "locked,prunable", style: styleError}
		case w.Locked:
			state = cell{text: "locked", style: styleWarn}
		case w.Prunable:
			state = cell{text: "prunable", style: styleError}
		}

		row := []cell{markers}
		if repos != nil {
			row = append(row, cell{text: repos[i]})
		}
		rows = append(rows, append(row, name, branch, head, state, cell{text: w.Path}))
	}
	return rows
}

// listJSON is the wt.list.v1 document.
type listJSON struct {
	Schema    string         `json:"schema"`
	Worktrees []worktreeJSON `json:"worktrees"`
}

type worktreeJSON struct {
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	Branch         *string `json:"branch"`
	Head           *string `json:"head"`
	Detached       bool    `json:"detached"`
	Bare           bool    `json:"bare"`
	Main           bool    `json:"main"`
	Current        bool    `json:"current"`
	Locked         bool    `json:"locked"`
	LockedReason   *string `json:"locked_reason"`
	Prunable       bool    `json:"prunable"`
	PrunableReason *string `json:"prunable_reason"`
}

func listDocument(ws []worktree.Worktree) listJSON {
	doc := listJSON{Schema: "wt.list.v1", Worktrees: make([]worktreeJSON, 0, len(ws))}
	for _, w := range ws {
		doc.Worktrees = append(doc.Worktrees, toWorktreeJSON(w))
	}
	return doc
}

func toWorktreeJSON(w worktree.Worktree) worktreeJSON {
	return worktreeJSON{
		Name:           w.Name,
		Path:           w.Path,
		Branch:         nullable(w.Branch),
		Head:           nullable(w.Head),
		Detached:       w.Detached,
		Bare:           w.Bare,
		Main:           w.Main,
		Current:        w.Current,
		Locked:         w.Locked,
		LockedReason:   nullable(w.LockedReason),
		Prunable:       w.Prunable,
		PrunableReason: nullable(w.PrunableReason),
	}
}

// listAllJSON is the wt.list.v1 document of --all-repos: a separate type,
// so that the fields it adds never reach wt list --json.
type listAllJSON struct {
	Schema     string            `json:"schema"`
	Worktrees  []worktreeAllJSON `json:"worktrees"`
	Unreadable []unreadableJSON  `json:"unreadable"`
}

type worktreeAllJSON struct {
	worktreeJSON
	Repo     string `json:"repo"`      // the repository's name, as in wt repos
	RepoPath string `json:"repo_path"` // the repository's path, as in wt repos
}

type unreadableJSON struct {
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func listAllDocument(scanned []repoWorktrees) listAllJSON {
	doc := listAllJSON{Schema: "wt.list.v1", Worktrees: []worktreeAllJSON{}, Unreadable: []unreadableJSON{}}
	for _, s := range scanned {
		if s.Err != nil {
			doc.Unreadable = append(doc.Unreadable, unreadableJSON{s.Repo.Name, s.Repo.Path, oneLine(gitMessage(s.Err))})
			continue
		}
		for _, w := range s.Worktrees {
			doc.Worktrees = append(doc.Worktrees, worktreeAllJSON{toWorktreeJSON(w), s.Repo.Name, s.Repo.Path})
		}
	}
	return doc
}

// nullable maps "" to JSON null.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

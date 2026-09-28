package cli

import (
	"bytes"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/worktree"
)

func (a *app) listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the worktrees of the current repository",
		Long: `List every worktree of the repository that contains the working directory.

The first column marks the current worktree with @ and the main one with ^.`,
		Args: noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			repo, err := a.requireRepository(cmd.Context(), dir)
			if err != nil {
				return err
			}
			if _, err := a.loadConfig(repo.root()); err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(listDocument(repo.worktrees))
			}
			var buf bytes.Buffer
			if err := renderTable(&buf, listRows(repo.worktrees), a.color()); err != nil {
				return err
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}

// listRows builds the table of `wt list`: markers, NAME, BRANCH, HEAD,
// STATE, PATH.
func listRows(ws []worktree.Worktree) [][]cell {
	rows := [][]cell{{
		{},
		{text: "NAME", style: styleBold},
		{text: "BRANCH", style: styleBold},
		{text: "HEAD", style: styleBold},
		{text: "STATE", style: styleBold},
		{text: "PATH", style: styleBold},
	}}
	for _, w := range ws {
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

		rows = append(rows, []cell{markers, name, branch, head, state, {text: w.Path}})
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
		doc.Worktrees = append(doc.Worktrees, worktreeJSON{
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
		})
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

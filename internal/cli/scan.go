package cli

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// repoWorktrees is what scanWorktrees read for one repository.
type repoWorktrees struct {
	Repo      workspace.Repo
	Worktrees []worktree.Worktree // nil when Err is not
	Err       error
}

// scanWorktrees lists the worktrees of every repository of repos, with git
// running for several at once, and returns them in the order of repos.
// Only the repository at index current (-1 for none) gets cwdReal, so no
// worktree of another repository is current, even in nested layouts.
//
// A repository git fails for keeps its error in Err, for the caller to
// warn about, report or ignore. git not being found is the only error
// returned, since then no repository can be read: git.ErrNotInstalled.
func (a *app) scanWorktrees(ctx context.Context, repos []workspace.Repo, current int, cwdReal string) ([]repoWorktrees, error) {
	results := make([]repoWorktrees, len(repos))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, r := range repos {
		results[i].Repo = r
		cwd := ""
		if i == current {
			cwd = cwdReal
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i].Worktrees, results[i].Err = a.listWorktrees(ctx, r.Path, cwd)
		}()
	}
	wg.Wait()
	for _, r := range results {
		if errors.Is(r.Err, git.ErrNotInstalled) {
			return nil, git.ErrNotInstalled
		}
	}
	return results, nil
}

// currentRepo is the index in repos of the repository repo belongs to, as
// wt repos marks it, or -1.
func currentRepo(repo *repository, repos []workspace.Repo) int {
	main := repo.main()
	if main == nil {
		return -1
	}
	id, err := workspace.IDOf(main.Path)
	if err != nil {
		return -1
	}
	return workspace.Current(repos, id)
}

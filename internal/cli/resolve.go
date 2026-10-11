package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// resolveMode is what a command's target may designate.
type resolveMode int

const (
	// modeNavigate is for wt cd and wt path: a name may also be a
	// repository, and "api:" is the repository itself.
	modeNavigate resolveMode = iota
	// modeWorktree is for wt remove, lock and unlock: only worktrees, and
	// "api:" is the main worktree.
	modeWorktree
)

// resolved is the destination of a target.
type resolved struct {
	Path     string             // where wt cd goes and wt path prints
	Worktree *worktree.Worktree // nil when the target is a repository (modeNavigate only)
	Repo     *repository        // the target's repository; nil when the target is a repository
	GitDir   string             // where git runs for that repository
	Other    bool               // the repository is not the working directory's
}

// resolve resolves target as wt cd, wt path, wt remove, wt lock and wt
// unlock do, from dir, and loads the configuration with the repository
// layer of the target's repository. Each step is a pure decision on what
// the edge already read, and the edge only reads what the next step needs:
//
//  1. A qualified target is checked for usage errors before reading
//     anything.
//  2. The working directory's repository, if any.
//  3. The configuration without a repository layer, which is enough for
//     the roots: a .wt.toml cannot set repos_root.
//  4. A qualified target: its repository side among the repositories under
//     the roots, then its name side among that repository's worktrees.
//  5. Otherwise, the current repository; then, in modeNavigate, the
//     repositories by name; then the linked worktrees of the others.
//  6. The configuration, with the repository layer of the target's
//     repository; its warnings are printed here, once.
//
// `-` is not resolved here: it is wt cd's own.
func (a *app) resolve(ctx context.Context, dir, target string, mode resolveMode) (resolved, *config.Config, error) {
	repoSide, nameSide, qualified := workspace.SplitQualified(target)
	if qualified {
		if err := qualifiedUsage(target, repoSide, nameSide); err != nil {
			return resolved{}, nil, err
		}
	}

	repo, err := a.loadRepository(ctx, dir)
	var notRepo *git.NotRepoError
	if err != nil && !errors.As(err, &notRepo) {
		return resolved{}, nil, gitError(err)
	}
	base, err := a.loadConfigQuiet("")
	if err != nil {
		return resolved{}, nil, err
	}

	var res resolved
	if qualified {
		res, err = a.resolveQualified(ctx, dir, repo, base, repoSide, nameSide, mode)
	} else {
		res, err = a.resolveName(ctx, dir, repo, notRepo, base, target, mode)
	}
	if err != nil {
		return resolved{}, nil, err
	}

	root := repo.root()
	if res.Other {
		root = otherRoot(res.Repo)
	}
	cfg, err := a.loadConfig(root)
	if err != nil {
		return resolved{}, nil, err
	}
	return res, cfg, nil
}

// otherRoot is the directory whose .wt.toml is the repository layer of a
// command that operates on repo, another repository than the working
// directory's: its main worktree, or "" for a bare repository, which has
// no committed .wt.toml to read.
func otherRoot(repo *repository) string {
	if main := repo.main(); main != nil && !main.Bare {
		return main.Path
	}
	return ""
}

// local is the worktree w of the working directory's repository.
func local(repo *repository, dir string, w worktree.Worktree) resolved {
	return resolved{Path: w.Path, Worktree: &w, Repo: repo, GitDir: dir}
}

// resolveQualified resolves <repoSide>:<nameSide>.
func (a *app) resolveQualified(ctx context.Context, dir string, repo *repository, base *config.Config, repoSide, nameSide string, mode resolveMode) (resolved, error) {
	r, names, err := a.matchRepo(base, repoSide)
	if err != nil {
		return resolved{}, err
	}
	if nameSide == "" {
		if mode == modeNavigate {
			return resolved{Path: r.Path}, nil
		}
		nameSide = "^"
	}
	target, gitDir, other, err := a.readRepo(ctx, dir, repo, names.repos, r)
	if err != nil {
		return resolved{}, err
	}
	w, err := worktree.Resolve(target.worktrees, nameSide)
	var nf *worktree.NotFoundError
	if errors.As(err, &nf) {
		return resolved{}, qualifiedNotFound(nameSide, names.of(r))
	}
	if err != nil {
		return resolved{}, worktreeError(err)
	}
	return resolved{Path: w.Path, Worktree: &w, Repo: target, GitDir: gitDir, Other: other}, nil
}

// matchRepo resolves the repository side of a qualified target among the
// repositories under the roots of base, which must name at least one.
func (a *app) matchRepo(base *config.Config, side string) (workspace.Repo, workspaceNames, error) {
	if len(reposRoot(base)) == 0 {
		return workspace.Repo{}, workspaceNames{}, a.noRootsError()
	}
	repos := a.discoverRepos(base, a.warn)
	names := newWorkspaceNames(repos, a.env.GOOS)
	r, err := workspace.MatchSide(repos, side, a.env.GOOS)
	var nf *workspace.NotFoundError
	var amb *workspace.AmbiguousError
	switch {
	case errors.As(err, &nf):
		return workspace.Repo{}, names, repoNotFound(nf)
	case errors.As(err, &amb):
		return workspace.Repo{}, names, repoAmbiguity(amb, names)
	}
	return r, names, nil
}

// readRepo reads the worktrees of r, one of repos, and says where git runs
// for it. When r is the working directory's repository, it is repo, already
// read with its current worktree, and git runs in dir as for an unqualified
// target; otherwise git runs in r's path, and other is true. A repository
// named by a qualified target is never skipped: git failing for it is an
// error.
func (a *app) readRepo(ctx context.Context, dir string, repo *repository, repos []workspace.Repo, r workspace.Repo) (target *repository, gitDir string, other bool, err error) {
	if cur := currentRepo(repo, repos); cur >= 0 && repos[cur].Path == r.Path {
		return repo, dir, false, nil
	}
	ws, err := a.listWorktrees(ctx, r.Path, "")
	if err != nil {
		return nil, "", false, otherRepoError(err)
	}
	return newRepository(ws), r.Path, true, nil
}

// resolveName resolves a target without ":".
func (a *app) resolveName(ctx context.Context, dir string, repo *repository, notRepo *git.NotRepoError, base *config.Config, target string, mode resolveMode) (resolved, error) {
	var inRepo error // why the current repository did not resolve target
	if repo != nil {
		w, err := worktree.Resolve(repo.worktrees, target)
		var amb *worktree.AmbiguousError
		switch {
		case err == nil:
			return local(repo, dir, w), nil
		case errors.As(err, &amb):
			// An ambiguity among the worktrees is not resolved elsewhere.
			return resolved{}, worktreeError(err)
		}
		inRepo = err
	}
	if target == "^" || target == "@" {
		if repo == nil {
			return resolved{}, gitError(notRepo)
		}
		return resolved{}, worktreeError(inRepo)
	}
	roots := len(reposRoot(base)) > 0
	if !roots {
		return resolved{}, nameNotFound(target, notRepo, false, mode)
	}

	repos := a.discoverRepos(base, a.warn)
	names := newWorkspaceNames(repos, a.env.GOOS)
	if mode == modeNavigate {
		r, err := workspace.Match(repos, target)
		var amb *workspace.AmbiguousError
		switch {
		case err == nil:
			return resolved{Path: r.Path}, nil
		case errors.As(err, &amb):
			return resolved{}, repoAmbiguity(amb, names)
		}
	}

	current := currentRepo(repo, repos)
	others := make([]workspace.Repo, 0, len(repos))
	for i, r := range repos {
		if i != current {
			others = append(others, r)
		}
	}
	scanned, err := a.scanWorktrees(ctx, others, -1, "")
	if err != nil {
		return resolved{}, gitError(err)
	}
	groups := a.readable(scanned, names, a.warn)
	hit, err := worktree.ResolveOthers(groups, target)
	var amb *worktree.OthersAmbiguousError
	switch {
	case err == nil:
		w := hit.Worktree
		return resolved{Path: w.Path, Worktree: &w, Repo: newRepository(groups[hit.Group]), GitDir: others[hit.Group].Path, Other: true}, nil
	case errors.As(err, &amb):
		return resolved{}, othersAmbiguity(amb, others, names)
	}
	return resolved{}, nameNotFound(target, notRepo, true, mode)
}

// readable returns the worktrees of each scanned repository, in the same
// order, and warns about each repository git could not read, whose
// worktrees count as absent.
func (a *app) readable(scanned []repoWorktrees, names workspaceNames, warn func(string)) [][]worktree.Worktree {
	groups := make([][]worktree.Worktree, len(scanned))
	for i, s := range scanned {
		if s.Err != nil {
			warn(fmt.Sprintf("cannot read the worktrees of %s: %s", names.of(s.Repo), oneLine(gitMessage(s.Err))))
			continue
		}
		groups[i] = s.Worktrees
	}
	return groups
}

// otherRepoError maps a failure to list the worktrees of a repository found
// under the roots. Unlike the working directory, such a directory is known
// to look like a repository: git not recognizing it is a git failure (exit
// 1), not a missing repository.
func otherRepoError(err error) error {
	var nre *git.NotRepoError
	if errors.As(err, &nre) {
		return &Error{Code: ExitError, Msg: nre.Error()}
	}
	return gitError(err)
}

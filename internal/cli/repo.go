package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

// workdir is the directory the command runs in: the process's working
// directory, or -C resolved against it.
func (a *app) workdir() (string, error) {
	cwd, err := a.env.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine the working directory: %w", err)
	}
	if a.dir == "" {
		return cwd, nil
	}
	dir := a.dir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", &Error{Code: ExitUsage, Msg: fmt.Sprintf("-C %s: no such directory", dir)}
	case err != nil:
		return "", &Error{Code: ExitUsage, Msg: fmt.Sprintf("-C %s: %v", dir, err)}
	case !info.IsDir():
		return "", &Error{Code: ExitUsage, Msg: fmt.Sprintf("-C %s: not a directory", dir)}
	}
	return dir, nil
}

// repository is the repository associated with a directory.
type repository struct {
	worktrees []worktree.Worktree
	current   *worktree.Worktree // nil when dir is inside no worktree path
}

// root is the directory whose .wt.toml is the repository layer: the current
// worktree's root, or "" when there is none. A bare repository has no
// working tree, so no committed .wt.toml to read.
func (r *repository) root() string {
	if r == nil || r.current == nil || r.current.Bare {
		return ""
	}
	return r.current.Path
}

// loadRepository lists the worktrees of the repository associated with dir.
// It returns git's errors unchanged; gitError maps them to exit codes.
//
// Symbolic links are resolved here, at the edge, in dir and in every path git
// reports, so that worktree.Build can compare paths purely: on macOS the
// working directory may be /var/... while git reports /private/var/.... A
// path that no longer exists (prunable) is used as reported.
func (a *app) loadRepository(ctx context.Context, dir string) (*repository, error) {
	entries, err := git.ListWorktrees(ctx, a.git, dir)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if real, err := filepath.EvalSymlinks(entries[i].Path); err == nil {
			entries[i].Path = real
		}
	}
	cwdReal := dir
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		cwdReal = real
	}
	repo := &repository{worktrees: worktree.Build(entries, cwdReal, a.env.GOOS)}
	for i := range repo.worktrees {
		if repo.worktrees[i].Current {
			repo.current = &repo.worktrees[i]
		}
	}
	return repo, nil
}

// requireRepository is loadRepository for commands that need a repository.
func (a *app) requireRepository(ctx context.Context, dir string) (*repository, error) {
	repo, err := a.loadRepository(ctx, dir)
	if err != nil {
		return nil, gitError(err)
	}
	return repo, nil
}

// optionalRepository is loadRepository for commands that also work outside
// a repository: not being in one returns nil, not an error.
func (a *app) optionalRepository(ctx context.Context, dir string) (*repository, error) {
	repo, err := a.loadRepository(ctx, dir)
	var nre *git.NotRepoError
	if errors.As(err, &nre) {
		return nil, nil
	}
	if err != nil {
		return nil, gitError(err)
	}
	return repo, nil
}

// gitError maps git failures to the exit-code contract.
func gitError(err error) error {
	var nre *git.NotRepoError
	var ce *git.CommandError
	switch {
	case errors.Is(err, git.ErrNotInstalled):
		return &Error{Code: ExitError, Msg: "git not found", Hints: []string{"install git and make sure it is on the PATH"}}
	case errors.As(err, &nre):
		return &Error{Code: ExitNotFound, Msg: nre.Error()}
	case errors.As(err, &ce):
		return &Error{Code: ExitError, Msg: ce.Error()}
	}
	return err
}

// home is the user's home directory: HOME, or USERPROFILE on windows.
func (a *app) home() string {
	if a.env.GOOS == "windows" {
		return a.env.Getenv("USERPROFILE")
	}
	return a.env.Getenv("HOME")
}

// userFile is the path of the user configuration file.
func (a *app) userFile() (string, error) {
	return config.UserFile(a.env.GOOS, a.env.Getenv, a.home())
}

// loadConfig reads and resolves every layer. repoRoot is the current
// worktree's root, or "" for no repository layer. Warnings are printed here.
func (a *app) loadConfig(repoRoot string) (*config.Config, error) {
	reg := config.Keys()
	userFile, err := a.userFile()
	if err != nil {
		return nil, err
	}
	var layers []config.Layer
	load := func(path string, source config.Source) error {
		layer, err := config.Load(path, source)
		if layer != nil {
			layers = append(layers, *layer)
		}
		return err
	}
	if err := load(userFile, config.SourceUser); err != nil {
		return nil, err
	}
	if repoRoot != "" {
		if err := load(filepath.Join(repoRoot, config.RepoFileName), config.SourceRepo); err != nil {
			return nil, err
		}
	}
	layers = append(layers, config.EnvLayer(reg, a.env.Getenv))
	cfg, err := config.Resolve(reg, layers...)
	if err != nil {
		return nil, err
	}
	for _, w := range cfg.Warnings {
		a.warn(w)
	}
	return cfg, nil
}

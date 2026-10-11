package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
	"github.com/jMautone/worktree-manager/internal/workspace"
)

// scanApp is an app that runs real git in sb's environment.
func scanApp(sb *testutil.Sandbox) *app {
	return &app{env: Env{GOOS: runtime.GOOS, Getenv: sb.Getenv}, git: git.Exec{Env: sb.Environ()}}
}

// scanRoot creates n repositories under <sandbox>/root, each with the
// linked worktree feat-<i>, and returns them as wt repos lists them.
func scanRoot(t *testing.T, sb *testutil.Sandbox, n int) []workspace.Repo {
	t.Helper()
	root := sb.Path("root")
	for i := range n {
		repo := filepath.Join(root, fmt.Sprintf("repo%02d", i))
		sb.InitRepo(repo)
		sb.AddWorktree(repo, filepath.Join(root, fmt.Sprintf("repo%02d.worktrees", i), "feat"), fmt.Sprintf("feat-%d", i))
	}
	found, warnings := workspace.Walk([]workspace.Root{{Raw: root, Path: root}}, 1)
	if len(warnings) > 0 {
		t.Fatal(warnings)
	}
	return workspace.List(found, runtime.GOOS)
}

func TestScanWorktreesOrder(t *testing.T) {
	sb := testutil.New(t)
	repos := scanRoot(t, sb, 12)
	a := scanApp(sb)
	for range 5 {
		results, err := a.scanWorktrees(context.Background(), repos, -1, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != len(repos) {
			t.Fatalf("%d results for %d repositories", len(results), len(repos))
		}
		for i, r := range results {
			if r.Err != nil || r.Repo.Path != repos[i].Path || len(r.Worktrees) != 2 {
				t.Fatalf("result %d = %+v, want the two worktrees of %s", i, r, repos[i].Path)
			}
			if want := fmt.Sprintf("feat-%d", i); r.Worktrees[1].Branch != want {
				t.Errorf("result %d has the branch %s, want %s", i, r.Worktrees[1].Branch, want)
			}
			for _, w := range r.Worktrees {
				if w.Current {
					t.Errorf("%s is current, with no working directory", w.Path)
				}
			}
		}
	}
}

func TestScanWorktreesUnreadable(t *testing.T) {
	sb := testutil.New(t)
	repos := scanRoot(t, sb, 3)
	sb.BreakHEAD(repos[1].Path)
	results, err := scanApp(sb).scanWorktrees(context.Background(), repos, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	if results[1].Err == nil || results[1].Worktrees != nil {
		t.Errorf("broken repository: %+v, want an error and no worktrees", results[1])
	}
	for _, i := range []int{0, 2} {
		if results[i].Err != nil || len(results[i].Worktrees) != 2 {
			t.Errorf("repository %d: %+v, want its worktrees", i, results[i])
		}
	}
}

func TestScanWorktreesCurrent(t *testing.T) {
	sb := testutil.New(t)
	repos := scanRoot(t, sb, 3)
	cwd := testutil.Comparable(t, filepath.Join(filepath.Dir(repos[1].Path), "repo01.worktrees", "feat"))
	results, err := scanApp(sb).scanWorktrees(context.Background(), repos, 1, cwd)
	if err != nil {
		t.Fatal(err)
	}
	var current []string
	for _, r := range results {
		for _, w := range r.Worktrees {
			if w.Current {
				current = append(current, w.Path)
			}
		}
	}
	if len(current) != 1 || current[0] != cwd {
		t.Errorf("current worktrees %q, want only %s", current, cwd)
	}
}

func TestScanWorktreesWithoutGit(t *testing.T) {
	sb := testutil.New(t)
	repos := scanRoot(t, sb, 2)
	t.Setenv("PATH", "")
	results, err := scanApp(sb).scanWorktrees(context.Background(), repos, -1, "")
	if !errors.Is(err, git.ErrNotInstalled) || results != nil {
		t.Errorf("scanWorktrees = %v, %v; want git.ErrNotInstalled", results, err)
	}
}

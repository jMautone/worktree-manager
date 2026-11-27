package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// ahead makes repo, on main, with the linked worktree feat one commit
// ahead of it that adds f.txt. It returns main's commit and feat's.
func ahead(t *testing.T, sb *testutil.Sandbox) (repo, feat, old, commit string) {
	t.Helper()
	repo = sb.Path("repo")
	sb.InitRepo(repo)
	old = commitFile(sb, repo, "a.txt", "a\n")
	feat = sb.Path("feat")
	sb.AddWorktree(repo, feat, "feat")
	commit = commitFile(sb, feat, "f.txt", "feat\n")
	return repo, feat, old, commit
}

func TestFastForwardCheckedOut(t *testing.T) {
	sb := testutil.New(t)
	repo, _, old, commit := ahead(t, sb)
	if err := git.FastForward(ctx, runner(sb), repo, "main", old, commit, repo); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(repo, "rev-parse", "main"); got != commit {
		t.Errorf("main = %s, want %s", got, commit)
	}
	if _, err := os.Stat(filepath.Join(repo, "f.txt")); err != nil {
		t.Errorf("the main worktree has no f.txt: %v", err)
	}
	if out := sb.Git(repo, "status", "--porcelain"); out != "" {
		t.Errorf("status = %q, want clean", out)
	}
}

func TestFastForwardNotCheckedOut(t *testing.T) {
	sb := testutil.New(t)
	repo, feat, old, commit := ahead(t, sb)
	sb.Git(repo, "branch", "dev", old)
	if err := git.FastForward(ctx, runner(sb), feat, "dev", old, commit, ""); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(repo, "rev-parse", "dev"); got != commit {
		t.Errorf("dev = %s, want %s", got, commit)
	}
	if got := sb.Git(repo, "rev-parse", "main"); got != old {
		t.Errorf("main moved to %s", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "f.txt")); err == nil {
		t.Error("the main worktree changed")
	}
}

func TestFastForwardStaleOld(t *testing.T) {
	sb := testutil.New(t)
	repo, _, old, commit := ahead(t, sb)
	sb.Git(repo, "branch", "dev", old)
	moved := commitFile(sb, repo, "b.txt", "b\n")
	sb.Git(repo, "branch", "-f", "dev", moved)

	if err := git.FastForward(ctx, runner(sb), repo, "main", old, commit, repo); err == nil {
		t.Error("checked out: no error with a stale old")
	}
	if got := sb.Git(repo, "rev-parse", "main"); got != moved {
		t.Errorf("main = %s, want %s", got, moved)
	}
	if err := git.FastForward(ctx, runner(sb), repo, "dev", old, commit, ""); err == nil {
		t.Error("not checked out: no error with a stale old")
	}
	if got := sb.Git(repo, "rev-parse", "dev"); got != moved {
		t.Errorf("dev = %s, want %s", got, moved)
	}
}

func TestFastForwardUntrackedInTheWay(t *testing.T) {
	sb := testutil.New(t)
	repo, _, old, commit := ahead(t, sb)
	sb.WriteFile(filepath.Join(repo, "f.txt"), "mine\n")
	if err := git.FastForward(ctx, runner(sb), repo, "main", old, commit, repo); err == nil {
		t.Fatal("no error with an untracked file in the way")
	}
	if got := sb.Git(repo, "rev-parse", "main"); got != old {
		t.Errorf("main moved to %s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "f.txt")); string(b) != "mine\n" {
		t.Errorf("f.txt = %q, want it untouched", b)
	}
}

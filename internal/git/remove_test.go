package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// registered returns git's entry for the worktree at dir, if any.
func registered(t *testing.T, sb *testutil.Sandbox, repo, dir string) (git.WorktreeEntry, bool) {
	t.Helper()
	es, err := git.ListWorktrees(ctx, runner(sb), repo)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.ToSlash(testutil.Comparable(t, dir))
	for _, e := range es {
		if filepath.ToSlash(testutil.Comparable(t, e.Path)) == want {
			return e, true
		}
	}
	return git.WorktreeEntry{}, false
}

func TestStatus(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	sb.WriteFile(filepath.Join(repo, "tracked"), "one\n")
	sb.WriteFile(filepath.Join(repo, ".gitignore"), "node_modules/\n*.env\n")
	sb.Git(repo, "add", ".")
	sb.Git(repo, "commit", "-q", "-m", "files")

	count := func(name string) int {
		t.Helper()
		entries, err := git.Status(ctx, runner(sb), repo)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return len(entries)
	}
	if n := count("clean"); n != 0 {
		t.Errorf("clean: %d entries", n)
	}

	// Only ignored files: nothing.
	sb.WriteFile(filepath.Join(repo, "node_modules", "x", "index.js"), "")
	sb.WriteFile(filepath.Join(repo, "secret.env"), "")
	if n := count("ignored"); n != 0 {
		t.Errorf("only ignored files: %d entries", n)
	}

	sb.WriteFile(filepath.Join(repo, "tracked"), "two\n")
	if n := count("modified"); n != 1 {
		t.Errorf("modified: %d entries, want 1", n)
	}
	sb.Git(repo, "add", "tracked")
	if n := count("staged"); n != 1 {
		t.Errorf("staged: %d entries, want 1", n)
	}
	sb.Git(repo, "commit", "-q", "-m", "two")

	sb.Git(repo, "config", "status.showUntrackedFiles", "no")
	sb.WriteFile(filepath.Join(repo, "untracked"), "")
	if n := count("untracked hidden by the configuration"); n != 1 {
		t.Errorf("untracked with status.showUntrackedFiles = no: %d entries, want 1", n)
	}

	if _, err := git.Status(ctx, runner(sb), sb.Root); err == nil {
		t.Error("Status outside a repository succeeded")
	}
}

func TestUpstreamAndIsAncestor(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)
	base := sb.Git(clone, "rev-parse", "HEAD")

	sb.Git(clone, "branch", "plain")
	if up, err := git.Upstream(ctx, runner(sb), clone, "plain"); err != nil || up != "" {
		t.Errorf("no upstream: %q, %v", up, err)
	}

	sb.Git(clone, "checkout", "-q", "-b", "feat")
	tip := sb.Commit(clone, "feat work")
	sb.Push(clone, "origin", "feat")
	sb.SetUpstream(clone, "feat", "origin/feat")
	if up, err := git.Upstream(ctx, runner(sb), clone, "feat"); err != nil || up != "origin/feat" {
		t.Errorf("with upstream: %q, %v", up, err)
	}

	// The remote branch is deleted, as a forge does when it merges a pull
	// request, and fetch --prune forgets it: still configured, gone.
	sb.Push(clone, "origin", "--delete", "feat")
	up, err := git.Upstream(ctx, runner(sb), clone, "feat")
	if err != nil || up != "origin/feat" {
		t.Errorf("upstream deleted: %q, %v; want the configured origin/feat", up, err)
	}
	if _, err := git.ResolveCommit(ctx, runner(sb), clone, up); !errors.Is(err, git.ErrNoRevision) {
		t.Errorf("ResolveCommit of the deleted upstream: %v, want ErrNoRevision", err)
	}

	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{base, tip, true},
		{tip, tip, true},
		{tip, base, false},
	} {
		if got, err := git.IsAncestor(ctx, runner(sb), clone, tc.a, tc.b); err != nil || got != tc.want {
			t.Errorf("IsAncestor(%.7s, %.7s) = %v, %v; want %v", tc.a, tc.b, got, err, tc.want)
		}
	}
	var ce *git.CommandError
	if _, err := git.IsAncestor(ctx, runner(sb), clone, "0123456789012345678901234567890123456789", tip); !errors.As(err, &ce) {
		t.Errorf("IsAncestor of a missing commit: %v, want a *CommandError", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)

	t.Run("removes and unregisters", func(t *testing.T) {
		wt := sb.Path("clean")
		sb.AddWorktree(repo, wt, "clean")
		if err := git.RemoveWorktree(ctx, runner(sb), repo, wt, false); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(wt); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the directory is still there: %v", err)
		}
		if _, ok := registered(t, sb, repo, wt); ok {
			t.Error("still registered")
		}
	})

	t.Run("directory gone", func(t *testing.T) {
		wt := sb.Path("gone")
		sb.AddWorktree(repo, wt, "gone")
		sb.MakePrunable(wt)
		if err := git.RemoveWorktree(ctx, runner(sb), repo, wt, false); err != nil {
			t.Fatal(err)
		}
		if _, ok := registered(t, sb, repo, wt); ok {
			t.Error("still registered")
		}
	})

	t.Run("untracked file", func(t *testing.T) {
		wt := sb.Path("dirty")
		sb.AddWorktree(repo, wt, "dirty")
		sb.WriteFile(filepath.Join(wt, "new"), "")
		var ce *git.CommandError
		if err := git.RemoveWorktree(ctx, runner(sb), repo, wt, false); !errors.As(err, &ce) {
			t.Fatalf("without --force: %v, want a *CommandError", err)
		}
		if _, ok := registered(t, sb, repo, wt); !ok || !exists(filepath.Join(wt, "new")) {
			t.Fatal("removed without --force")
		}
		if err := git.RemoveWorktree(ctx, runner(sb), repo, wt, true); err != nil {
			t.Fatalf("with --force: %v", err)
		}
		if _, ok := registered(t, sb, repo, wt); ok || exists(wt) {
			t.Error("not removed with --force")
		}
	})

	t.Run("directory cannot be deleted", func(t *testing.T) {
		wt := sb.Path("readonly")
		sb.AddWorktree(repo, wt, "readonly")
		sb.WriteFile(filepath.Join(wt, "sub", "file"), "x")
		sb.Git(wt, "add", ".")
		sb.Git(wt, "commit", "-q", "-m", "sub")
		sb.ReadOnlyDir(filepath.Join(wt, "sub"))
		var ce *git.CommandError
		if err := git.RemoveWorktree(ctx, runner(sb), repo, wt, false); !errors.As(err, &ce) || ce.Stderr == "" {
			t.Fatalf("%v, want a *CommandError with git's message", err)
		}
		// git unregisters it all the same.
		if _, ok := registered(t, sb, repo, wt); ok {
			t.Error("still registered")
		}
		if !exists(filepath.Join(wt, "sub", "file")) {
			t.Error("the file it could not delete is gone")
		}
	})
}

func TestLockUnlockAndPrune(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat, usb, gone := sb.Path("feat"), sb.Path("usb"), sb.Path("gone")
	for _, wt := range []string{feat, usb, gone} {
		sb.AddWorktree(repo, wt, filepath.Base(wt))
	}

	if err := git.LockWorktree(ctx, runner(sb), repo, usb, "on usb drive"); err != nil {
		t.Fatal(err)
	}
	if e, _ := registered(t, sb, repo, usb); !e.Locked || e.LockedReason != "on usb drive" {
		t.Errorf("locked with a reason: %+v", e)
	}
	if err := git.LockWorktree(ctx, runner(sb), repo, feat, ""); err != nil {
		t.Fatal(err)
	}
	if e, _ := registered(t, sb, repo, feat); !e.Locked || e.LockedReason != "" {
		t.Errorf("locked without a reason: %+v", e)
	}
	if err := git.UnlockWorktree(ctx, runner(sb), repo, feat); err != nil {
		t.Fatal(err)
	}
	if e, _ := registered(t, sb, repo, feat); e.Locked {
		t.Errorf("still locked: %+v", e)
	}

	sb.MakePrunable(usb)
	sb.MakePrunable(gone)
	if err := git.PruneWorktrees(ctx, runner(sb), repo); err != nil {
		t.Fatal(err)
	}
	if _, ok := registered(t, sb, repo, gone); ok {
		t.Error("gone was not pruned")
	}
	if _, ok := registered(t, sb, repo, usb); !ok {
		t.Error("the locked worktree was pruned")
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

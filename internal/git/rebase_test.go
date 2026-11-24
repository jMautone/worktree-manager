package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// commitFile writes content to name in dir and commits it.
func commitFile(sb *testutil.Sandbox, dir, name, content string) string {
	sb.WriteFile(filepath.Join(dir, name), content)
	sb.Git(dir, "add", name)
	sb.Git(dir, "commit", "-q", "-m", "edit "+name)
	return sb.Git(dir, "rev-parse", "HEAD")
}

// diverged makes repo with main and the linked worktree feat, each with a
// commit of its own: to file f.txt in both when conflict, to different
// files otherwise. It returns feat's path and the commit of main.
func diverged(t *testing.T, sb *testutil.Sandbox, conflict bool) (repo, feat, base string) {
	t.Helper()
	repo = sb.Path("repo")
	sb.InitRepo(repo)
	commitFile(sb, repo, "f.txt", "one\n")
	feat = sb.Path("feat")
	sb.AddWorktree(repo, feat, "feat")
	base = commitFile(sb, repo, "f.txt", "main\n")
	if conflict {
		commitFile(sb, feat, "f.txt", "feat\n")
	} else {
		commitFile(sb, feat, "g.txt", "feat\n")
	}
	return repo, feat, base
}

func TestTrackedChanges(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	commitFile(sb, repo, "tracked", "one\n")

	changed := func(name string) bool {
		t.Helper()
		ok, err := git.TrackedChanges(ctx, runner(sb), repo)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return ok
	}
	if changed("clean") {
		t.Error("clean: changes reported")
	}
	sb.WriteFile(filepath.Join(repo, "untracked"), "x")
	if changed("untracked") {
		t.Error("an untracked file counts as a change")
	}
	sb.WriteFile(filepath.Join(repo, "tracked"), "two\n")
	if !changed("modified") {
		t.Error("a modified tracked file does not count")
	}
	sb.Git(repo, "add", "tracked")
	if !changed("staged") {
		t.Error("a staged change does not count")
	}
}

func TestInProgress(t *testing.T) {
	sb := testutil.New(t)
	repo, feat, base := diverged(t, sb, true)

	op := func(dir string) string {
		t.Helper()
		got, err := git.InProgress(ctx, runner(sb), dir)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := op(feat); got != "" {
		t.Fatalf("clean worktree: %q", got)
	}

	if err := git.Rebase(ctx, runner(sb), feat, base); err == nil {
		t.Fatal("rebase with a conflict succeeded")
	}
	if got := op(feat); got != "rebase" {
		t.Errorf("linked worktree with a stopped rebase: %q, want rebase", got)
	}
	if got := op(repo); got != "" {
		t.Errorf("main worktree, whose git dir is another: %q, want none", got)
	}
	if err := git.RebaseAbort(ctx, runner(sb), feat); err != nil {
		t.Fatal(err)
	}

	// A merge stopped on a conflict, in the main worktree.
	sb.Git(repo, "checkout", "-q", "-b", "other", base+"~1")
	commitFile(sb, repo, "f.txt", "other\n")
	cmd := []string{"merge", "-q", base}
	if _, err := runner(sb).Run(ctx, repo, cmd...); err == nil {
		t.Fatal("merge with a conflict succeeded")
	}
	if got := op(repo); got != "merge" {
		t.Errorf("main worktree with a stopped merge: %q, want merge", got)
	}
}

func TestRebase(t *testing.T) {
	sb := testutil.New(t)
	_, feat, base := diverged(t, sb, false)
	if err := git.Rebase(ctx, runner(sb), feat, base); err != nil {
		t.Fatal(err)
	}
	head := sb.Git(feat, "rev-parse", "HEAD")
	if ok, err := git.IsAncestor(ctx, runner(sb), feat, base, head); err != nil || !ok {
		t.Errorf("base is not an ancestor of feat after the rebase (%v)", err)
	}
	if b, err := os.ReadFile(filepath.Join(feat, "g.txt")); err != nil || string(b) != "feat\n" {
		t.Errorf("feat lost its own commit: %q, %v", b, err)
	}
}

func TestRebaseConflictAndAbort(t *testing.T) {
	sb := testutil.New(t)
	_, feat, base := diverged(t, sb, true)
	before := sb.Git(feat, "rev-parse", "HEAD")

	if err := git.Rebase(ctx, runner(sb), feat, base); err == nil {
		t.Fatal("rebase with a conflict succeeded")
	}
	if ok, err := git.Unmerged(ctx, runner(sb), feat); err != nil || !ok {
		t.Fatalf("Unmerged = %v, %v; want true", ok, err)
	}
	if err := git.RebaseAbort(ctx, runner(sb), feat); err != nil {
		t.Fatal(err)
	}
	if after := sb.Git(feat, "rev-parse", "HEAD"); after != before {
		t.Errorf("HEAD after abort = %s, want %s", after, before)
	}
	if b, _ := os.ReadFile(filepath.Join(feat, "f.txt")); string(b) != "feat\n" {
		t.Errorf("f.txt after abort = %q", b)
	}
	if ok, _ := git.Unmerged(ctx, runner(sb), feat); ok {
		t.Error("unmerged paths left after abort")
	}
}

func TestRebaseNeverStashes(t *testing.T) {
	sb := testutil.New(t)
	repo, feat, base := diverged(t, sb, false)
	sb.Git(repo, "config", "rebase.autoStash", "true")
	sb.WriteFile(filepath.Join(feat, "g.txt"), "dirty\n")

	if err := git.Rebase(ctx, runner(sb), feat, base); err == nil {
		t.Fatal("rebase of a dirty worktree succeeded")
	}
	if got := sb.Git(feat, "stash", "list"); got != "" {
		t.Errorf("stash list = %q, want empty", got)
	}
	if b, _ := os.ReadFile(filepath.Join(feat, "g.txt")); string(b) != "dirty\n" {
		t.Errorf("g.txt = %q, want the uncommitted change", b)
	}
}

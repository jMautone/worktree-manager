package git_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// pending makes repo, on main, with the linked worktree feat, which has a
// modified tracked file, an untracked file and an ignored one.
func pending(t *testing.T, sb *testutil.Sandbox) (repo, feat string) {
	t.Helper()
	repo = sb.Path("repo")
	sb.InitRepo(repo)
	commitFile(sb, repo, ".gitignore", "*.log\n")
	commitFile(sb, repo, "a.txt", "a\n")
	feat = sb.Path("feat")
	sb.AddWorktree(repo, feat, "feat")
	sb.WriteFile(filepath.Join(feat, "a.txt"), "changed\n")
	sb.WriteFile(filepath.Join(feat, "new.txt"), "new\n")
	sb.WriteFile(filepath.Join(feat, "debug.log"), "log\n")
	return repo, feat
}

// files lists the files of commit.
func files(sb *testutil.Sandbox, dir, commit string) string {
	return sb.Git(dir, "ls-tree", "-r", "--name-only", commit)
}

// rejectCommits installs a commit-msg hook in repo that rejects every
// commit.
func rejectCommits(sb *testutil.Sandbox, repo string) {
	sb.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"), "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "commit-msg"), 0o755); err != nil {
		sb.Git(repo, "status")
	}
}

func commitAll(t *testing.T, sb *testutil.Sandbox, dir, msg string) (git.Snapshot, error) {
	t.Helper()
	snap, err := git.TakeSnapshot(ctx, runner(sb), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := git.StageAll(ctx, runner(sb), dir); err != nil {
		t.Fatal(err)
	}
	return snap, git.Commit(ctx, runner(sb), dir, msg, false)
}

func TestCommitAllPendingChanges(t *testing.T) {
	sb := testutil.New(t)
	_, feat := pending(t, sb)
	old := sb.Git(feat, "rev-parse", "HEAD")
	if _, err := commitAll(t, sb, feat, "fix login"); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(feat, "rev-parse", "HEAD^"); got != old {
		t.Errorf("parent = %s, want %s", got, old)
	}
	if got := sb.Git(feat, "log", "-1", "--format=%B"); got != "fix login" {
		t.Errorf("message = %q", got)
	}
	list := files(sb, feat, "HEAD")
	if !strings.Contains(list, "new.txt") || strings.Contains(list, "debug.log") {
		t.Errorf("files = %q, want new.txt and no debug.log", list)
	}
	if _, err := os.Stat(filepath.Join(feat, "debug.log")); err != nil {
		t.Error("debug.log is gone")
	}
}

func TestCommitRejectedRestoresTheIndex(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	commitFile(sb, repo, "a.txt", "a\n")
	commitFile(sb, repo, "b.txt", "b\n")
	sb.WriteFile(filepath.Join(repo, "a.txt"), "staged\n")
	sb.Git(repo, "add", "a.txt")
	sb.WriteFile(filepath.Join(repo, "b.txt"), "unstaged\n")
	rejectCommits(sb, repo)
	old := sb.Git(repo, "rev-parse", "HEAD")
	before := sb.Git(repo, "status", "--porcelain")

	snap, err := commitAll(t, sb, repo, "x")
	if err == nil {
		t.Fatal("the commit was not rejected")
	}
	if err := snap.Restore(ctx, runner(sb), repo, false); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(repo, "rev-parse", "HEAD"); got != old {
		t.Errorf("HEAD moved to %s", got)
	}
	if got := sb.Git(repo, "status", "--porcelain"); got != before {
		t.Errorf("status = %q, want %q", got, before)
	}
}

// threeCommits makes the worktree feat with three commits main lacks.
func threeCommits(t *testing.T, sb *testutil.Sandbox) (repo, feat, base string) {
	t.Helper()
	repo = sb.Path("repo")
	sb.InitRepo(repo)
	base = commitFile(sb, repo, "a.txt", "a\n")
	feat = sb.Path("feat")
	sb.AddWorktree(repo, feat, "feat")
	commitFile(sb, feat, "f1.txt", "1\n")
	commitFile(sb, feat, "f2.txt", "2\n")
	commitFile(sb, feat, "f3.txt", "3\n")
	return repo, feat, base
}

func TestSquash(t *testing.T) {
	sb := testutil.New(t)
	_, feat, base := threeCommits(t, sb)
	r := runner(sb)
	old := sb.Git(feat, "rev-parse", "HEAD")
	mb, err := git.MergeBase(ctx, r, feat, "main", "HEAD")
	if err != nil || mb != base {
		t.Fatalf("MergeBase = %s, %v; want %s", mb, err, base)
	}
	if n, err := git.CountCommits(ctx, r, feat, mb, "HEAD"); err != nil || n != 3 {
		t.Fatalf("CountCommits = %d, %v", n, err)
	}
	msgs, err := git.Messages(ctx, r, feat, mb, "HEAD")
	if err != nil || strings.Join(msgs, "|") != "edit f1.txt|edit f2.txt|edit f3.txt" {
		t.Fatalf("Messages = %q, %v", msgs, err)
	}
	snap, _ := git.TakeSnapshot(ctx, r, feat)
	if err := git.ResetSoft(ctx, r, feat, mb); err != nil {
		t.Fatal(err)
	}
	if err := git.Commit(ctx, r, feat, "add files", true); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(feat, "rev-parse", "HEAD^"); got != base {
		t.Errorf("parent = %s, want %s", got, base)
	}
	if got, want := sb.Git(feat, "rev-parse", "HEAD^{tree}"), sb.Git(feat, "rev-parse", old+"^{tree}"); got != want {
		t.Errorf("tree = %s, want the old HEAD's %s", got, want)
	}
	if snap.Head != old {
		t.Errorf("snapshot HEAD = %s", snap.Head)
	}
}

func TestSquashRejectedGoesBack(t *testing.T) {
	sb := testutil.New(t)
	repo, feat, base := threeCommits(t, sb)
	rejectCommits(sb, repo)
	r := runner(sb)
	old := sb.Git(feat, "rev-parse", "HEAD")
	snap, _ := git.TakeSnapshot(ctx, r, feat)
	if err := git.ResetSoft(ctx, r, feat, base); err != nil {
		t.Fatal(err)
	}
	if err := git.Commit(ctx, r, feat, "x", true); err == nil {
		t.Fatal("the commit was not rejected")
	}
	if err := snap.Restore(ctx, r, feat, true); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(feat, "rev-parse", "HEAD"); got != old {
		t.Errorf("HEAD = %s, want %s", got, old)
	}
	if got := sb.Git(feat, "status", "--porcelain"); got != "" {
		t.Errorf("status = %q, want clean", got)
	}
}

// editor sets GIT_EDITOR to cmd, a shell command git runs with the file to
// edit as its argument, through its own sh on every OS.
func editor(sb *testutil.Sandbox, cmd string) git.Terminal {
	sb.Setenv("GIT_EDITOR", cmd)
	return git.Terminal{Env: sb.Environ(), Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}
}

func TestCommitEditWrites(t *testing.T) {
	sb := testutil.New(t)
	_, feat := pending(t, sb)
	term := editor(sb, `f() { echo "from editor" > "$1"; }; f`)
	_ = git.StageAll(ctx, runner(sb), feat)
	if err := git.CommitEdit(ctx, runner(sb), term, feat, "", false); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(feat, "log", "-1", "--format=%B"); got != "from editor" {
		t.Errorf("message = %q", got)
	}
}

func TestCommitEditEmpty(t *testing.T) {
	sb := testutil.New(t)
	_, feat := pending(t, sb)
	old := sb.Git(feat, "rev-parse", "HEAD")
	term := editor(sb, `f() { : > "$1"; }; f`)
	_ = git.StageAll(ctx, runner(sb), feat)
	if err := git.CommitEdit(ctx, runner(sb), term, feat, "", false); !errors.Is(err, git.ErrEmptyMessage) {
		t.Fatalf("err = %v, want ErrEmptyMessage", err)
	}
	if got := sb.Git(feat, "rev-parse", "HEAD"); got != old {
		t.Error("HEAD moved")
	}
}

func TestCommitEditInitialMessage(t *testing.T) {
	sb := testutil.New(t)
	_, feat := pending(t, sb)
	copied := filepath.ToSlash(sb.Path("msg.txt"))
	term := editor(sb, `f() { cp "$1" "`+copied+`"; }; f`)
	_ = git.StageAll(ctx, runner(sb), feat)
	if err := git.CommitEdit(ctx, runner(sb), term, feat, "# comment\n\nadd form\n\nfix typo\n", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(sb.Path("msg.txt"))
	if err != nil || !strings.Contains(string(b), "add form\n\nfix typo") {
		t.Errorf("the editor opened with %q, %v", b, err)
	}
	if got := sb.Git(feat, "log", "-1", "--format=%B"); got != "add form\n\nfix typo" {
		t.Errorf("message = %q", got)
	}
}

// A hook that rejects the commit is a failure of git, not an empty
// message, even with the message of an earlier commit left behind.
func TestCommitEditRejectedIsNotEmpty(t *testing.T) {
	sb := testutil.New(t)
	repo, feat := pending(t, sb)
	editmsg := sb.Git(feat, "rev-parse", "--git-path", "COMMIT_EDITMSG")
	if !filepath.IsAbs(editmsg) {
		editmsg = filepath.Join(feat, editmsg)
	}
	sb.WriteFile(editmsg, "\n")
	rejectCommits(sb, repo)
	term := editor(sb, `f() { echo "from editor" > "$1"; }; f`)
	_ = git.StageAll(ctx, runner(sb), feat)
	err := git.CommitEdit(ctx, runner(sb), term, feat, "", false)
	var ce *git.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a CommandError", err)
	}
}

func TestBlank(t *testing.T) {
	for msg, want := range map[string]bool{
		"":                       true,
		"\n\n":                   true,
		"# a comment\n  \n# b\n": true,
		"x":                      false,
		"# c\nfix\n":             false,
	} {
		if got := git.Blank(msg); got != want {
			t.Errorf("Blank(%q) = %v, want %v", msg, got, want)
		}
	}
}

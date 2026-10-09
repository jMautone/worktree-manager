package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxCreatesEveryWorktreeKind(t *testing.T) {
	sb := New(t)

	repo := sb.Path("repo")
	sb.InitRepo(repo)
	linked := sb.Path("linked")
	sb.AddWorktree(repo, linked, "feature/abc1")
	detached := sb.Path("detached")
	sb.AddDetachedWorktree(repo, detached)
	locked := sb.Path("locked")
	sb.AddWorktree(repo, locked, "locked-branch")
	sb.LockWorktree(repo, locked, "on usb drive")
	prunable := sb.Path("prunable")
	sb.AddWorktree(repo, prunable, "prunable-branch")
	sb.MakePrunable(prunable)

	out := sb.Git(repo, "worktree", "list", "--porcelain") + "\n"
	for _, want := range []string{
		"worktree " + filepath.ToSlash(Comparable(t, repo)) + "\n",
		"worktree " + filepath.ToSlash(Comparable(t, linked)) + "\n",
		"branch refs/heads/feature/abc1\n",
		"worktree " + filepath.ToSlash(Comparable(t, detached)) + "\n",
		"detached\n",
		"worktree " + filepath.ToSlash(Comparable(t, locked)) + "\n",
		"locked on usb drive\n",
		"worktree " + filepath.ToSlash(Comparable(t, prunable)) + "\n",
		"prunable ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("git worktree list does not contain %q:\n%s", want, out)
		}
	}
}

func TestSandboxCreatesBareRepositoryWithLinkedWorktree(t *testing.T) {
	sb := New(t)

	bare := sb.Path("bare.git")
	sb.InitBare(bare)
	linked := sb.Path("bare-linked")
	sb.AddWorktree(bare, linked, "feature/x")

	out := sb.Git(bare, "worktree", "list", "--porcelain") + "\n"
	for _, want := range []string{
		"worktree " + filepath.ToSlash(Comparable(t, bare)) + "\nbare\n",
		"worktree " + filepath.ToSlash(Comparable(t, linked)) + "\n",
		"branch refs/heads/feature/x\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("git worktree list does not contain %q:\n%s", want, out)
		}
	}
}

func TestSandboxIsolatesGitAndWtConfiguration(t *testing.T) {
	sb := New(t)

	if got := sb.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Errorf("GIT_CONFIG_NOSYSTEM = %q, want 1", got)
	}
	for _, key := range []string{"GIT_CONFIG_GLOBAL", "HOME", "USERPROFILE", "XDG_CONFIG_HOME", "APPDATA"} {
		if got := sb.Getenv(key); !strings.HasPrefix(got, sb.Root) {
			t.Errorf("%s = %q, want a path inside %q", key, got, sb.Root)
		}
	}
	if got := sb.Getenv("WT_CONFIG"); got != "" {
		t.Errorf("WT_CONFIG = %q, want unset", got)
	}
	if got := sb.Git(sb.Root, "config", "--global", "user.email"); got != "wt@example.com" {
		t.Errorf("global user.email = %q, want the sandbox's", got)
	}

	sb.Setenv("WT_CONFIG", "/x/wt.toml")
	found := false
	for _, kv := range sb.Environ() {
		if kv == "WT_CONFIG=/x/wt.toml" {
			found = true
		}
	}
	if !found {
		t.Error("Setenv is not reflected in Environ")
	}
}

func TestComparableResolvesSymlinkedTempDir(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" && strings.HasPrefix(dir, "/var/") && !strings.HasPrefix(real, "/private/var/") {
		t.Fatalf("expected macOS temp dir %q to resolve under /private/var, got %q", dir, real)
	}

	if a, b := Comparable(t, dir), Comparable(t, real); a != b {
		t.Errorf("Comparable(%q) = %q, Comparable(%q) = %q; want equal", dir, a, real, b)
	}
}

func TestComparableResolvesMissingPathThroughExistingAncestor(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := Comparable(t, filepath.Join(dir, "gone", "deeper"))
	want := filepath.Join(real, "gone", "deeper")
	if got != want {
		t.Errorf("Comparable of a missing path = %q, want %q", got, want)
	}
}

func TestComparableUsesNativeSeparators(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Comparable(t, filepath.ToSlash(dir)+"/sub")
	want := Comparable(t, filepath.Join(dir, "sub"))
	if got != want {
		t.Errorf("Comparable with forward slashes = %q, want %q", got, want)
	}
}

func TestSandboxRemoteHelpers(t *testing.T) {
	sb := New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)

	if got := sb.Git(clone, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); got != "origin/main" {
		t.Errorf("origin/HEAD = %q, want origin/main", got)
	}

	id := sb.Commit(clone, "local")
	if got := sb.Git(clone, "rev-parse", "HEAD"); got != id || len(id) < 40 {
		t.Errorf("Commit returned %q, HEAD is %q", id, got)
	}
	sb.Push(clone, "origin", "HEAD:refs/heads/topic")
	if got := sb.Git(origin, "rev-parse", "refs/heads/topic"); got != id {
		t.Errorf("origin topic = %q, want %q", got, id)
	}

	pushed := sb.PushFromAnotherClone(origin, "main")
	if got := sb.Git(origin, "rev-parse", "refs/heads/main"); got != pushed {
		t.Errorf("origin main = %q, want the commit from the other clone %q", got, pushed)
	}
	if got := sb.Git(clone, "rev-parse", "refs/remotes/origin/main"); got == pushed {
		t.Error("the clone saw the push without fetching")
	}

	sb.Git(clone, "fetch", "-q", "origin")
	if got := sb.Git(clone, "rev-parse", "refs/remotes/origin/main"); got != pushed {
		t.Errorf("after fetch origin/main = %q, want %q", got, pushed)
	}

	sb.BreakRemote(clone, "origin")
	cmd := exec.Command("git", "fetch", "-q", "origin")
	cmd.Dir = clone
	cmd.Env = sb.Environ()
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("fetch from the broken remote succeeded: %s", out)
	}
}

func TestSandboxBranchHelpers(t *testing.T) {
	sb := New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)

	sb.Git(clone, "checkout", "-q", "-b", "feat")
	tip := sb.Commit(clone, "feat work")
	sb.Push(clone, "origin", "feat")
	sb.SetUpstream(clone, "feat", "origin/feat")
	if got := sb.Git(clone, "for-each-ref", "--format=%(upstream:short)", "refs/heads/feat"); got != "origin/feat" {
		t.Errorf("upstream of feat = %q, want origin/feat", got)
	}

	sb.Git(clone, "checkout", "-q", "main")
	squash := sb.SquashMerge(clone, "feat")
	if got := sb.Git(clone, "rev-parse", "main"); got != squash || squash == tip {
		t.Errorf("main = %q, squash commit %q, feat %q", got, squash, tip)
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", tip, squash)
	cmd.Dir = clone
	cmd.Env = sb.Environ()
	if err := cmd.Run(); err == nil {
		t.Error("the squash commit has the branch's tip as an ancestor")
	}
}

func TestSandboxReadOnlyDir(t *testing.T) {
	sb := New(t)
	dir := sb.Path("ro")
	sb.WriteFile(filepath.Join(dir, "kept"), "")
	sb.ReadOnlyDir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "new"), nil, 0o644); err == nil {
		t.Error("a file was created in the read-only directory")
	}
	if err := os.Remove(filepath.Join(dir, "kept")); err == nil {
		t.Error("a file was deleted from the read-only directory")
	}
}

func TestSandboxCreatesTheDotBareLayout(t *testing.T) {
	sb := New(t)
	proj := sb.Path("proj")
	sb.InitDotBare(proj)

	if b, err := os.ReadFile(filepath.Join(proj, ".git")); err != nil || string(b) != "gitdir: ./.bare\n" {
		t.Fatalf(".git = %q, %v", b, err)
	}
	// git reports the bare repository as the main worktree.
	out := sb.Git(proj, "worktree", "list", "--porcelain") + "\n"
	if want := "worktree " + filepath.ToSlash(Comparable(t, filepath.Join(proj, ".bare"))) + "\nbare\n"; !strings.Contains(out, want) {
		t.Errorf("git worktree list does not contain %q:\n%s", want, out)
	}
	linked := sb.Path("proj", "feat")
	sb.AddWorktree(proj, linked, "feat")
	if got := sb.Git(linked, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat" {
		t.Errorf("HEAD of the linked worktree = %q", got)
	}
}

func TestSandboxCreatesASeparateGitDir(t *testing.T) {
	sb := New(t)
	dir, gitdir := sb.Path("sep"), sb.Path("store", "sep.git")
	sb.InitSeparateGitDir(dir, gitdir)

	info, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf(".git is not a file: %v, %v", info, err)
	}
	if got := sb.Git(dir, "rev-parse", "--git-dir"); Comparable(t, got) != Comparable(t, gitdir) {
		t.Errorf("git dir = %q, want %q", got, gitdir)
	}
	if got := sb.Git(dir, "log", "--format=%s"); got != "initial commit" {
		t.Errorf("log = %q", got)
	}
}

func TestSandboxCreatesLinks(t *testing.T) {
	sb := New(t)
	target := sb.Path("target")
	sb.WriteFile(filepath.Join(target, "f"), "x")

	link := sb.Path("link")
	if runtime.GOOS == "windows" {
		sb.Junction(target, link)
	} else {
		sb.Symlink(target, link)
	}
	if b, err := os.ReadFile(filepath.Join(link, "f")); err != nil || string(b) != "x" {
		t.Errorf("reading through the link: %q, %v", b, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.IsDir() {
		t.Errorf("Lstat(link) = %v, %v; want a link, not a directory", info, err)
	}
}

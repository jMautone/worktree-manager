package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

var ctx = context.Background()

func runner(sb *testutil.Sandbox) git.Runner { return git.Exec{Env: sb.Environ()} }

func TestCheckBranchName(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	// With a previous branch, git expands @{-1} to its name.
	sb.Git(repo, "checkout", "-q", "-b", "other")
	sb.Git(repo, "checkout", "-q", "main")

	for _, name := range []string{"feat", "feature/abc1", "feat<x>", "a|b", "jm/auth"} {
		if err := git.CheckBranchName(ctx, runner(sb), repo, name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"a b", "@{-1}", "-x", "HEAD", ".x", "x.lock", "", "jm/a b"} {
		var e *git.InvalidBranchError
		if err := git.CheckBranchName(ctx, runner(sb), repo, name); !errors.As(err, &e) || e.Name != name {
			t.Errorf("%q: %v, want an *InvalidBranchError", name, err)
		}
	}
}

func TestOriginHEADAndRemotes(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)
	plain := sb.Path("plain")
	sb.InitRepo(plain)
	bare := sb.Path("bare.git")
	sb.Git(sb.Root, "clone", "-q", "--bare", origin, bare)

	for _, tc := range []struct {
		dir, originHEAD, head string
		remotes               []string
	}{
		{clone, "origin/main", "main", []string{"origin"}},
		{plain, "", "main", nil},
		// A bare clone copies refs/heads only: no origin/HEAD.
		{bare, "", "main", []string{"origin"}},
	} {
		got, err := git.OriginHEAD(ctx, runner(sb), tc.dir)
		if err != nil || got != tc.originHEAD {
			t.Errorf("%s: OriginHEAD = %q, %v; want %q", tc.dir, got, err, tc.originHEAD)
		}
		head, err := git.SymbolicHEAD(ctx, runner(sb), tc.dir)
		if err != nil || head != tc.head {
			t.Errorf("%s: SymbolicHEAD = %q, %v; want %q", tc.dir, head, err, tc.head)
		}
		remotes, err := git.Remotes(ctx, runner(sb), tc.dir)
		if err != nil || !slices.Equal(remotes, tc.remotes) {
			t.Errorf("%s: Remotes = %q, %v; want %q", tc.dir, remotes, err, tc.remotes)
		}
	}

	sb.Git(plain, "checkout", "-q", "--detach")
	if head, err := git.SymbolicHEAD(ctx, runner(sb), plain); err != nil || head != "" {
		t.Errorf("detached: SymbolicHEAD = %q, %v; want empty", head, err)
	}
}

func TestRefs(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)
	sb.Git(clone, "branch", "feat")
	sb.Git(clone, "tag", "v1")

	refs, err := git.Refs(ctx, runner(sb), clone, "refs/heads", "refs/remotes")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"refs/heads/main", "refs/heads/feat", "refs/remotes/origin/main", "refs/remotes/origin/HEAD"} {
		if !slices.Contains(refs, want) {
			t.Errorf("refs %q do not contain %s", refs, want)
		}
	}
	if slices.Contains(refs, "refs/tags/v1") {
		t.Errorf("refs %q contain a tag", refs)
	}
	if refs, err := git.Refs(ctx, runner(sb), clone, "refs/heads/nope"); err != nil || len(refs) != 0 {
		t.Errorf("no match: %q, %v", refs, err)
	}
}

func TestResolveCommit(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	head := sb.Git(repo, "rev-parse", "HEAD")
	sb.Git(repo, "tag", "-a", "-m", "annotated", "v1")

	for _, rev := range []string{"main", "HEAD", "v1", head, head[:7]} {
		got, err := git.ResolveCommit(ctx, runner(sb), repo, rev)
		if err != nil || got != head {
			t.Errorf("%q: %q, %v; want %s", rev, got, err, head)
		}
	}
	for _, rev := range []string{"nope", "-x", "--all", "origin/main"} {
		if got, err := git.ResolveCommit(ctx, runner(sb), repo, rev); !errors.Is(err, git.ErrNoRevision) {
			t.Errorf("%q: %q, %v; want ErrNoRevision", rev, got, err)
		}
	}
}

func TestFetch(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)
	pushed := sb.PushFromAnotherClone(origin, "main")

	if err := git.Fetch(ctx, runner(sb), clone, "origin"); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(clone, "rev-parse", "origin/main"); got != pushed {
		t.Errorf("origin/main = %s, want the pushed commit %s", got, pushed)
	}

	sb.BreakRemote(clone, "origin")
	var ce *git.CommandError
	if err := git.Fetch(ctx, runner(sb), clone, "origin"); !errors.As(err, &ce) || ce.Stderr == "" {
		t.Errorf("unreachable remote: %v, want a *CommandError with git's message", err)
	}
}

func TestCreateBranchHasNoUpstream(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	clone := sb.Path("clone")
	sb.Clone(origin, clone)
	head := sb.Git(clone, "rev-parse", "origin/main")

	// The name of the base, not only its commit: git would set an upstream.
	if err := git.CreateBranch(ctx, runner(sb), clone, "feat", "origin/main"); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(clone, "rev-parse", "feat"); got != head {
		t.Errorf("feat = %s, want %s", got, head)
	}
	if got := sb.Git(clone, "for-each-ref", "--format=%(upstream)", "refs/heads/feat"); got != "" {
		t.Errorf("feat has the upstream %q, want none", got)
	}
	if err := git.CreateBranch(ctx, runner(sb), clone, "feat", head); err == nil {
		t.Error("created a branch that already exists")
	}
	if err := git.DeleteBranch(ctx, runner(sb), clone, "feat"); err != nil {
		t.Fatal(err)
	}
	if refs, _ := git.Refs(ctx, runner(sb), clone, "refs/heads/feat"); len(refs) != 0 {
		t.Errorf("feat still exists: %q", refs)
	}
}

func TestAddWorktree(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	sb.Git(repo, "branch", "feat")
	sb.Git(repo, "branch", "fix")

	dest := sb.Path("wt", "repo", "feat")
	if err := git.AddWorktree(ctx, runner(sb), repo, dest, "feat"); err != nil {
		t.Fatal(err)
	}
	if got := sb.Git(dest, "branch", "--show-current"); got != "feat" {
		t.Errorf("branch in %s = %q, want feat", dest, got)
	}

	sb.WriteFile(sb.Path("blocker"), "")
	err := git.AddWorktree(ctx, runner(sb), repo, sb.Path("blocker", "fix"), "fix")
	var ce *git.CommandError
	if !errors.As(err, &ce) || ce.Stderr == "" {
		t.Errorf("parent is a file: %v, want a *CommandError with git's message", err)
	}
	if _, err := os.Stat(filepath.Join(sb.Path("blocker"), "fix")); err == nil {
		t.Error("something was created under the file")
	}
}

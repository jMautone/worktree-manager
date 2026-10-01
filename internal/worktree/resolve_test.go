package worktree

import (
	"errors"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
)

const head = "0123456789abcdef0123456789abcdef01234567"

func TestResolve(t *testing.T) {
	// /src/repo (main, current from /src/repo/sub), and its linked worktrees.
	repo := []git.WorktreeEntry{
		{Path: "/src/repo", Head: head, Branch: "main"},
		{Path: "/src/repo.worktrees/feat", Head: head, Branch: "feat"},
		{Path: "/src/repo.worktrees/feature-abc1", Head: head, Branch: "feature/abc1"},
		{Path: "/src/repo.worktrees/api", Head: head, Branch: "fix"},
		{Path: "/src/repo.worktrees/fix", Head: head, Branch: "hotfix"},
		{Path: "/src/repo.worktrees/detached", Head: head, Detached: true},
		{Path: "/a/x", Head: head, Branch: "feature/x"},
		{Path: "/b/x", Head: head, Branch: "fix/x"},
	}
	for _, tc := range []struct {
		name   string
		target string
		want   string // path of the destination; "" when an error is expected
		err    func(error) bool
	}{
		{"by name", "feat", "/src/repo.worktrees/feat", nil},
		{"by branch with a slash", "feature/abc1", "/src/repo.worktrees/feature-abc1", nil},
		{"name before branch", "fix", "/src/repo.worktrees/fix", nil},
		{"branch when no name matches", "hotfix", "/src/repo.worktrees/fix", nil},
		{"main", "^", "/src/repo", nil},
		{"current", "@", "/src/repo", nil},
		{"main by its name", "repo", "/src/repo", nil},
		{"ambiguous name", "x", "", isAmbiguous("/a/x", "/b/x")},
		{"no such worktree", "nope", "", isNotFound("nope")},
		{"case differs", "Feat", "", isNotFound("Feat")},
		{"empty target is not the branch of a detached worktree", "", "", isNotFound("")},
		{"a path is not a name", "repo.worktrees/feat", "", isNotFound("repo.worktrees/feat")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := Build(repo, "/src/repo/sub", "linux")
			got, err := Resolve(ws, tc.target)
			switch {
			case tc.err == nil && err != nil:
				t.Fatalf("Resolve(%q): %v", tc.target, err)
			case tc.err == nil && got.Path != tc.want:
				t.Errorf("Resolve(%q) = %s, want %s", tc.target, got.Path, tc.want)
			case tc.err != nil && !tc.err(err):
				t.Errorf("Resolve(%q) = %s, %v; not the expected error", tc.target, got.Path, err)
			}
		})
	}
}

func TestResolveIsCaseSensitiveOnEveryOS(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		ws := Build([]git.WorktreeEntry{
			{Path: "C:/src/repo", Head: head, Branch: "main"},
			{Path: "C:/src/repo.worktrees/feat", Head: head, Branch: "feature/Feat"},
		}, "C:/src/repo", goos)
		for _, target := range []string{"Feat", "FEAT", "feature/feat", "Main", "REPO"} {
			if w, err := Resolve(ws, target); !isNotFound(target)(err) {
				t.Errorf("%s: Resolve(%q) = %s, %v; want not found", goos, target, w.Path, err)
			}
		}
		if w, err := Resolve(ws, "feat"); err != nil || w.Name != "feat" {
			t.Errorf("%s: Resolve(feat) = %+v, %v", goos, w, err)
		}
	}
}

func TestResolveBareRepositoryAsMain(t *testing.T) {
	ws := Build([]git.WorktreeEntry{
		{Path: "/a/bare.git", Bare: true},
		{Path: "/a/wt", Head: head, Branch: "main"},
	}, "/a/wt", "linux")
	w, err := Resolve(ws, "^")
	if err != nil || w.Path != "/a/bare.git" || !w.Bare {
		t.Errorf("Resolve(^) = %+v, %v; want the bare repository", w, err)
	}
}

func TestResolveCurrentWhenNoneIs(t *testing.T) {
	// Inside the repository's git directory of a linked worktree, for
	// example: git answers, but no worktree path contains the directory.
	ws := Build([]git.WorktreeEntry{
		{Path: "/a/repo", Head: head, Branch: "main"},
		{Path: "/a/wt", Head: head, Branch: "x"},
	}, "/elsewhere", "linux")
	if w, err := Resolve(ws, "@"); !errors.Is(err, ErrNoCurrent) {
		t.Errorf("Resolve(@) = %+v, %v; want ErrNoCurrent", w, err)
	}
	if err := ErrNoCurrent; err.Error() != "not inside a worktree" {
		t.Errorf("ErrNoCurrent = %q", err)
	}
}

func TestResolveErrorMessages(t *testing.T) {
	if got := (&NotFoundError{Target: "nope"}).Error(); got != `no worktree named "nope"` {
		t.Errorf("NotFoundError = %q", got)
	}
	if got := (&AmbiguousError{Target: "x"}).Error(); got != `"x" matches more than one worktree` {
		t.Errorf("AmbiguousError = %q", got)
	}
}

func isNotFound(target string) func(error) bool {
	return func(err error) bool {
		var nf *NotFoundError
		return errors.As(err, &nf) && nf.Target == target
	}
}

func isAmbiguous(paths ...string) func(error) bool {
	return func(err error) bool {
		var ae *AmbiguousError
		if !errors.As(err, &ae) || len(ae.Candidates) != len(paths) {
			return false
		}
		for i, p := range paths {
			if ae.Candidates[i].Path != p {
				return false
			}
		}
		return true
	}
}

package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

func TestListWorktreesFromRealRepository(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	linked := sb.Path("linked")
	sb.AddWorktree(repo, linked, "feature/abc1")

	entries, err := git.ListWorktrees(context.Background(), git.Exec{Env: sb.Environ()}, linked)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if got := testutil.Comparable(t, entries[1].Path); got != testutil.Comparable(t, linked) || entries[1].Branch != "feature/abc1" {
		t.Errorf("linked entry = %+v", entries[1])
	}
}

func TestRunnerGitMissing(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := git.Exec{Env: []string{"PATH="}}.Run(context.Background(), t.TempDir(), "version")
	if !errors.Is(err, git.ErrNotInstalled) {
		t.Errorf("err = %v, want ErrNotInstalled", err)
	}
}

func TestRunnerNotARepository(t *testing.T) {
	sb := testutil.New(t)
	outside := sb.Path("outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := git.ListWorktrees(context.Background(), git.Exec{Env: sb.Environ()}, outside)
	var nre *git.NotRepoError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NotRepoError", err)
	}
	if nre.Dir != outside {
		t.Errorf("NotRepoError.Dir = %q, want %q", nre.Dir, outside)
	}
}

// A user whose git speaks another language must still get exit 3, which is
// detected from git's message.
func TestRunnerNotARepositoryWithLocalizedEnvironment(t *testing.T) {
	sb := testutil.New(t)
	sb.Setenv("LANG", "es_ES.UTF-8")
	sb.Setenv("LC_ALL", "es_ES.UTF-8")
	sb.Setenv("LANGUAGE", "es")
	_, err := git.ListWorktrees(context.Background(), git.Exec{Env: sb.Environ()}, sb.Root)
	var nre *git.NotRepoError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NotRepoError", err)
	}
}

func TestRunnerOtherFailureCarriesGitStderr(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	// A corrupt config makes every git command in the repository fail.
	sb.WriteFile(filepath.Join(repo, ".git", "config"), "[core\nbroken")

	_, err := git.ListWorktrees(context.Background(), git.Exec{Env: sb.Environ()}, repo)
	var ce *git.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if !strings.Contains(ce.Stderr, "bad config") {
		t.Errorf("Stderr = %q, want git's own message", ce.Stderr)
	}
	if !strings.Contains(err.Error(), "bad config") {
		t.Errorf("Error() = %q does not include git's message", err.Error())
	}
}

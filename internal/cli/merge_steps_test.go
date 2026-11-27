package cli_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
)

// Scenarios of merge-worktree that commit or squash, and the merge
// scenarios of hooks and project-hooks-approval about the commit events.

// Scenario "Squash without committing".
func TestMergeSquashWithoutCommitting(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.run("merge", "--squash", "--no-commit", "-m", "x").mustCode(t, 5)
	if h.head(feat) != own {
		t.Error("feat moved")
	}
}

// Scenario "Message needed in a script".
func TestMergeMessageNeeded(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	main, own := h.head(repo), h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("merge")
	r.mustCode(t, 2)
	if got := firstLine(r.stderr); got != "wt: a commit message is needed" {
		t.Errorf("stderr's first line = %q", got)
	}
	if h.head(repo) != main || h.head(feat) != own {
		t.Error("a branch moved")
	}
}

// Scenario "Merge checks the commit events" of project-hooks-approval.
func TestMergeChecksTheCommitEvents(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.repoHooks(feat, hookLine("pre-commit", "npm run lint"))
	own := h.head(feat)
	mustNotApproved(t, h.run("merge", "-m", "x"), "pre-commit", "npm run lint")
	if h.head(feat) != own || !strings.Contains(h.status(feat), ".wt.toml") {
		t.Error("feat moved, or its pending change is gone")
	}
}

// Scenario "Clean merge does not check the commit events".
func TestMergeCleanDoesNotCheckTheCommitEvents(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.repoHooks(feat, hookLine("pre-commit", "npm run lint"))
	h.sb.Git(feat, "add", ".wt.toml")
	h.sb.Git(feat, "commit", "-q", "-m", "hooks")
	h.run("merge").mustCode(t, 0)
}

// Scenario "Pending change committed and merged".
func TestMergeCommitsPendingChanges(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	directive := h.activate()
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("merge", "-m", "finish")
	r.mustCode(t, 0)
	lines := stdoutLines(r)
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "committed ") || !strings.HasSuffix(lines[0], " to feat") ||
		lines[1] != "merged feat into main" || lines[2] != "removed worktree "+feat {
		t.Errorf("stdout = %q", lines)
	}
	if h.message(repo, "main") != "finish" {
		t.Errorf("main's HEAD = %q", h.message(repo, "main"))
	}
	h.mustBeRemoved(repo, feat)
	mustDirective(t, directive, repo)
}

// Scenario "Empty message stops the merge".
func TestMergeEmptyMessage(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	main, own := h.head(repo), h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.gitEditor(`: > "$1"`)
	h.run("merge").mustCode(t, 1)
	if h.head(repo) != main || h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Errorf("a branch moved, or status %q", h.status(feat))
	}
	h.mustBeIntact(repo, feat, "feat")
}

// Scenario "Merged as one commit".
func TestMergeSquash(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	threeCommits(h, feat)
	main := h.head(repo)
	r := h.run("merge", "--squash", "-m", "add login", "--json")
	r.mustCode(t, 0)
	if got := h.sb.Git(repo, "rev-list", "--count", main+"..main"); got != "1" || h.message(repo, "main") != "add login" {
		t.Errorf("main has %s new commits, message %q", got, h.message(repo, "main"))
	}
	h.mustBeRemoved(repo, feat)
	if h.branchExists(repo, "feat") {
		t.Error("the branch feat still exists")
	}
	doc := decodeOne(t, r.stdout)
	if doc["committed"] != true || doc["squashed"] != true {
		t.Errorf("document = %v", doc)
	}
}

// --squash with one commit: nothing to squash, and the merge goes on.
func TestMergeNothingToSquash(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	r := h.run("merge", "--squash", "--no-remove")
	r.mustCode(t, 0)
	mustStdout(t, r, "nothing to squash", "merged feat into main")
	if h.message(repo, "main") != "edit feat.txt" {
		t.Error("the commit of feat is not in main")
	}
}

// Scenario "Order of the events of a merge that commits".
func TestMergeCommitHookOrder(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	events := h.sb.Path("events.txt")
	var lines []string
	for _, e := range []string{"pre-commit", "post-commit", "pre-merge", "post-merge", "pre-remove", "post-remove"} {
		lines = append(lines, hookLine(e, "echo "+e+">> "+events))
	}
	h.hooks("", lines...)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.run("merge", "-m", "x").mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
	got := strings.Fields(fileText(t, events))
	if len(got) != 6 || !slices.Equal(got[:3], []string{"pre-commit", "pre-merge", "pre-remove"}) {
		t.Fatalf("events = %q", got)
	}
	rest := slices.Clone(got[3:])
	slices.Sort(rest)
	if !slices.Equal(rest, []string{"post-commit", "post-merge", "post-remove"}) {
		t.Errorf("events = %q", got)
	}
}

// Scenario "pre-commit fails in a merge" of hooks.
func TestMergePreCommitFails(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	main, own := h.head(repo), h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.hooks("", hookLine("pre-commit", "exit 1"))
	h.run("merge", "-m", "x").mustCode(t, 7)
	if h.head(repo) != main || h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Errorf("a branch moved, or status %q", h.status(feat))
	}
}

// Scenario "Preview of a commit".
func TestMergeDryRunCommit(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("merge", "--dry-run")
	r.mustCode(t, 0)
	lines := stdoutLines(r)
	if len(lines) < 2 || lines[0] != "would commit pending changes to feat" || lines[1] != "would merge feat into main" {
		t.Errorf("stdout = %q", lines)
	}
	if h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Error("the dry run changed something")
	}
}

// With --no-remove, post-commit runs right after the commit, in the
// worktree.
func TestMergeNoRemovePostCommitInTheWorktree(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.hooks("", hookLine("post-commit", "echo ran> post.txt"))
	h.run("merge", "-m", "x", "--no-remove").mustCode(t, 0)
	if !exists(filepath.Join(feat, "post.txt")) {
		t.Error("post-commit did not run in the worktree")
	}
}

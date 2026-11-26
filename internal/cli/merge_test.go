package cli_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
)

// Scenarios of the merge-worktree capability, and the merge scenarios of
// hooks and project-hooks-approval.

// mergeRepo creates <sandbox>/origin.git, its clone <sandbox>/repo on main,
// and the linked worktree <sandbox>/repo.worktrees/feat on the branch feat
// with a commit of its own adding feat.txt. It starts inside feat.
func mergeRepo(h *harness) (repo, feat string) {
	h.t.Helper()
	repo, _, wts := syncRepo(h, "feat")
	feat = wts["feat"]
	h.cwd = feat
	return repo, feat
}

// inDir runs wt with args from dir, and goes back.
func (h *harness) inDir(dir string, args ...string) result {
	h.t.Helper()
	saved := h.cwd
	defer func() { h.cwd = saved }()
	h.cwd = dir
	return h.run(args...)
}

func (h *harness) branchExists(repo, branch string) bool {
	h.t.Helper()
	return h.branchHead(repo, branch) != ""
}

func TestMergeTwoTargets(t *testing.T) {
	h := newHarness(t)
	h.run("merge", "main", "develop").mustCode(t, 2)
}

// Scenario "Main worktree".
func TestMergeMainWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	main := h.head(repo)
	r := h.inDir(repo, "merge")
	r.mustCode(t, 2)
	if got := firstLine(r.stderr); got != "wt: cannot merge the main worktree" {
		t.Errorf("stderr's first line = %q", got)
	}
	if h.head(repo) != main {
		t.Error("main moved")
	}
}

func TestMergeOutsideARepository(t *testing.T) {
	h := newHarness(t)
	h.run("merge").mustCode(t, 3)
}

func TestMergeDetachedHEAD(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	scratch := h.sb.Path("repo.worktrees", "scratch")
	h.sb.Git(repo, "worktree", "add", "-q", "--detach", scratch)
	r := h.inDir(scratch, "merge", "--no-remove")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, "HEAD is detached") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

// Scenario "Working directory override".
func TestMergeWorkingDirectoryOverride(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commit := h.head(feat)
	h.inDir(repo, "-C", feat, "merge").mustCode(t, 0)
	if h.head(repo) != commit {
		t.Errorf("main = %s, want feat's %s", h.head(repo), commit)
	}
	h.mustBeRemoved(repo, feat)
}

// Scenario "Default target".
func TestMergeDefaultTarget(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	r := h.run("merge", "--no-remove")
	r.mustCode(t, 0)
	mustStdout(t, r, "merged feat into main")
	if h.head(repo) != h.head(feat) {
		t.Error("main is not at feat's commit")
	}
}

// Scenario "Target not found".
func TestMergeTargetNotFound(t *testing.T) {
	h := newHarness(t)
	mergeRepo(h)
	r := h.run("merge", "nope")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, "nope") {
		t.Errorf("stderr does not name nope: %q", r.stderr)
	}
}

func TestMergeIntoItsOwnBranch(t *testing.T) {
	h := newHarness(t)
	mergeRepo(h)
	h.run("merge", "feat").mustCode(t, 2)
}

// Scenario "Uncommitted change", and an untracked file.
func TestMergeWithChanges(t *testing.T) {
	for _, tc := range []struct{ name, file string }{
		{"modified", "feat.txt"},
		{"untracked", "new.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			repo, feat := mergeRepo(h)
			main, own := h.head(repo), h.head(feat)
			h.sb.WriteFile(filepath.Join(feat, tc.file), "changed\n")
			r := h.run("merge")
			r.mustCode(t, 5)
			if h.head(repo) != main || h.head(feat) != own {
				t.Error("a branch moved")
			}
			h.mustBeIntact(repo, feat, "feat")
		})
	}
}

func TestMergeRebaseInProgress(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "feat.txt", "main\n")
	cmd := exec.Command("git", "rebase", "main")
	cmd.Dir, cmd.Env = feat, h.sb.Environ()
	if cmd.Run() == nil {
		t.Fatal("the rebase did not stop on a conflict")
	}
	r := h.run("merge")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, "a rebase is in progress") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestMergeLocked(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	h.sb.Git(repo, "worktree", "lock", feat)
	h.run("merge").mustCode(t, 5)
	h.run("merge", "--no-remove").mustCode(t, 0)
	h.mustBeIntact(repo, feat, "feat")
}

// Scenario "Target checked out with changes".
func TestMergeTargetCheckedOutWithChanges(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(repo, "main.txt"), "changed\n")
	r := h.run("merge")
	r.mustCode(t, 5)
	if h.head(feat) != own {
		t.Error("feat was rebased")
	}
}

// Scenario "Hook not approved".
func TestMergeHookNotApproved(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	h.repoHooks(feat, hookLine("pre-merge", "npm test"))
	h.sb.Git(feat, "add", ".wt.toml")
	h.sb.Git(feat, "commit", "-q", "-m", "hooks")
	own := h.head(feat)
	mustNotApproved(t, h.run("merge"), "pre-merge", "npm test")
	if h.head(feat) != own {
		t.Error("feat was rebased")
	}
	// --dry-run checks approvals as without it.
	r := h.run("merge", "--dry-run")
	r.mustCode(t, 8)
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
}

// Scenario "Merge checks the remove events" of project-hooks-approval.
func TestMergeChecksTheRemoveEvents(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	main := h.head(repo)
	h.repoHooks(feat, hookLine("pre-remove", "make clean"))
	h.sb.Git(feat, "add", ".wt.toml")
	h.sb.Git(feat, "commit", "-q", "-m", "hooks")
	own := h.head(feat)
	mustNotApproved(t, h.run("merge"), "pre-remove", "make clean")
	if h.head(repo) != main || h.head(feat) != own {
		t.Error("a branch moved")
	}
	// Without the removal, its events are not checked.
	h.run("merge", "--no-remove").mustCode(t, 0)
}

// Scenario "Behind the target".
func TestMergeBehindTheTarget(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	upstream := commitIn(h, repo, "main.txt", "main\n")
	r := h.run("merge", "--no-remove")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto main", "merged feat into main")
	if !h.contains(feat, upstream) || h.head(repo) != h.head(feat) {
		t.Error("main does not contain feat's rebased commits")
	}
	if !exists(filepath.Join(repo, "feat.txt")) {
		t.Error("the main worktree has no feat.txt")
	}
}

// Scenario "Conflict".
func TestMergeConflict(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "feat.txt", "main\n")
	main, own := h.head(repo), h.head(feat)
	r := h.run("merge")
	r.mustCode(t, 6)
	if got := firstLine(r.stderr); got != "wt: conflict rebasing feat onto main; rebase aborted" {
		t.Errorf("stderr's first line = %q", got)
	}
	if !strings.Contains(r.stderr, `git rebase main`) {
		t.Errorf("no hint naming git rebase main: %q", r.stderr)
	}
	if h.head(feat) != own || h.rebasing(feat) {
		t.Error("feat was left rebased or rebasing")
	}
	if h.head(repo) != main {
		t.Error("main moved")
	}
}

// Scenario "Behind without rebasing".
func TestMergeBehindWithoutRebasing(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	r := h.run("merge", "--no-rebase")
	r.mustCode(t, 5)
	if got := firstLine(r.stderr); got != "wt: feat does not contain main" {
		t.Errorf("stderr's first line = %q", got)
	}
	if !strings.Contains(r.stderr, "--no-rebase") {
		t.Errorf("no hint naming --no-rebase: %q", r.stderr)
	}
}

// A target not checked out anywhere is moved without touching any worktree.
func TestMergeTargetNotCheckedOut(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	h.sb.Git(repo, "branch", "develop", "main")
	main := h.head(repo)
	h.run("merge", "develop").mustCode(t, 0)
	if got := h.branchHead(repo, "develop"); got == main || got == "" {
		t.Errorf("develop = %q, want feat's commit", got)
	}
	if h.head(repo) != main {
		t.Error("main moved")
	}
	h.mustBeRemoved(repo, feat)
}

// Scenario "Nothing to merge".
func TestMergeNothingToMerge(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.run("create", "feat").mustCode(t, 0)
	feat := h.sb.Path("repo.worktrees", "feat")
	r := h.inDir(feat, "merge")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "feat is already in main") {
		t.Errorf("stdout = %q", r.stdout)
	}
	h.mustBeRemoved(repo, feat)
}

// Scenario "Merged and removed".
func TestMergeMergedAndRemoved(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	h.userConfig("default_base = \"origin/main\"\n")
	directive := h.activate()
	r := h.run("merge", "main")
	r.mustCode(t, 0)
	mustStdout(t, r, "merged feat into main", "removed worktree "+feat, "deleted branch feat")
	mustDirective(t, directive, repo)
	h.mustBeRemoved(repo, feat)
	if h.branchExists(repo, "feat") {
		t.Error("the branch feat still exists")
	}
}

// Scenario "Keeping the worktree".
func TestMergeKeepingTheWorktree(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	directive := h.activate()
	h.run("merge", "--no-remove").mustCode(t, 0)
	h.mustBeIntact(repo, feat, "feat")
	if got := readDirective(t, directive); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
}

// Scenario "pre-remove fails after the merge".
func TestMergePreRemoveFailsAfterTheMerge(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	h.hooks("", hookLine("pre-remove", "exit 1"))
	r := h.run("merge")
	r.mustCode(t, 7)
	if h.head(repo) != h.head(feat) {
		t.Error("main does not contain feat's commit")
	}
	h.mustBeIntact(repo, feat, "feat")
	if !strings.Contains(r.stderr, "feat was merged into main") {
		t.Errorf("no hint saying feat was merged: %q", r.stderr)
	}
}

// Scenario "pre-merge fails" of hooks.
func TestMergePreMergeFails(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	main := h.head(repo)
	h.hooks("", hookLine("pre-merge", "exit 1"))
	r := h.run("merge")
	r.mustCode(t, 7)
	if !strings.Contains(r.stderr, "pre-merge hook failed") || !strings.Contains(r.stderr, "--no-hooks") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.head(repo) != main {
		t.Error("main moved")
	}
	h.mustBeIntact(repo, feat, "feat")
}

// Scenario "Values of a merge event" of hooks.
func TestMergeHookValues(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.hooks("", hookLine("pre-merge", "echo {branch} {target}> vars.txt"))
	h.run("merge", "main", "--no-remove").mustCode(t, 0)
	if got := fileText(t, filepath.Join(feat, "vars.txt")); got != "feat main" {
		t.Errorf("vars.txt = %q", got)
	}
}

// Scenario "Target outside a merge event" of hooks.
func TestMergeTargetOutsideAMergeEvent(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("pre-remove", "echo {target}"))
	r := h.run("list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "hooks.pre-remove") || !strings.Contains(r.stderr, "target") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

// Scenario "Order of the merge events" of hooks.
func TestMergeHookOrder(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	events := h.sb.Path("events.txt")
	var lines []string
	for _, e := range []string{"pre-merge", "post-merge", "pre-remove", "post-remove"} {
		lines = append(lines, hookLine(e, "echo "+e+">> "+events))
	}
	h.hooks("", lines...)
	h.run("merge").mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
	got := strings.Fields(fileText(t, events))
	want := []string{"pre-merge", "pre-remove", "post-remove", "post-merge"}
	if !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

// Scenario "Preview".
func TestMergeDryRun(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	main, own := h.head(repo), h.head(feat)
	directive := h.activate()
	r := h.run("merge", "--dry-run")
	r.mustCode(t, 0)
	lines := stdoutLines(r)
	if len(lines) < 3 || lines[0] != "would rebase feat onto main" || lines[1] != "would merge feat into main" || lines[2] != "would remove worktree "+feat {
		t.Errorf("stdout = %q", lines)
	}
	if h.head(repo) != main || h.head(feat) != own {
		t.Error("a branch moved")
	}
	h.mustBeIntact(repo, feat, "feat")
	if got := readDirective(t, directive); got != "" {
		t.Errorf("directive %q, want empty", got)
	}
}

// Scenario "Merged as JSON".
func TestMergeJSON(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	r := h.run("merge", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.merge.v1" || doc["branch"] != "feat" || doc["target"] != "main" || doc["removed"] != true || doc["rebased"] != false {
		t.Errorf("document = %v", doc)
	}
	if doc["head"] != h.head(repo) || doc["name"] != "feat" || doc["path"] != feat {
		t.Errorf("document = %v, want head %s", doc, h.head(repo))
	}
}

// Scenario "Targets".
func TestMergeCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	h.sb.Git(repo, "branch", "develop")
	got, _, _ := h.complete("merge", "")
	if !slices.Contains(got, "main") || !slices.Contains(got, "develop") || slices.Contains(got, "feat") {
		t.Errorf("completions = %q", got)
	}
	h.cwd = h.sb.Path("outside")
	if got, _, r := h.complete("merge", ""); len(got) != 0 || r.stderr != "" {
		t.Errorf("outside a repository: completions %q, stderr %q", got, r.stderr)
	}
}

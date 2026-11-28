package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
)

// Scenarios of the merge-steps capability, and the commit scenarios of
// hooks and project-hooks-approval.

// gitEditor makes git's editor the shell function body f, which gets the
// file to edit as $1. git runs its editor through its own sh on every OS.
func (h *harness) gitEditor(f string) {
	h.sb.Setenv("GIT_EDITOR", "f() { "+f+"; }; f")
	h.stdinTTY = true
}

// threeCommits gives feat, from mergeRepo, two more commits of its own:
// three that main does not contain.
func threeCommits(h *harness, feat string) {
	h.t.Helper()
	commitIn(h, feat, "f2.txt", "2\n")
	commitIn(h, feat, "f3.txt", "3\n")
}

func (h *harness) message(dir, rev string) string {
	h.t.Helper()
	return h.sb.Git(dir, "log", "-1", "--format=%B", rev)
}

func (h *harness) ownCommits(dir, target string) []string {
	h.t.Helper()
	out := h.sb.Git(dir, "rev-list", target+"..HEAD")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (h *harness) status(dir string) string {
	h.t.Helper()
	return h.sb.Git(dir, "status", "--porcelain")
}

// --- 4.1: the command ---

func TestStepNoSubcommand(t *testing.T) {
	h := newHarness(t)
	r := h.run("step")
	r.mustCode(t, 2)
	if r.stdout != "" || !strings.Contains(r.stderr, "commit") {
		t.Errorf("stdout = %q, stderr = %q; want the help on stderr", r.stdout, r.stderr)
	}
}

func TestStepUnknownSubcommand(t *testing.T) {
	h := newHarness(t)
	r := h.run("step", "fixup")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, "fixup") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestStepUsageErrors(t *testing.T) {
	h := newHarness(t)
	h.run("step", "commit", "main").mustCode(t, 2)
	h.run("step", "squash", "main", "dev").mustCode(t, 2)
	r := h.run("step", "rebase", "--no-hooks")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, "--no-hooks") {
		t.Errorf("stderr = %q", r.stderr)
	}
	h.run("step", "rebase", "-m", "x").mustCode(t, 2)
}

// --- 4.2: the plan and its checks ---

func TestStepMainWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(repo, "README.md"), "changed\n")
	r := h.inDir(repo, "step", "commit", "-m", "x")
	r.mustCode(t, 2)
	if got := firstLine(r.stderr); got != "wt: cannot run wt step in the main worktree" {
		t.Errorf("stderr's first line = %q", got)
	}
}

func TestStepOutsideARepository(t *testing.T) {
	h := newHarness(t)
	h.run("step", "commit").mustCode(t, 3)
}

func TestStepDetachedHEAD(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	scratch := h.sb.Path("repo.worktrees", "scratch")
	h.sb.AddDetachedWorktree(repo, scratch)
	h.sb.WriteFile(filepath.Join(scratch, "x.txt"), "x\n")
	h.cwd = scratch
	h.run("step", "commit", "-m", "x").mustCode(t, 5)
}

func TestStepRebaseInProgress(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "feat.txt", "main\n")
	cmd := exec.Command("git", "rebase", "main")
	cmd.Dir, cmd.Env = feat, h.sb.Environ()
	if cmd.Run() == nil {
		t.Fatal("the rebase did not stop on a conflict")
	}
	r := h.run("step", "squash")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, "a rebase is in progress") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestStepDefaultTarget(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	upstream := commitIn(h, repo, "main.txt", "main\n")
	r := h.run("step", "rebase")
	r.mustCode(t, 0)
	if !h.contains(feat, upstream) {
		t.Error("feat was not rebased onto main")
	}
}

func TestStepTargetNotFound(t *testing.T) {
	h := newHarness(t)
	mergeRepo(h)
	r := h.run("step", "squash", "nope")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, "nope") {
		t.Errorf("stderr = %q", r.stderr)
	}
	h.run("step", "push", "feat").mustCode(t, 2)
}

func TestStepRebaseModifiedFile(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("step", "rebase")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, "wt step commit") {
		t.Errorf("stderr = %q, want a hint naming wt step commit", r.stderr)
	}
	if h.head(feat) != own {
		t.Error("feat moved")
	}
}

func TestStepRebaseUntrackedFile(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "main.txt", "main\n")
	h.sb.WriteFile(filepath.Join(feat, "notes.txt"), "notes\n")
	r := h.run("step", "rebase")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto main")
	if !exists(filepath.Join(feat, "notes.txt")) {
		t.Error("notes.txt is gone")
	}
}

// Scenario "Behind the target" of pushing.
func TestStepPushBehind(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	main := commitIn(h, repo, "main.txt", "main\n")
	r := h.run("step", "push")
	r.mustCode(t, 5)
	if got := firstLine(r.stderr); got != "wt: feat does not contain main" {
		t.Errorf("stderr's first line = %q", got)
	}
	if !strings.Contains(r.stderr, "hint: ") || !strings.Contains(r.stderr, "wt step rebase") {
		t.Errorf("stderr = %q, want a hint naming wt step rebase", r.stderr)
	}
	if h.head(repo) != main {
		t.Error("main moved")
	}
}

// Scenario "Script without a message".
func TestStepCommitWithoutATerminal(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("step", "commit")
	r.mustCode(t, 2)
	if got := firstLine(r.stderr); got != "wt: a commit message is needed" {
		t.Errorf("stderr's first line = %q", got)
	}
	if !strings.Contains(r.stderr, "-m") || h.head(feat) != own {
		t.Errorf("stderr = %q, or feat moved", r.stderr)
	}
	// --json needs -m even in a terminal.
	h.stdinTTY = true
	h.run("step", "commit", "--json").mustCode(t, 2)
}

// --- 4.3: commit and squash ---

// Scenario "Untracked and ignored files".
func TestStepCommitUntrackedAndIgnored(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	commitIn(h, feat, ".gitignore", "*.log\n")
	h.sb.WriteFile(filepath.Join(feat, "new.txt"), "new\n")
	h.sb.WriteFile(filepath.Join(feat, "debug.log"), "log\n")
	h.run("step", "commit", "-m", "add new").mustCode(t, 0)
	files := h.sb.Git(feat, "ls-tree", "-r", "--name-only", "HEAD")
	if !strings.Contains(files, "new.txt") || strings.Contains(files, "debug.log") {
		t.Errorf("files = %q", files)
	}
	if !exists(filepath.Join(feat, "debug.log")) {
		t.Error("debug.log is gone")
	}
}

// Scenario "Commit with a message".
func TestStepCommitWithAMessage(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("step", "commit", "-m", "fix login")
	r.mustCode(t, 0)
	short := h.sb.Git(feat, "rev-parse", "--short", "HEAD")
	mustStdout(t, r, "committed "+short+" to feat")
	if h.message(feat, "HEAD") != "fix login" || h.sb.Git(feat, "rev-parse", "HEAD^") != own {
		t.Error("wrong commit")
	}
	if s := h.status(feat); s != "" {
		t.Errorf("status = %q", s)
	}
}

// Scenarios "Nothing to commit" and "No message needed".
func TestStepNothingToCommit(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	mergeRepo(h)
	ran := h.sb.Path("ran.txt")
	h.hooks("", hookLine("pre-commit", "echo x> "+ran))
	r := h.run("step", "commit")
	r.mustCode(t, 0)
	mustStdout(t, r, "nothing to commit")
	if exists(ran) {
		t.Error("pre-commit ran")
	}
	h.run("step", "commit", "--json").mustCode(t, 0)
}

// Scenario "Three commits".
func TestStepSquashThreeCommits(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	threeCommits(h, feat)
	old := h.head(feat)
	r := h.run("step", "squash", "-m", "add login")
	r.mustCode(t, 0)
	short := h.sb.Git(feat, "rev-parse", "--short", "HEAD")
	mustStdout(t, r, "squashed 3 commits of feat into "+short)
	if own := h.ownCommits(feat, "main"); len(own) != 1 || h.message(feat, "HEAD") != "add login" {
		t.Errorf("own commits = %v, message %q", own, h.message(feat, "HEAD"))
	}
	if h.sb.Git(feat, "rev-parse", "HEAD^{tree}") != h.sb.Git(feat, "rev-parse", old+"^{tree}") {
		t.Error("the files changed")
	}
}

// Scenario "Commits and pending changes".
func TestStepSquashWithPendingChanges(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	commitIn(h, feat, "f2.txt", "2\n")
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.run("step", "squash", "-m", "x").mustCode(t, 0)
	if own := h.ownCommits(feat, "main"); len(own) != 1 {
		t.Errorf("own commits = %v", own)
	}
	if got := h.sb.Git(feat, "show", "HEAD:feat.txt"); got != "changed" {
		t.Errorf("feat.txt = %q", got)
	}
	if s := h.status(feat); s != "" {
		t.Errorf("status = %q", s)
	}
}

// Scenario "One commit", and the squash of pending changes only.
func TestStepNothingToSquash(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	own := h.head(feat)
	r := h.run("step", "squash")
	r.mustCode(t, 0)
	mustStdout(t, r, "nothing to squash")
	if h.head(feat) != own {
		t.Error("feat moved")
	}

	h.sb.Git(repo, "merge", "-q", "--ff-only", "feat")
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r = h.run("step", "squash", "-m", "only pending")
	r.mustCode(t, 0)
	short := h.sb.Git(feat, "rev-parse", "--short", "HEAD")
	mustStdout(t, r, "committed "+short+" to feat")
}

// Scenario "Message from the editor".
func TestStepCommitMessageFromTheEditor(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.gitEditor(`echo "from editor" > "$1"`)
	h.run("step", "commit").mustCode(t, 0)
	if got := h.message(feat, "HEAD"); got != "from editor" {
		t.Errorf("message = %q", got)
	}
}

// Scenario "Messages of the branch".
func TestStepSquashMessagesOfTheBranch(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	commitIn(h, feat, "form.txt", "form\n")
	copied := filepath.ToSlash(h.sb.Path("msg.txt"))
	h.gitEditor(`cp "$1" "` + copied + `"`)
	h.run("step", "squash").mustCode(t, 0)
	got := fileText(t, h.sb.Path("msg.txt"))
	first, second := strings.Index(got, "edit feat.txt"), strings.Index(got, "edit form.txt")
	if !strings.HasPrefix(got, "# Squashing 2 commits of feat") || first < 0 || second < first {
		t.Errorf("the editor opened with %q", got)
	}
}

// Scenario "Editor saves nothing".
func TestStepCommitEmptyMessage(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.gitEditor(`: > "$1"`)
	r := h.run("step", "commit")
	r.mustCode(t, 1)
	if got := firstLine(lastNonGit(r.stderr)); got != "wt: empty commit message; nothing committed" {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Errorf("feat moved, or status = %q", h.status(feat))
	}
	h.run("step", "commit", "-m", " ").mustCode(t, 1)
}

// lastNonGit is stderr from wt's own first line: git's editor messages
// come before it.
func lastNonGit(stderr string) string {
	if i := strings.Index(stderr, "wt: "); i >= 0 {
		return stderr[i:]
	}
	return stderr
}

// rejectCommits installs a commit-msg hook in repo that rejects every
// commit.
func (h *harness) rejectCommits(repo string) {
	h.t.Helper()
	p := filepath.Join(repo, ".git", "hooks", "commit-msg")
	h.sb.WriteFile(p, "#!/bin/sh\necho rejected >&2\nexit 1\n")
	if err := os.Chmod(p, 0o755); err != nil {
		h.t.Fatal(err)
	}
}

// Scenario "Native hook rejects the commit".
func TestStepCommitRejected(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, feat, "b.txt", "b\n")
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "staged\n")
	h.sb.Git(feat, "add", "feat.txt")
	h.sb.WriteFile(filepath.Join(feat, "b.txt"), "unstaged\n")
	before := h.status(feat)
	h.rejectCommits(repo)
	r := h.run("step", "commit", "-m", "x")
	r.mustCode(t, 1)
	if h.head(feat) != own || h.status(feat) != before {
		t.Errorf("feat moved, or status %q, want %q", h.status(feat), before)
	}
}

// Scenario "Squash rejected".
func TestStepSquashRejected(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	threeCommits(h, feat)
	own := h.head(feat)
	h.rejectCommits(repo)
	h.run("step", "squash", "-m", "x").mustCode(t, 1)
	if h.head(feat) != own || h.status(feat) != "" {
		t.Errorf("feat moved, or status %q", h.status(feat))
	}
}

// Scenario "pre-commit fails".
func TestStepPreCommitFails(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	own := h.head(feat)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.hooks("", hookLine("pre-commit", "exit 1"))
	h.run("step", "commit", "-m", "x").mustCode(t, 7)
	if h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Errorf("feat moved, or status %q", h.status(feat))
	}
}

// Scenario "Order of the commit events" of hooks.
func TestStepCommitHookOrder(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.hooks("", hookLine("pre-commit", "git log -1 --format=%s> pre.txt"), hookLine("post-commit", "git log -1 --format=%s> post.txt"))
	h.run("step", "commit", "-m", "new").mustCode(t, 0)
	if got := fileText(t, filepath.Join(feat, "pre.txt")); got != "edit feat.txt" {
		t.Errorf("pre.txt = %q", got)
	}
	if got := fileText(t, filepath.Join(feat, "post.txt")); got != "new" {
		t.Errorf("post.txt = %q", got)
	}
}

// Scenario "Commit without hooks" of hooks.
func TestStepCommitWithoutHooks(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	h.hooks("", hookLine("pre-commit", "exit 1"))
	h.run("step", "commit", "-m", "x", "--no-hooks").mustCode(t, 0)
	if h.message(feat, "HEAD") != "x" {
		t.Error("no commit")
	}
}

// --- 4.4: rebase and push ---

// Scenarios "Rebased" and "Conflict" of the steps.
func TestStepRebase(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	upstream := commitIn(h, repo, "main.txt", "main\n")
	r := h.run("step", "rebase")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto main")
	if !h.contains(feat, upstream) || h.head(repo) != upstream {
		t.Error("feat was not rebased, or main moved")
	}
	r = h.run("step", "rebase")
	r.mustCode(t, 0)
	mustStdout(t, r, "feat already contains main")
}

func TestStepRebaseConflict(t *testing.T) {
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	commitIn(h, repo, "feat.txt", "main\n")
	own := h.head(feat)
	r := h.run("step", "rebase")
	r.mustCode(t, 6)
	if got := firstLine(r.stderr); got != "wt: conflict rebasing feat onto main; rebase aborted" {
		t.Errorf("stderr's first line = %q", got)
	}
	if h.head(feat) != own || h.rebasing(feat) {
		t.Error("feat was left rebased or rebasing")
	}
}

// Scenario "Target moved, worktree kept", with its hooks.
func TestStepPush(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := mergeRepo(h)
	events := h.sb.Path("events.txt")
	h.hooks("", hookLine("pre-merge", "echo pre-merge>> "+events), hookLine("post-merge", "echo post-merge>> "+events))
	r := h.run("step", "push")
	r.mustCode(t, 0)
	mustStdout(t, r, "merged feat into main")
	if h.head(repo) != h.head(feat) || !h.branchExists(repo, "feat") || !exists(feat) {
		t.Error("main was not moved, or feat is gone")
	}
	if got := strings.Fields(fileText(t, events)); strings.Join(got, " ") != "pre-merge post-merge" {
		t.Errorf("events = %q", got)
	}
	r = h.run("step", "push")
	r.mustCode(t, 0)
	mustStdout(t, r, "feat is already in main")
}

// --- 4.5: dry run and JSON ---

// Scenarios "Preview of a squash" and "Dry run needs no message".
func TestStepDryRun(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	threeCommits(h, feat)
	own := h.head(feat)
	r := h.run("step", "squash", "--dry-run")
	r.mustCode(t, 0)
	mustStdout(t, r, "would squash 3 commits of feat")
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r = h.run("step", "commit", "--dry-run")
	r.mustCode(t, 0)
	mustStdout(t, r, "would commit pending changes to feat")
	if h.head(feat) != own || h.status(feat) != "M feat.txt" {
		t.Error("the dry run changed something")
	}
}

// Scenario "Commit as JSON".
func TestStepJSON(t *testing.T) {
	h := newHarness(t)
	_, feat := mergeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "feat.txt"), "changed\n")
	r := h.run("step", "commit", "-m", "x", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.step.v1" || doc["step"] != "commit" || doc["branch"] != "feat" || doc["target"] != "" ||
		doc["changed"] != true || doc["head"] != h.head(feat) {
		t.Errorf("document = %v", doc)
	}
}

// --- 4.6: completions ---

func TestStepCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _ := mergeRepo(h)
	h.sb.Git(repo, "branch", "develop")
	got, _, _ := h.complete("step", "")
	for _, want := range []string{"commit", "squash", "rebase", "push"} {
		if !slices.Contains(got, want) {
			t.Errorf("completions = %q, want %s", got, want)
		}
	}
	got, _, _ = h.complete("step", "squash", "")
	if !slices.Contains(got, "main") || !slices.Contains(got, "develop") || slices.Contains(got, "feat") {
		t.Errorf("completions = %q", got)
	}
	h.cwd = h.sb.Path("outside")
	if got, _, r := h.complete("step", "rebase", ""); len(got) != 0 || r.stderr != "" {
		t.Errorf("outside a repository: completions %q, stderr %q", got, r.stderr)
	}
}

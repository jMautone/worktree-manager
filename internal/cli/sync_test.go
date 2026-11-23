package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Scenarios of the sync-worktrees capability.

// syncRepo creates <sandbox>/origin.git, its clone <sandbox>/repo, and for
// each name a linked worktree <sandbox>/repo.worktrees/<name> on the branch
// <name>, with a commit of its own adding <name>.txt. It starts in the main
// worktree.
func syncRepo(h *harness, names ...string) (repo, origin string, wts map[string]string) {
	h.t.Helper()
	repo, origin = cloneRepo(h)
	wts = map[string]string{}
	for _, n := range names {
		p := h.sb.Path("repo.worktrees", n)
		h.sb.AddWorktree(repo, p, n)
		commitIn(h, p, n+".txt", n+"\n")
		wts[n] = p
	}
	return repo, origin, wts
}

// commitIn writes content to name in dir, commits it, and returns the
// commit.
func commitIn(h *harness, dir, name, content string) string {
	h.t.Helper()
	h.sb.WriteFile(filepath.Join(dir, name), content)
	h.sb.Git(dir, "add", name)
	h.sb.Git(dir, "commit", "-q", "-m", "edit "+name)
	return h.sb.Git(dir, "rev-parse", "HEAD")
}

// pushFile advances branch on origin as someone else would, with a commit
// that writes content to name. It returns that commit.
func pushFile(h *harness, origin, branch, name, content string) string {
	h.t.Helper()
	other, err := os.MkdirTemp(h.sb.Root, "pusher-")
	if err != nil {
		h.t.Fatal(err)
	}
	h.sb.Clone(origin, other)
	id := commitIn(h, other, name, content)
	h.sb.Push(other, "origin", "HEAD:refs/heads/"+branch)
	return id
}

func (h *harness) head(dir string) string {
	h.t.Helper()
	return h.sb.Git(dir, "rev-parse", "HEAD")
}

// contains reports whether commit is in the history of dir's HEAD.
func (h *harness) contains(dir, commit string) bool {
	h.t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", commit, "HEAD")
	cmd.Dir = dir
	cmd.Env = h.sb.Environ()
	return cmd.Run() == nil
}

// rebasing reports whether dir has a rebase in progress.
func (h *harness) rebasing(dir string) bool {
	h.t.Helper()
	for _, p := range []string{"rebase-merge", "rebase-apply"} {
		path := h.sb.Git(dir, "rev-parse", "--git-path", p)
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// fetches runs wt with args under GIT_TRACE and counts the git fetch
// commands it ran.
func (h *harness) fetches(args ...string) (result, int) {
	h.t.Helper()
	trace := h.sb.Path("trace.log")
	_ = os.Remove(trace)
	h.sb.Setenv("GIT_TRACE", trace)
	r := h.run(args...)
	h.sb.Setenv("GIT_TRACE", "")
	b, _ := os.ReadFile(trace)
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "built-in: git fetch") {
			n++
		}
	}
	return r, n
}

func stdoutLines(r result) []string {
	s := strings.TrimSuffix(r.stdout, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func mustStdout(t *testing.T, r result, want ...string) {
	t.Helper()
	if got := stdoutLines(r); !slices.Equal(got, want) {
		t.Errorf("stdout = %q, want %q\nstderr:\n%s", got, want, r.stderr)
	}
}

func TestSyncUsageErrors(t *testing.T) {
	h := newHarness(t)
	_, _, _ = syncRepo(h, "feat")

	r := h.run("sync", "feat", "--all")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, "--all") {
		t.Errorf("stderr does not name --all:\n%s", r.stderr)
	}
	r = h.run("sync", "--no-hooks")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, "--no-hooks") {
		t.Errorf("stderr does not name --no-hooks:\n%s", r.stderr)
	}
	h.run("sync", "--base", "").mustCode(t, 2)
}

func TestSyncCurrentWorktree(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat", "fix")
	fix := h.head(wts["fix"])
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")

	h.cwd = wts["feat"]
	r := h.run("sync")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto origin/main")
	if !h.contains(wts["feat"], pushed) {
		t.Error("feat does not contain the commit pushed to origin/main")
	}
	if h.head(wts["fix"]) != fix {
		t.Error("fix was rebased too")
	}
}

func TestSyncMainWorktreeByName(t *testing.T) {
	h := newHarness(t)
	repo, origin, _ := syncRepo(h)
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")

	r := h.run("sync", "^")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased ^ onto origin/main")
	if h.head(repo) != pushed {
		t.Errorf("main is at %s, want %s", h.head(repo), pushed)
	}
}

func TestSyncNameNotFound(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat")
	feat := h.head(wts["feat"])
	pushFile(h, origin, "main", "main.txt", "main\n")

	h.run("sync", "feat", "nope").mustCode(t, 3)
	if h.head(wts["feat"]) != feat {
		t.Error("feat was rebased")
	}
}

func TestSyncAllLinkedWorktreesOnly(t *testing.T) {
	h := newHarness(t)
	repo, origin, wts := syncRepo(h, "feat", "fix")
	h.sb.AddDetachedWorktree(repo, h.sb.Path("repo.worktrees", "scratch"))
	main := h.head(repo)
	pushFile(h, origin, "main", "main.txt", "main\n")

	r := h.run("sync", "--all")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto origin/main", "rebased fix onto origin/main")
	if h.head(repo) != main {
		t.Error("the main worktree was rebased")
	}
	_ = wts
}

func TestSyncAllWithNoLinkedWorktree(t *testing.T) {
	h := newHarness(t)
	syncRepo(h)
	r := h.run("sync", "--all")
	r.mustCode(t, 0)
	mustStdout(t, r)
}

func TestSyncAllOutsideARepository(t *testing.T) {
	h := newHarness(t)
	r := h.run("sync", "--all")
	r.mustCode(t, 3)
	if !strings.HasPrefix(firstLine(r.stderr), "wt: not a git repository: ") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestSyncWorktreeOfAnotherRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api := filepath.Join(root, "api")
	advanced := commitIn(h, api, "main.txt", "main\n")
	h.cwd = filepath.Join(root, "web")

	r := h.run("sync", "api:feat")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased api:feat onto main")
	if !h.contains(feat, advanced) {
		t.Error("feat of api was not rebased onto main")
	}
}

func TestSyncBaseFromTheRepositoryConfiguration(t *testing.T) {
	h := newHarness(t)
	repo, _, wts := syncRepo(h, "feat")
	h.sb.Git(repo, "branch", "develop")
	h.sb.Git(repo, "checkout", "-q", "develop")
	developed := commitIn(h, repo, "develop.txt", "develop\n")
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \"develop\"\n")

	r := h.run("sync", "feat")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto develop")
	if !h.contains(wts["feat"], developed) {
		t.Error("feat was not rebased onto develop")
	}
}

func TestSyncExplicitBase(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat")
	released := pushFile(h, origin, "release", "release.txt", "release\n")

	r := h.run("sync", "feat", "--base", "origin/release")
	r.mustCode(t, 0)
	mustStdout(t, r, "rebased feat onto origin/release")
	if !h.contains(wts["feat"], released) {
		t.Error("feat was not rebased onto origin/release")
	}
}

func TestSyncFetchesOnceForSeveralWorktrees(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "a", "b", "c")
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")

	r, n := h.fetches("sync", "--all")
	r.mustCode(t, 0)
	if n != 1 {
		t.Errorf("git fetch ran %d times, want 1", n)
	}
	for name, p := range wts {
		if !h.contains(p, pushed) {
			t.Errorf("%s does not contain the fetched origin/main", name)
		}
	}
}

func TestSyncRemoteCannotBeReached(t *testing.T) {
	h := newHarness(t)
	repo, origin, wts := syncRepo(h, "feat")
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")
	h.sb.Git(repo, "fetch", "-q", "origin")
	h.sb.BreakRemote(repo, "origin")

	r := h.run("sync", "feat")
	r.mustCode(t, 0)
	if !strings.Contains(r.stderr, "warning") || !strings.Contains(r.stderr, "origin") {
		t.Errorf("stderr has no warning naming origin:\n%s", r.stderr)
	}
	if !h.contains(wts["feat"], pushed) {
		t.Error("feat was not rebased onto the local origin/main")
	}
}

func TestSyncLocalBaseFetchesNothing(t *testing.T) {
	h := newHarness(t)
	syncRepo(h, "feat")
	r, n := h.fetches("sync", "feat", "--base", "main")
	r.mustCode(t, 0)
	if n != 0 {
		t.Errorf("git fetch ran %d times, want 0", n)
	}
	r, n = h.fetches("sync", "feat", "--no-fetch")
	r.mustCode(t, 0)
	if n != 0 {
		t.Errorf("with --no-fetch, git fetch ran %d times, want 0", n)
	}
}

func TestSyncBaseNotFound(t *testing.T) {
	h := newHarness(t)
	_, _, wts := syncRepo(h, "feat", "fix")
	feat, fix := h.head(wts["feat"]), h.head(wts["fix"])

	r := h.run("sync", "feat", "fix", "--base", "nope")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, "nope") || r.stdout != "" {
		t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
	}
	if h.head(wts["feat"]) != feat || h.head(wts["fix"]) != fix {
		t.Error("a worktree was rebased")
	}
}

func TestSyncSkipsAModifiedWorktree(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat", "fix")
	feat := h.head(wts["feat"])
	h.sb.WriteFile(filepath.Join(wts["feat"], "feat.txt"), "changed\n")
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")

	r := h.run("sync", "--all")
	r.mustCode(t, 5)
	mustStdout(t, r, "skipped feat: has uncommitted changes", "rebased fix onto origin/main")
	if h.head(wts["feat"]) != feat {
		t.Error("feat's HEAD moved")
	}
	if b, _ := os.ReadFile(filepath.Join(wts["feat"], "feat.txt")); string(b) != "changed\n" {
		t.Errorf("feat.txt = %q", b)
	}
	if !h.contains(wts["fix"], pushed) {
		t.Error("fix was not rebased")
	}
	if !strings.Contains(r.stderr, "wt: 1 of 2 worktrees not synced") {
		t.Errorf("stderr has no summary:\n%s", r.stderr)
	}
	mustHint(t, r, "feat: has uncommitted changes")
}

func TestSyncUntrackedFileDoesNotStopTheRebase(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat")
	h.sb.WriteFile(filepath.Join(wts["feat"], "notes.txt"), "mine\n")
	pushed := pushFile(h, origin, "main", "main.txt", "main\n")

	h.run("sync", "feat").mustCode(t, 0)
	if !h.contains(wts["feat"], pushed) {
		t.Error("feat was not rebased")
	}
	if b, _ := os.ReadFile(filepath.Join(wts["feat"], "notes.txt")); string(b) != "mine\n" {
		t.Errorf("notes.txt = %q", b)
	}
}

func TestSyncSkipsARebaseInProgress(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat")
	commitIn(h, wts["feat"], "f.txt", "feat\n")
	pushFile(h, origin, "main", "f.txt", "main\n")
	h.sb.Git(wts["feat"], "fetch", "-q", "origin")
	cmd := exec.Command("git", "rebase", "origin/main")
	cmd.Dir, cmd.Env = wts["feat"], h.sb.Environ()
	if cmd.Run() == nil {
		t.Fatal("the rebase did not stop on a conflict")
	}

	r := h.run("sync", "feat")
	r.mustCode(t, 5)
	mustStdout(t, r, "skipped feat: a rebase is in progress")
	if !h.rebasing(wts["feat"]) {
		t.Error("the rebase in feat is no longer in progress")
	}
}

func TestSyncFreshWorktreeIsUpToDate(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.run("create", "feat", "--no-cd").mustCode(t, 0)
	r := h.run("sync", "feat")
	r.mustCode(t, 0)
	mustStdout(t, r, "feat is up to date with origin/main")
}

func TestSyncNeverStashes(t *testing.T) {
	h := newHarness(t)
	repo, origin, wts := syncRepo(h, "feat")
	h.sb.Git(repo, "config", "rebase.autoStash", "true")
	h.sb.WriteFile(filepath.Join(wts["feat"], "feat.txt"), "changed\n")
	pushFile(h, origin, "main", "main.txt", "main\n")

	r := h.run("sync", "feat")
	r.mustCode(t, 5)
	mustStdout(t, r, "skipped feat: has uncommitted changes")
	if got := h.sb.Git(repo, "stash", "list"); got != "" {
		t.Errorf("stash list = %q", got)
	}
}

func TestSyncConflictThenTheNextWorktree(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat", "fix")
	feat := commitIn(h, wts["feat"], "f.txt", "feat\n")
	pushed := pushFile(h, origin, "main", "f.txt", "main\n")

	r := h.run("sync", "--all")
	r.mustCode(t, 6)
	mustStdout(t, r, "conflict in feat: rebase onto origin/main aborted", "rebased fix onto origin/main")
	if h.head(wts["feat"]) != feat {
		t.Error("feat's HEAD moved")
	}
	if h.rebasing(wts["feat"]) {
		t.Error("feat was left with a rebase in progress")
	}
	if !h.contains(wts["fix"], pushed) {
		t.Error("fix was not rebased")
	}
}

func TestSyncSkippedAndConflict(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "docs", "feat", "fix")
	h.sb.WriteFile(filepath.Join(wts["feat"], "feat.txt"), "changed\n")
	commitIn(h, wts["fix"], "f.txt", "fix\n")
	pushed := pushFile(h, origin, "main", "f.txt", "main\n")

	r := h.run("sync", "--all")
	r.mustCode(t, 6)
	if !strings.Contains(r.stderr, "wt: 2 of 3 worktrees not synced") {
		t.Errorf("stderr has no summary:\n%s", r.stderr)
	}
	mustHint(t, r, "feat: has uncommitted changes")
	mustHint(t, r, "fix", "git rebase", wts["fix"])
	if !h.contains(wts["docs"], pushed) {
		t.Error("docs was not rebased")
	}
}

func TestSyncUntrackedFileInTheWay(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat")
	feat := h.head(wts["feat"])
	h.sb.WriteFile(filepath.Join(wts["feat"], "new.txt"), "mine\n")
	pushFile(h, origin, "main", "new.txt", "theirs\n")

	r := h.run("sync", "feat")
	r.mustCode(t, 1)
	if !strings.HasPrefix(r.stdout, "failed feat: ") {
		t.Errorf("stdout = %q", r.stdout)
	}
	if h.head(wts["feat"]) != feat || h.rebasing(wts["feat"]) {
		t.Error("feat was not left as it was")
	}
	if b, _ := os.ReadFile(filepath.Join(wts["feat"], "new.txt")); string(b) != "mine\n" {
		t.Errorf("new.txt = %q", b)
	}
}

func TestSyncDryRun(t *testing.T) {
	h := newHarness(t)
	repo, origin, wts := syncRepo(h, "docs", "feat", "fix")
	pushFile(h, origin, "main", "main.txt", "main\n")
	h.sb.Git(repo, "fetch", "-q", "origin")
	h.sb.Git(wts["fix"], "rebase", "-q", "origin/main")
	pushFile(h, origin, "main", "later.txt", "later\n")
	h.sb.WriteFile(filepath.Join(wts["docs"], "docs.txt"), "changed\n")
	before := map[string]string{}
	for n, p := range wts {
		before[n] = h.head(p)
	}
	remote := h.sb.Git(repo, "rev-parse", "origin/main")

	r, n := h.fetches("sync", "--all", "--dry-run")
	r.mustCode(t, 5)
	mustStdout(t, r, "would fetch origin", "skipped docs: has uncommitted changes", "would rebase feat onto origin/main", "fix is up to date with origin/main")
	if n != 0 {
		t.Errorf("git fetch ran %d times under --dry-run", n)
	}
	for name, p := range wts {
		if h.head(p) != before[name] {
			t.Errorf("%s moved", name)
		}
	}
	if got := h.sb.Git(repo, "rev-parse", "origin/main"); got != remote {
		t.Error("origin/main moved")
	}
}

func TestSyncJSON(t *testing.T) {
	h := newHarness(t)
	_, origin, wts := syncRepo(h, "feat", "fix")
	commitIn(h, wts["fix"], "f.txt", "fix\n")
	pushFile(h, origin, "main", "f.txt", "main\n")

	r := h.run("sync", "feat", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.sync.v1" {
		t.Errorf("schema = %v", doc["schema"])
	}
	ws, _ := doc["worktrees"].([]any)
	if len(ws) != 1 {
		t.Fatalf("worktrees = %v", doc["worktrees"])
	}
	w := ws[0].(map[string]any)
	if w["name"] != "feat" || w["result"] != "rebased" || w["base"] != "origin/main" || w["branch"] != "feat" || w["head"] != h.head(wts["feat"]) {
		t.Errorf("worktree = %v", w)
	}

	r = h.run("sync", "fix", "--json")
	r.mustCode(t, 6)
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
	if e := decodeOne(t, r.stderr); e["schema"] != "wt.error.v1" || e["code"] != float64(6) {
		t.Errorf("stderr = %v", e)
	}
}

func TestSyncCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _, _ := syncRepo(h, "feat", "usb")
	h.sb.LockWorktree(repo, h.sb.Path("repo.worktrees", "usb"), "")
	h.sb.AddDetachedWorktree(repo, h.sb.Path("repo.worktrees", "scratch"))

	values, _, _ := h.complete("sync", "feat", "")
	if !slices.Contains(values, "usb") {
		t.Errorf("usb not offered: %v", values)
	}
	if slices.Contains(values, "feat") || slices.Contains(values, "scratch") {
		t.Errorf("feat or scratch offered: %v", values)
	}
	values, _, _ = h.complete("sync", "--base", "")
	if !slices.Contains(values, "origin/main") || !slices.Contains(values, "main") {
		t.Errorf("--base offers %v", values)
	}
}

package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
)

// logLines splits a log, or anything hooks wrote, into lines, without the
// \r cmd.exe writes on Windows.
func logLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// readLog reads the log at path, or fails.
func readLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	return logLines(string(b))
}

// --- 2.3: __run-hooks ---

func TestRunHooksWritesTheLog(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	r := h.run("__run-hooks", "--event", "post-remove", "--dir", dir, "--", "echo a", "echo b")
	r.mustCode(t, 0)
	want := []string{
		"==> post-remove  echo a", "a", "==> exited with 0",
		"==> post-remove  echo b", "b", "==> exited with 0",
	}
	if got := logLines(r.stdout); !slices.Equal(got, want) {
		t.Errorf("log:\n%q\nwant\n%q", got, want)
	}
	if r.stderr != "" {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestRunHooksStopsAtTheFirstFailure(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	r := h.run("__run-hooks", "--event", "post-cd", "--dir", dir, "--", "exit 1", "echo second> second.txt")
	r.mustCode(t, 0)
	if want := []string{"==> post-cd  exit 1", "==> exited with 1"}; !slices.Equal(logLines(r.stdout), want) {
		t.Errorf("log = %q, want %q", logLines(r.stdout), want)
	}
	if exists(filepath.Join(dir, "second.txt")) {
		t.Error("the second command ran")
	}
}

func TestRunHooksCommandThatDoesNotStart(t *testing.T) {
	h := newHarness(t)
	missing := filepath.Join(t.TempDir(), "missing")
	r := h.run("__run-hooks", "--event", "post-remove", "--dir", missing, "--", "echo a", "echo b")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	if len(lines) != 2 || lines[0] != "==> post-remove  echo a" || !strings.HasPrefix(lines[1], "==> could not start: ") {
		t.Errorf("log = %q, want the header and could not start", lines)
	}
}

func TestRunHooksIsHidden(t *testing.T) {
	h := newHarness(t)
	r := h.run("-h")
	r.mustCode(t, 0)
	if strings.Contains(r.stdout, "run-hooks") {
		t.Errorf("wt -h shows __run-hooks:\n%s", r.stdout)
	}
	for _, word := range []string{"", "_", "__"} {
		got, _, _ := h.complete(word)
		for _, v := range got {
			if strings.Contains(v, "run-hooks") {
				t.Errorf("wt %q<TAB> offers %q", word, v)
			}
		}
	}
}

// hookLine is the line of the [hooks] table that sets event to commands.
func hookLine(event string, commands ...string) string {
	quoted := make([]string, len(commands))
	for i, c := range commands {
		quoted[i] = strconv.Quote(c)
	}
	return event + " = [" + strings.Join(quoted, ", ") + "]\n"
}

// hooks writes the user configuration: top, then the [hooks] table with
// lines.
func (h *harness) hooks(top string, lines ...string) {
	h.t.Helper()
	h.userConfig(top + "[hooks]\n" + strings.Join(lines, ""))
}

// fileText is the contents of the file at path, trimmed of the spaces and
// line ends that sh and cmd.exe write differently.
func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
}

// lastLines are the last n lines of s.
func lastLines(s string, n int) []string {
	lines := logLines(s)
	if len(lines) < n {
		return lines
	}
	return lines[len(lines)-n:]
}

// logPath is where the log of event for the worktree named name of the
// repository whose common git directory is common goes.
func logPath(common, name, event string) string {
	return filepath.Join(common, "wt", "logs", name, event+".log")
}

// --- 3.2: wt create ---

// Scenario "Order of the create events".
func TestCreateHookOrder(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.hooks("",
		hookLine("pre-create", "git branch --list feat > pre.txt"),
		hookLine("post-create", "git branch --show-current > post.txt"))
	path := h.mustCreate("feat", "feat")
	if got := fileText(t, filepath.Join(repo, "pre.txt")); got != "" {
		t.Errorf("pre.txt = %q, want empty: the branch existed before pre-create", got)
	}
	if got := fileText(t, filepath.Join(path, "post.txt")); got != "feat" {
		t.Errorf("post.txt = %q, want feat", got)
	}
}

// Scenario "Worktree of another repository".
func TestCreateHookInAnotherRepository(t *testing.T) {
	h := newHarness(t)
	root, _, web := createWorkspace(h)
	h.hooks("repos_root = ["+strconv.Quote(root)+"]\n", hookLine("post-create", "echo {repo} > repo.txt"))
	h.cwd = web
	r := h.run("create", "api:feat")
	r.mustCode(t, 0)
	path := createdPath(t, r.stdout)
	mustSamePath(t, path, filepath.Join(root, "api.worktrees", "feat"))
	if got := fileText(t, filepath.Join(path, "repo.txt")); got != "api" {
		t.Errorf("repo.txt = %q, want api", got)
	}
}

// Scenarios "Values of a create event" and "Name made by worktree_path".
func TestCreateHookValues(t *testing.T) {
	t.Run("values", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo {repo} {name} {branch} {base} > vars.txt"))
		path := h.mustCreate("feature/x", "feature/x")
		if got := fileText(t, filepath.Join(path, "vars.txt")); got != "repo feature-x feature/x origin/main" {
			t.Errorf("vars.txt = %q", got)
		}
	})
	t.Run("name made by worktree_path", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("worktree_path = '{repo_parent}/{repo}.worktrees/{branch|sanitize}'\nbranch_prefix = 'jm/'\n",
			hookLine("post-create", "echo {name} > name.txt"))
		path := h.mustCreate("jm/auth", "auth")
		if got := fileText(t, filepath.Join(path, "name.txt")); got != "jm-auth" {
			t.Errorf("name.txt = %q, want jm-auth", got)
		}
	})
	t.Run("path and repo_path", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo {path}> path.txt", "echo {repo_path}> repo_path.txt"))
		path := h.mustCreate("feat", "feat")
		mustSamePath(t, fileText(t, filepath.Join(path, "path.txt")), path)
		mustSamePath(t, fileText(t, filepath.Join(path, "repo_path.txt")), repo)
	})
}

// Scenario "Literal braces" of hooks.
func TestCreateHookLiteralBraces(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "awk '{{print $1}}' in.txt"))
	r := h.run("create", "feat", "--dry-run")
	r.mustCode(t, 0)
	if !slices.Contains(logLines(r.stdout), "would run post-create hook: awk '{print $1}' in.txt") {
		t.Errorf("stdout = %q", r.stdout)
	}
}

// Scenarios "In order" and "First failure stops the event".
func TestCreateHookCommandsInOrder(t *testing.T) {
	t.Run("in order", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo a> out.txt", "echo b>> out.txt"))
		path := h.mustCreate("feat", "feat")
		if got := fileText(t, filepath.Join(path, "out.txt")); got != "a\nb" {
			t.Errorf("out.txt = %q, want a and b", got)
		}
	})
	t.Run("first failure stops the event", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("post-create", "exit 3", "echo second> second.txt"))
		r := h.run("create", "feat")
		r.mustCode(t, 7)
		if exists(h.sb.Path("repo.worktrees", "feat", "second.txt")) {
			t.Error("the second command ran")
		}
	})
}

// Scenario "Interpreter operators (macOS, Linux)".
func TestCreateHookInterpreterOperators(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh's operators; the scenario for Windows is TestCreateHookInterpreterOperatorsWindows")
	}
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "echo a > a.txt && echo b > b.txt"))
	path := h.mustCreate("feat", "feat")
	if !exists(filepath.Join(path, "a.txt")) || !exists(filepath.Join(path, "b.txt")) {
		t.Error("a.txt or b.txt is missing")
	}
}

// Scenario "Output on stderr".
func TestCreateHookOutputOnStderr(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "echo hello"))
	r := h.run("create", "feat")
	r.mustCode(t, 0)
	lines := logLines(r.stderr)
	if i := slices.Index(lines, "==> post-create  echo hello"); i < 0 || i+1 >= len(lines) || lines[i+1] != "hello" {
		t.Errorf("stderr = %q, want the header followed by hello", lines)
	}
	if strings.Contains(r.stderr, "\x1b[") {
		t.Errorf("stderr has color: %q", r.stderr)
	}
	want := "created worktree " + createdPath(t, r.stdout) + " on new branch feat from origin/main\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

// Scenario "The command waits (macOS, Linux)".
func TestCreateHookWaits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh's sleep; the scenario for Windows is TestCreateHookWaitsWindows")
	}
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "sleep 1; touch prepared"))
	h.run("create", "feat", "-x", "test -f prepared").mustCode(t, 0)
}

// Scenario "pre-create fails", and "Report after a hook" of cli-contract.
func TestCreatePreCreateFails(t *testing.T) {
	t.Run("pre-create fails", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		d := h.activate()
		h.hooks("", hookLine("pre-create", "exit 1"))
		r := h.run("create", "feat", "-x", "echo ran> ran.txt")
		r.mustCode(t, 7)
		last := lastLines(r.stderr, 2)
		if last[0] != "wt: pre-create hook failed: exit 1 exited with 1" || !strings.HasPrefix(last[1], "hint: ") || !strings.Contains(last[1], "--no-hooks") {
			t.Errorf("stderr ends with %q", last)
		}
		if h.branchHead(repo, "feat") != "" || exists(h.sb.Path("repo.worktrees", "feat")) || h.worktreeCount(repo) != 1 {
			t.Error("something was created")
		}
		if got := readDirective(t, d); got != "" {
			t.Errorf("directive = %q", got)
		}
		if r.stdout != "" {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
	t.Run("report after a hook", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("pre-create", "echo checking", "exit 3"))
		r := h.run("create", "feat")
		r.mustCode(t, 7)
		lines := logLines(r.stderr)
		if want := []string{"==> pre-create  echo checking", "checking", "==> pre-create  exit 3"}; len(lines) < 3 || !slices.Equal(lines[:3], want) {
			t.Errorf("stderr starts with %q, want %q", lines, want)
		}
		last := lastLines(r.stderr, 2)
		if last[0] != "wt: pre-create hook failed: exit 3 exited with 3" || !strings.Contains(last[1], "--no-hooks") {
			t.Errorf("stderr ends with %q", last)
		}
		if r.stdout != "" {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
}

// Scenarios "post-create fails" and "Not run when a hook fails".
func TestCreatePostCreateFails(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	d := h.activate()
	h.hooks("", hookLine("post-create", "exit 2"))
	r := h.run("create", "feat", "-x", "echo ran> ran.txt")
	r.mustCode(t, 7)
	path := h.sb.Path("repo.worktrees", "feat")
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}
	last := lastLines(r.stderr, 2)
	if last[0] != "wt: post-create hook failed: exit 2 exited with 2" || !strings.HasPrefix(last[1], "hint: ") || !strings.Contains(last[1], path) {
		t.Errorf("stderr ends with %q, want a hint naming %s", last, path)
	}
	if h.branchHead(repo, "feat") == "" || h.worktreeCount(repo) != 2 || !exists(path) {
		t.Error("the worktree or the branch is missing")
	}
	mustDirective(t, d, path)
	if exists(filepath.Join(path, "ran.txt")) {
		t.Error("-x ran")
	}
}

// Scenarios "Hooks of a repository are ignored" and "Create without hooks".
func TestCreateHooksNotRun(t *testing.T) {
	t.Run("hooks of a repository", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "[hooks]\n"+hookLine("post-create", "echo pwned > pwned.txt"))
		r := h.run("create", "feat")
		r.mustCode(t, 0)
		if exists(filepath.Join(createdPath(t, r.stdout), "pwned.txt")) {
			t.Error("the repository's hook ran")
		}
		if !strings.Contains(r.stderr, "wt: warning: ") || !strings.Contains(r.stderr, "hooks.post-create") || !strings.Contains(r.stderr, ".wt.toml") {
			t.Errorf("stderr = %q, want a warning naming hooks.post-create and .wt.toml", r.stderr)
		}
	})
	t.Run("--no-hooks", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("pre-create", "exit 1"), hookLine("post-create", "echo ran> ran.txt"))
		path := h.mustCreate("feat", "feat", "--no-hooks")
		if exists(filepath.Join(path, "ran.txt")) {
			t.Error("post-create ran")
		}
	})
}

// Scenarios "Preview of a create with hooks" of hooks and "Preview with
// hooks" of create-worktree.
func TestCreatePreviewWithHooks(t *testing.T) {
	t.Run("with a fetch", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("pre-create", "echo {name}"), hookLine("post-create", "npm ci"))
		r := h.run("create", "feat", "--dry-run", "--no-cd")
		r.mustCode(t, 0)
		path := h.sb.Path("repo.worktrees", "feat")
		lines := logLines(r.stdout)
		if len(lines) == 4 {
			mustSamePath(t, strings.TrimSuffix(strings.TrimPrefix(lines[2], "would create worktree "), " on new branch feat from origin/main"), path)
			lines[2] = "would create worktree <path> on new branch feat from origin/main"
		}
		want := []string{"would fetch origin", "would run pre-create hook: echo feat", "would create worktree <path> on new branch feat from origin/main", "would run post-create hook: npm ci"}
		if !slices.Equal(lines, want) {
			t.Errorf("stdout = %q, want %q", lines, want)
		}
		if h.worktreeCount(repo) != 1 || exists(path) {
			t.Error("a worktree was created")
		}
		if exists(filepath.Join(repo, ".git", "wt")) {
			t.Error("a log was written")
		}
	})
	t.Run("through the function", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.activate()
		h.hooks("fetch_before_create = false\n", hookLine("pre-create", "echo check {name}"), hookLine("post-create", "npm ci"))
		r := h.run("create", "feat", "--dry-run", "-x", "claude")
		r.mustCode(t, 0)
		lines := logLines(r.stdout)
		if len(lines) != 5 || lines[0] != "would run pre-create hook: echo check feat" || !strings.HasPrefix(lines[1], "would create worktree ") ||
			lines[2] != "would run post-create hook: npm ci" || !strings.HasPrefix(lines[3], "would change directory to ") || lines[4] != "would run claude" {
			t.Errorf("stdout = %q", lines)
		}
	})
	t.Run("--no-hooks", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("pre-create", "echo {name}"))
		r := h.run("create", "feat", "--dry-run", "--no-cd", "--no-hooks")
		r.mustCode(t, 0)
		if strings.Contains(r.stdout, "would run") {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
	t.Run("JSON", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		h.hooks("", hookLine("pre-create", "echo {name}"))
		r := h.run("create", "feat", "--dry-run", "--json")
		r.mustCode(t, 0)
		if doc := decodeOne(t, r.stdout); doc["schema"] != "wt.create.v1" {
			t.Errorf("stdout = %s", r.stdout)
		}
	})
}

// Scenarios "Hook output with JSON" and "Failed hook with JSON".
func TestCreateHooksWithJSON(t *testing.T) {
	t.Run("output in the log", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo hello"))
		r := h.run("create", "feat", "--json")
		r.mustCode(t, 0)
		if doc := decodeOne(t, r.stdout); doc["schema"] != "wt.create.v1" {
			t.Errorf("stdout = %s", r.stdout)
		}
		if r.stderr != "" {
			t.Errorf("stderr = %q", r.stderr)
		}
		log := readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "post-create"))
		if want := []string{"==> post-create  echo hello", "hello", "==> exited with 0"}; !slices.Equal(log, want) {
			t.Errorf("log = %q, want %q", log, want)
		}
	})
	t.Run("failed hook", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo hello", "exit 1"))
		r := h.run("create", "feat", "--json")
		r.mustCode(t, 7)
		if r.stdout != "" {
			t.Errorf("stdout = %q", r.stdout)
		}
		doc := decodeOne(t, r.stderr)
		if doc["schema"] != "wt.error.v1" || doc["code"] != 7.0 {
			t.Errorf("stderr = %s", r.stderr)
		}
		hints := fmt.Sprint(doc["hints"])
		path := h.sb.Path("repo.worktrees", "feat")
		log := logPath(filepath.Join(repo, ".git"), "feat", "post-create")
		if !strings.Contains(hints, path) || !strings.Contains(hints, log) {
			t.Errorf("hints = %s, want %s and %s", hints, path, log)
		}
		if !slices.Contains(readLog(t, log), "hello") {
			t.Errorf("log = %q", readLog(t, log))
		}
	})
}

// Scenario "Line after the post-create hook".
func TestCreateLineAfterThePostCreateHook(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.combined = true
	h.hooks("", hookLine("post-create", "echo installing"))
	r := h.run("create", "feat")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	installing := slices.Index(lines, "installing")
	created := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "created worktree ") })
	if installing < 0 || created < 0 || installing > created {
		t.Errorf("output = %q, want installing before the created line", lines)
	}
}

// --- 3.3: wt remove ---

// Scenario "Remove without hooks".
func TestRemoveWithoutHooks(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.hooks("", hookLine("pre-remove", "exit 1"))
	h.run("remove", "feat", "--no-hooks").mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
}

// Scenario "Hooks around the removal".
func TestRemoveHooksAroundTheRemoval(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.hooks("",
		hookLine("pre-remove", "git status --short > {repo_path}/pre.txt"),
		hookLine("post-remove", "git worktree list > post.txt"))
	h.run("remove", "feat").mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
	if !exists(filepath.Join(repo, "pre.txt")) {
		t.Error("the main worktree has no pre.txt")
	}
	post := fileText(t, filepath.Join(repo, "post.txt"))
	if post == "" || strings.Contains(post, "repo.worktrees") {
		t.Errorf("post.txt = %q, want the worktrees without feat", post)
	}
}

// Scenario "No post-remove when the removal fails (macOS, Linux)".
func TestRemoveNoPostRemoveWhenTheRemovalFails(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.commitFile(feat, filepath.Join("sub", "file.txt"), "x")
	h.sb.Push(feat, "origin", "HEAD:main")
	h.sb.ReadOnlyDir(t, filepath.Join(feat, "sub"))
	h.hooks("", hookLine("post-remove", "echo ran> ran.txt"))
	h.run("remove", "feat").mustCode(t, 1)
	if exists(logPath(filepath.Join(repo, ".git"), "feat", "post-remove")) || exists(filepath.Join(repo, "ran.txt")) {
		t.Error("post-remove ran")
	}
}

// post-remove starts once the worktree is gone, even when its branch could
// not be deleted.
func TestRemovePostRemoveWhenTheBranchCannotBeDeleted(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.hooks("", hookLine("post-remove", "echo bye"))
	// A lock file makes git refuse to delete the branch.
	h.sb.WriteFile(filepath.Join(repo, ".git", "refs", "heads", "feat.lock"), "")
	r := h.run("remove", "feat")
	r.mustCode(t, 1)
	h.mustBeRemoved(repo, feat)
	if !strings.Contains(r.stderr, "cannot delete branch feat") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if !slices.Contains(readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "post-remove")), "bye") {
		t.Error("post-remove did not run")
	}
}

// Scenario "Preview with hooks" of remove-worktree.
func TestRemovePreviewWithHooks(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.hooks("", hookLine("pre-remove", "echo bye {name}"), hookLine("post-remove", "docker rm -f {repo}-{name}-db"))
	r := h.run("remove", "feat", "--dry-run")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	if len(lines) == 4 {
		mustSamePath(t, strings.TrimPrefix(lines[1], "would remove worktree "), feat)
		lines[1] = "would remove worktree <path>"
	}
	want := []string{"would run pre-remove hook: echo bye feat", "would remove worktree <path>", "would delete branch feat", "would run post-remove hook in the background: docker rm -f repo-feat-db"}
	if !slices.Equal(lines, want) {
		t.Errorf("stdout = %q, want %q", lines, want)
	}
	h.mustBeIntact(repo, feat, "feat")
	if exists(filepath.Join(repo, ".git", "wt")) {
		t.Error("a log was written")
	}
}

// Scenario "pre-remove fails".
func TestRemovePreRemoveFails(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := removeRepo(h)
	d := h.activate()
	h.cwd = feat
	h.hooks("", hookLine("pre-remove", "exit 1"), hookLine("post-remove", "echo ran> {repo_path}/ran.txt"))
	calls := h.recordChdir(nil)
	r := h.run("remove", "@")
	r.mustCode(t, 7)
	h.mustBeIntact(repo, feat, "feat")
	if last := lastLines(r.stderr, 2); last[0] != "wt: pre-remove hook failed: exit 1 exited with 1" || !strings.Contains(last[1], "--no-hooks") {
		t.Errorf("stderr ends with %q", last)
	}
	if got := readDirective(t, d); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
	if len(*calls) != 0 {
		t.Errorf("Chdir called with %q", *calls)
	}
	if exists(filepath.Join(repo, "ran.txt")) {
		t.Error("post-remove ran")
	}
}

// Scenarios "A failure does not change the exit code" and "Log of
// post-remove".
func TestRemovePostRemoveLog(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	t.Run("log", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := removeRepo(h)
		h.hooks("", hookLine("post-remove", "echo bye"))
		r := h.run("remove", "feat")
		r.mustCode(t, 0)
		log := logPath(filepath.Join(repo, ".git"), "feat", "post-remove")
		if want := []string{"==> post-remove  echo bye", "bye", "==> exited with 0"}; !slices.Equal(readLog(t, log), want) {
			t.Errorf("log = %q, want %q", readLog(t, log), want)
		}
		if notice := "==> post-remove  running in the background, log: "; !strings.Contains(r.stderr, notice) {
			t.Errorf("stderr = %q, want %q", r.stderr, notice)
		} else {
			_, got, _ := strings.Cut(strings.TrimSpace(r.stderr), notice)
			mustSamePath(t, got, log)
		}
		if lines := logLines(r.stdout); len(lines) != 2 || !strings.HasPrefix(lines[0], "removed worktree ") {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
	t.Run("a failure does not change the exit code", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := removeRepo(h)
		h.hooks("", hookLine("post-remove", "exit 1"))
		h.run("remove", "feat").mustCode(t, 0)
		log := readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "post-remove"))
		if log[len(log)-1] != "==> exited with 1" {
			t.Errorf("log = %q", log)
		}
	})
	t.Run("JSON", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := removeRepo(h)
		h.hooks("", hookLine("post-remove", "echo bye"))
		r := h.run("remove", "feat", "--json")
		r.mustCode(t, 0)
		if doc := decodeOne(t, r.stdout); doc["schema"] != "wt.remove.v1" || r.stderr != "" {
			t.Errorf("stdout %s, stderr %q", r.stdout, r.stderr)
		}
		if !slices.Contains(readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "post-remove")), "bye") {
			t.Error("post-remove did not run")
		}
	})
}

// Scenario "Bare repository" of hooks.
func TestRemoveHookLogOfABareRepository(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	proj := h.sb.Path("root", "proj")
	h.sb.InitDotBare(proj)
	feat := filepath.Join(proj, "feat")
	h.sb.AddWorktree(proj, feat, "feat")
	h.cwd = proj
	h.hooks("", hookLine("post-remove", "echo bye"))
	h.run("remove", "feat").mustCode(t, 0)
	if !slices.Contains(readLog(t, logPath(filepath.Join(proj, ".bare"), "feat", "post-remove")), "bye") {
		t.Error("the log has no bye")
	}
}

// pre-remove runs in the main worktree when the worktree's directory no
// longer exists.
func TestRemovePreRemoveWithoutTheDirectory(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.MakePrunable(feat)
	h.hooks("", hookLine("pre-remove", "echo {name}> pre.txt"))
	h.run("remove", "feat").mustCode(t, 0)
	if got := fileText(t, filepath.Join(repo, "pre.txt")); got != "feat" {
		t.Errorf("pre.txt in the main worktree = %q, want feat", got)
	}
}

// --- 3.4: wt cd ---

// Scenario "Hooks around the jump".
func TestCdHooksAroundTheJump(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := cdRepo(h)
	d := h.activate()
	h.hooks("", hookLine("pre-cd", "echo {name}> {repo_path}/pre.txt"), hookLine("post-cd", "echo {branch}> post.txt"))
	r := h.run("cd", "feat")
	r.mustCode(t, 0)
	if got := fileText(t, filepath.Join(repo, "pre.txt")); got != "feat" {
		t.Errorf("pre.txt = %q, want feat", got)
	}
	if got := fileText(t, filepath.Join(feat, "post.txt")); got != "feat" {
		t.Errorf("post.txt = %q, want feat", got)
	}
	mustDirective(t, d, feat)
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}
}

// Scenario "Preview a jump with hooks".
func TestCdPreviewWithHooks(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)
	d := h.activate()
	h.hooks("", hookLine("pre-cd", "echo in {name}"), hookLine("post-cd", "git fetch"))
	r := h.run("cd", "--dry-run", "feat")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	if len(lines) == 3 {
		mustSamePath(t, strings.TrimPrefix(lines[1], "would change directory to "), feat)
		lines[1] = "would change directory to <path>"
	}
	want := []string{"would run pre-cd hook: echo in feat", "would change directory to <path>", "would run post-cd hook in the background: git fetch"}
	if !slices.Equal(lines, want) {
		t.Errorf("stdout = %q, want %q", lines, want)
	}
	if got := readDirective(t, d); got != "" {
		t.Errorf("directive = %q", got)
	}
	if exists(filepath.Join(repo, ".git", "wt")) {
		t.Error("a log was written")
	}
}

// Scenarios "pre-cd fails" and "Jump without hooks".
func TestCdPreCdFails(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := cdRepo(h)
	d := h.activate()
	h.hooks("", hookLine("pre-cd", "exit 1"), hookLine("post-cd", "echo ran> {repo_path}/ran.txt"))
	r := h.run("cd", "feat")
	r.mustCode(t, 7)
	if got := readDirective(t, d); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
	if last := lastLines(r.stderr, 2); last[0] != "wt: pre-cd hook failed: exit 1 exited with 1" || !strings.Contains(last[1], "--no-hooks") {
		t.Errorf("stderr ends with %q", last)
	}
	if exists(filepath.Join(repo, "ran.txt")) {
		t.Error("post-cd ran")
	}

	h.run("cd", "feat", "--no-hooks").mustCode(t, 0)
	mustDirective(t, d, feat)
	if exists(filepath.Join(repo, "ran.txt")) {
		t.Error("post-cd ran with --no-hooks")
	}
}

// Scenario "Previous directory outside every repository".
func TestCdPreviousDirectoryOutsideEveryRepository(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	_, feat := cdRepo(h)
	d := h.activate()
	h.hooks("", hookLine("pre-cd", "echo x> ran.txt"), hookLine("post-cd", "echo x> post.txt"))
	h.run("cd", "feat").mustCode(t, 0)
	if !exists(filepath.Join(feat, "ran.txt")) {
		t.Fatal("pre-cd did not run for feat")
	}
	outside := h.sb.Path("outside")
	h.sb.Setenv("WT_PREVIOUS_DIR", outside)
	h.cwd = feat
	h.run("cd", "-").mustCode(t, 0)
	mustDirective(t, d, outside)
	if exists(filepath.Join(outside, "ran.txt")) || exists(filepath.Join(outside, "post.txt")) {
		t.Error("a cd hook ran outside every repository")
	}
}

// wt cd - to a directory inside a worktree runs the hooks at its root.
func TestCdPreviousDirectoryInsideAWorktree(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)
	d := h.activate()
	sub := filepath.Join(feat, "deep", "dir")
	h.mkdir(sub)
	h.hooks("", hookLine("pre-cd", "echo {name}> ran.txt"))
	h.sb.Setenv("WT_PREVIOUS_DIR", sub)
	h.run("cd", "-").mustCode(t, 0)
	mustDirective(t, d, sub)
	if got := fileText(t, filepath.Join(feat, "ran.txt")); got != "feat" {
		t.Errorf("ran.txt at the root of feat = %q, want feat", got)
	}
}

// Scenario "Removing from inside does not fire cd events".
func TestRemoveFiresNoCdEvent(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, feat := removeRepo(h)
	d := h.activate()
	ran := h.mkdir(h.sb.Path("cd-ran"))
	h.hooks("",
		hookLine("pre-cd", "echo x> "+filepath.Join(ran, "pre")),
		hookLine("post-cd", "echo x> "+filepath.Join(ran, "post")))
	h.cwd = feat
	h.run("remove", "@").mustCode(t, 0)
	mustDirective(t, d, repo)
	if entries, _ := os.ReadDir(ran); len(entries) != 0 {
		t.Errorf("a cd hook ran: %v", entries)
	}
}

// Scenario "Printing a path runs no hook".
func TestPathRunsNoHook(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)
	h.hooks("", hookLine("pre-cd", "exit 1"))
	r := h.run("path", "feat")
	r.mustCode(t, 0)
	mustSamePath(t, strings.TrimSpace(r.stdout), feat)
}

// Scenarios "Notice on stderr" and "Each run replaces the log".
func TestCdPostCdLog(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, _ := cdRepo(h)
	h.activate()
	h.hooks("", hookLine("post-cd", "echo {name}"))
	var r result
	for range 2 {
		r = h.run("cd", "feat")
		r.mustCode(t, 0)
	}
	log := logPath(filepath.Join(repo, ".git"), "feat", "post-cd")
	if lines := readLog(t, log); !slices.Equal(lines, []string{"==> post-cd  echo feat", "feat", "==> exited with 0"}) {
		t.Errorf("log = %q, want one run", lines)
	}
	notice := "==> post-cd  running in the background, log: "
	got, ok := strings.CutPrefix(strings.TrimSpace(r.stderr), notice)
	if !ok {
		t.Fatalf("stderr = %q, want %q", r.stderr, notice)
	}
	mustSamePath(t, got, log)
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}
}

// With --json, pre-cd writes to its log and the document is the same.
func TestCdHooksWithJSON(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, _ := cdRepo(h)
	h.activate()
	h.hooks("", hookLine("pre-cd", "echo hi"), hookLine("post-cd", "echo bye"))
	r := h.run("cd", "feat", "--json")
	r.mustCode(t, 0)
	if doc := decodeOne(t, r.stdout); doc["schema"] != "wt.cd.v1" || r.stderr != "" {
		t.Errorf("stdout %s, stderr %q", r.stdout, r.stderr)
	}
	if !slices.Contains(readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "pre-cd")), "hi") {
		t.Error("the log of pre-cd has no hi")
	}
	if !slices.Contains(readLog(t, logPath(filepath.Join(repo, ".git"), "feat", "post-cd")), "bye") {
		t.Error("the log of post-cd has no bye")
	}
}

// Scenario "Command without hooks".
func TestNoHooksOnOtherCommands(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	for _, args := range [][]string{{"list"}, {"path", "^"}, {"each", "--no-hooks", "echo"}, {"lock", "x"}, {"prune"}} {
		if args[0] != "each" {
			args = append(args, "--no-hooks")
		}
		r := h.run(args...)
		r.mustCode(t, 2)
		if !strings.Contains(r.stderr, "--no-hooks") || !strings.Contains(r.stderr, "unknown flag") {
			t.Errorf("wt %v: stderr = %q", args, r.stderr)
		}
	}
}

// Without cd hooks, wt cd - runs no git command, as before hooks.
func TestCdPreviousWithoutHooksRunsNoGit(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)
	h.activate()
	h.hooks("", hookLine("pre-create", "exit 1"), hookLine("post-remove", "exit 1"))
	h.sb.Setenv("WT_PREVIOUS_DIR", feat)
	calls, restore := cli.RecordGit()
	defer restore()
	h.run("cd", "-").mustCode(t, 0)
	if len(*calls) != 0 {
		t.Errorf("git ran: %q", *calls)
	}
}

// --- 3.5: help ---

func TestHelpNamesTheHooks(t *testing.T) {
	h := newHarness(t)
	for cmd, events := range map[string][]string{
		"create": {"pre-create", "post-create"},
		"cd":     {"pre-cd", "post-cd"},
		"remove": {"pre-remove", "post-remove"},
	} {
		r := h.run(cmd, "-h")
		r.mustCode(t, 0)
		for _, want := range append(events, "--no-hooks", "run no hook", "background", "wt config --help") {
			if cmd == "create" && want == "background" {
				continue
			}
			if !strings.Contains(r.stdout, want) {
				t.Errorf("wt %s -h does not mention %q:\n%s", cmd, want, r.stdout)
			}
		}
	}
	r := h.run("config", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"[hooks]", "post-create = [", "pre-cd", "post-remove", "{base}", "{{ and }}", "background", "sh -c", "cmd.exe"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("wt config -h does not mention %q:\n%s", want, r.stdout)
		}
	}
}

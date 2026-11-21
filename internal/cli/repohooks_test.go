package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/approval"
	"github.com/jMautone/worktree-manager/internal/cli"
)

// Tests of project-hooks-approval: hooks in .wt.toml, their approval, wt
// hook list and wt hook approve.

// repoHooks writes the .wt.toml of the worktree at dir with the [hooks]
// table of lines.
func (h *harness) repoHooks(dir string, lines ...string) string {
	h.t.Helper()
	p := filepath.Join(dir, ".wt.toml")
	h.sb.WriteFile(p, "[hooks]\n"+strings.Join(lines, ""))
	return p
}

// approve runs wt hook approve with the working directory at dir.
func (h *harness) approve(dir string, args ...string) result {
	h.t.Helper()
	cwd := h.cwd
	defer func() { h.cwd = cwd }()
	h.cwd = dir
	r := h.run(append([]string{"hook", "approve"}, args...)...)
	r.mustCode(h.t, 0)
	return r
}

// approvalsPath is the sandbox's approvals.toml.
func (h *harness) approvalsPath() string {
	return filepath.Join(filepath.Dir(h.userConfigPath()), "approvals.toml")
}

// mustNotApproved checks that r failed with exit 8 for command of event.
func mustNotApproved(t *testing.T, r result, event, command string) {
	t.Helper()
	r.mustCode(t, 8)
	re := regexp.MustCompile(`(?m)^wt: ` + regexp.QuoteMeta(event) + ` hook of .*\.wt\.toml is not approved: ` + regexp.QuoteMeta(command) + `$`)
	if !re.MatchString(r.stderr) {
		t.Errorf("stderr = %q, want the %s hook %q not approved", r.stderr, event, command)
	}
}

// --- 3.2: create, cd and remove ---

// Scenarios "Hooks read from the repository" and "Approve then create".
func TestRepoHooksRunOnceApproved(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.repoHooks(repo, hookLine("post-create", "echo {name}> repo.txt"))
	r := h.approve(repo)
	if r.stdout != "approved post-create hook: echo {name}> repo.txt\n" {
		t.Errorf("approve stdout = %q", r.stdout)
	}
	r = h.run("create", "feat")
	r.mustCode(t, 0)
	path := createdPath(t, r.stdout)
	if got := fileText(t, filepath.Join(path, "repo.txt")); got != "feat" {
		t.Errorf("repo.txt = %q, want feat", got)
	}
	if strings.Contains(r.stderr, "hooks.post-create") {
		t.Errorf("stderr = %q, want no warning naming hooks.post-create", r.stderr)
	}
}

// Scenario "Hooks of a repository are ignored" of hooks, and "post-create
// not approved".
func TestRepoHookNotApprovedCreatesNothing(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	d := h.activate()
	h.repoHooks(repo, hookLine("post-create", "echo pwned> pwned.txt"))
	r := h.run("create", "feat", "-x", "echo ran> ran.txt")
	mustNotApproved(t, r, "post-create", "echo pwned> pwned.txt")
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}
	if h.branchHead(repo, "feat") != "" || h.worktreeCount(repo) != 1 || exists(h.sb.Path("repo.worktrees", "feat")) {
		t.Error("a branch, a worktree or its directory was created")
	}
	if exists(filepath.Join(repo, "pwned.txt")) || exists(filepath.Join(repo, "ran.txt")) {
		t.Error("a command ran")
	}
	if got := readDirective(t, d); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
}

// Scenarios "User commands before the repository's" and "A failing user
// command stops the repository's".
func TestRepoHooksAfterTheUsers(t *testing.T) {
	t.Run("order", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("post-create", "echo user> order.txt"))
		h.repoHooks(repo, hookLine("post-create", "echo repo>> order.txt"))
		h.approve(repo)
		path := h.mustCreate("feat", "feat")
		if got := logLines(fileText(t, filepath.Join(path, "order.txt"))); len(got) != 2 || strings.TrimSpace(got[0]) != "user" || strings.TrimSpace(got[1]) != "repo" {
			t.Errorf("order.txt = %q, want user then repo", got)
		}
	})
	t.Run("a failing user command", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.hooks("", hookLine("post-create", "exit 1"))
		h.repoHooks(repo, hookLine("post-create", "echo repo> repo.txt"))
		h.approve(repo)
		h.run("create", "feat").mustCode(t, 7)
		if exists(filepath.Join(h.sb.Path("repo.worktrees", "feat"), "repo.txt")) {
			t.Error("the repository's command ran")
		}
	})
}

// Scenarios "Hooks of the other repository" and "Hint for another
// repository".
func TestRepoHooksOfTheOtherRepository(t *testing.T) {
	h := newHarness(t)
	root, api, web := createWorkspace(h)
	h.repoHooks(web, hookLine("post-create", "echo web> from.txt"))
	h.repoHooks(api, hookLine("post-create", "echo api> from.txt"))
	h.cwd = web

	r := h.run("create", "api:feat")
	mustNotApproved(t, r, "post-create", "echo api> from.txt")
	hint := "wt -C " + api + " hook approve"
	if !strings.Contains(r.stderr, hint) {
		t.Errorf("stderr = %q, want a hint with %q", r.stderr, hint)
	}

	h.approve(api)
	r = h.run("create", "api:feat")
	r.mustCode(t, 0)
	path := createdPath(t, r.stdout)
	mustSamePath(t, path, filepath.Join(root, "api.worktrees", "feat"))
	if got := fileText(t, filepath.Join(path, "from.txt")); got != "api" {
		t.Errorf("from.txt = %q, want api", got)
	}
}

// Scenario "Approved in one repository only".
func TestRepoHookApprovedInOneRepositoryOnly(t *testing.T) {
	h := newHarness(t)
	a := h.sb.Path("a")
	h.sb.InitRepo(a)
	b := h.sb.Path("b")
	h.sb.InitRepo(b)
	h.repoHooks(a, hookLine("post-create", "echo x"))
	h.repoHooks(b, hookLine("post-create", "echo x"))
	h.approve(a)
	h.cwd = b
	mustNotApproved(t, h.run("create", "feat"), "post-create", "echo x")
}

// Scenario "Approved for one event only".
func TestRepoHookApprovedForOneEventOnly(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)
	h.repoHooks(repo, hookLine("post-create", "echo m"))
	h.approve(repo)
	h.repoHooks(repo, hookLine("post-create", "echo m"), hookLine("pre-remove", "echo m"))
	mustNotApproved(t, h.run("remove", "feat"), "pre-remove", "echo m")
	if !exists(feat) {
		t.Error("the worktree was removed")
	}
}

// Scenarios "Command changed after approval" and "Revoked by hand".
func TestRepoHookApprovalLost(t *testing.T) {
	t.Run("command changed", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.repoHooks(repo, hookLine("post-create", "echo ci"))
		h.approve(repo)
		h.repoHooks(repo, hookLine("post-create", "echo ci --force"))
		mustNotApproved(t, h.run("create", "feat"), "post-create", "echo ci --force")
		if h.worktreeCount(repo) != 1 {
			t.Error("a worktree was created")
		}
	})
	t.Run("revoked by hand", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.repoHooks(repo, hookLine("post-create", "echo ci"))
		h.approve(repo)
		h.sb.WriteFile(h.approvalsPath(), "# nothing\n")
		mustNotApproved(t, h.run("create", "feat"), "post-create", "echo ci")
	})
}

// Scenario "Approval valid in every worktree".
func TestRepoHookApprovalValidInEveryWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.repoHooks(repo, hookLine("post-create", "echo {name}> n.txt"))
	h.approve(repo)
	a := h.sb.Path("repo.worktrees", "a")
	h.sb.AddWorktree(repo, a, "a")
	h.repoHooks(a, hookLine("post-create", "echo {name}> n.txt"))
	h.cwd = a
	path := h.mustCreate("b", "b")
	if got := fileText(t, filepath.Join(path, "n.txt")); got != "b" {
		t.Errorf("n.txt = %q, want b", got)
	}
}

// Scenario "Background event not approved".
func TestRepoHookBackgroundEventNotApproved(t *testing.T) {
	defer cli.RunBackgroundInProcess()()
	h := newHarness(t)
	repo, _ := cdRepo(h)
	d := h.activate()
	h.repoHooks(repo, hookLine("post-cd", "git fetch"))
	mustNotApproved(t, h.run("cd", "feat"), "post-cd", "git fetch")
	if got := readDirective(t, d); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
}

// Scenario "Not approved with JSON".
func TestRepoHookNotApprovedWithJSON(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.repoHooks(repo, hookLine("pre-create", "make check"))
	r := h.run("create", "feat", "--json")
	r.mustCode(t, 8)
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}
	doc := decodeOne(t, r.stderr)
	if doc["schema"] != "wt.error.v1" || doc["code"] != float64(8) {
		t.Errorf("stderr = %s, want wt.error.v1 with code 8", r.stderr)
	}
}

// Scenario "Hints in the current repository".
func TestRepoHookHintsInTheCurrentRepository(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.repoHooks(repo, hookLine("pre-create", "make check"))
	r := h.run("create", "feat")
	r.mustCode(t, 8)
	var approve, noHooks bool
	for _, line := range logLines(r.stderr) {
		if hint, ok := strings.CutPrefix(line, "hint: "); ok {
			approve = approve || strings.Contains(hint, "'wt hook approve'")
			noHooks = noHooks || strings.Contains(hint, "--no-hooks")
		}
	}
	if !approve || !noHooks {
		t.Errorf("stderr = %q, want hints with 'wt hook approve' and --no-hooks", r.stderr)
	}
}

// Scenarios "Other events are not checked", "Skipping hooks skips the
// check" and "Dry run not approved".
func TestRepoHookCheck(t *testing.T) {
	t.Run("other events", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.repoHooks(repo, hookLine("pre-remove", "make clean"))
		h.mustCreate("feat", "feat")
	})
	t.Run("--no-hooks", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.repoHooks(repo, hookLine("post-create", "echo ci> ci.txt"))
		path := h.mustCreate("feat", "feat", "--no-hooks")
		if exists(filepath.Join(path, "ci.txt")) {
			t.Error("the repository's hook ran")
		}
	})
	t.Run("--dry-run", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.repoHooks(repo, hookLine("post-create", "npm ci"))
		r := h.run("create", "feat", "--dry-run")
		mustNotApproved(t, r, "post-create", "npm ci")
		if r.stdout != "" {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
}

// Scenario "Preview with repository hooks".
func TestRepoHooksPreview(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.hooks("", hookLine("post-create", "cp ~/.npmrc ."))
	h.repoHooks(repo, hookLine("post-create", "npm ci"))
	h.approve(repo)
	r := h.run("create", "feat", "--dry-run", "--no-cd")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	i := slices.Index(lines, "would run post-create hook: cp ~/.npmrc .")
	if i < 0 || i+1 >= len(lines) || lines[i+1] != "would run post-create hook: npm ci" {
		t.Errorf("stdout:\n%s\nwant the user's post-create line, then the repository's", r.stdout)
	}
}

// Scenario "Invalid repository hook" of project-hooks-approval.
func TestRepoHookInvalid(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.repoHooks(h.cwd, hookLine("pre-remove", "echo {base}"))
	r := h.run("list")
	r.mustCode(t, 1)
	for _, want := range []string{"hooks.pre-remove", ".wt.toml", "base"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
}

// --- 3.3: zero cost ---

func TestRepoWithoutHooksReadsNoApprovals(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \"origin/main\"\n")
	h.sb.WriteFile(h.approvalsPath(), "[[approval")
	h.mustCreate("feat", "feat")
}

// --- 4: wt hook list and wt hook approve ---

// Scenario "User and repository hooks".
func TestHookList(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.hooks("", hookLine("post-create", "cp ~/.npmrc ."))
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	h.approve(h.cwd)
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"), hookLine("pre-remove", "make clean"))
	r := h.run("hook", "list")
	r.mustCode(t, 0)
	lines := logLines(r.stdout)
	want := [][]string{
		{"EVENT", "SOURCE", "APPROVED", "COMMAND"},
		{"post-create", "user", "-", "cp", "~/.npmrc", "."},
		{"post-create", "repo", "yes", "npm", "ci"},
		{"pre-remove", "repo", "no", "make", "clean"},
	}
	if len(lines) != len(want) {
		t.Fatalf("hook list:\n%s", r.stdout)
	}
	for i, w := range want {
		if got := strings.Fields(lines[i]); !slices.Equal(got, w) {
			t.Errorf("line %d = %q, want %q", i, got, w)
		}
	}
}

// Scenarios "No hooks" and "Outside a repository".
func TestHookListEmpty(t *testing.T) {
	h := newHarness(t)
	h.run("hook", "list").mustCode(t, 3)
	h.cwd = h.repo()
	r := h.run("hook", "list")
	r.mustCode(t, 0)
	if r.stdout != "" {
		t.Errorf("stdout = %q, want nothing", r.stdout)
	}
	h.cwd = h.sb.Path("outside")
	h.run("hook", "approve").mustCode(t, 3)
}

// Scenario "JSON" of "Hook list as JSON".
func TestHookListJSON(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	file := h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	r := h.run("hook", "list", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.hook.list.v1" {
		t.Errorf("schema = %v", doc["schema"])
	}
	mustSamePath(t, doc["repo_path"].(string), h.cwd)
	mustSamePath(t, doc["repo_file"].(string), file)
	hooks, _ := doc["hooks"].([]any)
	want := map[string]any{"event": "post-create", "source": "repo", "command": "npm ci", "approved": false}
	if len(hooks) != 1 || !mapsEqual(hooks[0].(map[string]any), want) {
		t.Errorf("hooks = %v, want [%v]", hooks, want)
	}

	os.Remove(file)
	doc = decodeOne(t, h.run("hook", "list", "--json").stdout)
	if doc["repo_file"] != nil {
		t.Errorf("repo_file = %v, want null", doc["repo_file"])
	}
	if hooks, ok := doc["hooks"].([]any); !ok || len(hooks) != 0 {
		t.Errorf("hooks = %v, want []", doc["hooks"])
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Scenario "Broken file".
func TestHookListBrokenApprovals(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	h.sb.WriteFile(h.approvalsPath(), "[[approval")
	r := h.run("hook", "list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, h.approvalsPath()) {
		t.Errorf("stderr = %q, want it to name %s", r.stderr, h.approvalsPath())
	}
}

// Scenarios "Only the events named", "Nothing to approve" and "Unknown
// event".
func TestHookApprove(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"), hookLine("pre-remove", "make clean"))
	r := h.approve(h.cwd, "post-create")
	if r.stdout != "approved post-create hook: npm ci\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	if !strings.Contains(h.run("hook", "list").stdout, "no") {
		t.Error("make clean is shown as approved")
	}
	h.approve(h.cwd)
	before := fileText(t, h.approvalsPath())
	if r := h.approve(h.cwd); r.stdout != "nothing to approve\n" {
		t.Errorf("stdout = %q, want nothing to approve", r.stdout)
	}
	if fileText(t, h.approvalsPath()) != before {
		t.Error("approvals.toml changed")
	}

	r = h.run("hook", "approve", "post-merge")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, "post-merge") {
		t.Errorf("stderr = %q, want it to name post-merge", r.stderr)
	}
}

// Scenarios "Preview" and "JSON" of "Approving hooks in a dry run or as
// JSON".
func TestHookApproveDryRunAndJSON(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	r := h.run("hook", "approve", "--dry-run")
	r.mustCode(t, 0)
	if r.stdout != "would approve post-create hook: npm ci\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	if exists(h.approvalsPath()) {
		t.Error("--dry-run wrote approvals.toml")
	}
	r = h.run("hook", "approve", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	approved, _ := doc["approved"].([]any)
	if doc["schema"] != "wt.hook.approve.v1" || len(approved) != 1 || !mapsEqual(approved[0].(map[string]any), map[string]any{"event": "post-create", "command": "npm ci"}) {
		t.Errorf("stdout = %s", r.stdout)
	}
	r = h.run("hook", "approve", "--json")
	if approved, ok := decodeOne(t, r.stdout)["approved"].([]any); !ok || len(approved) != 0 {
		t.Errorf("second approve: %s, want an empty approved array", r.stdout)
	}
}

// Scenarios "Entry written", "Location (macOS, Linux)" and "Next to
// WT_CONFIG".
func TestHookApproveWritesTheFile(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	h.approve(h.cwd)
	b, err := os.ReadFile(h.approvalsPath())
	if err != nil {
		t.Fatal(err)
	}
	set, err := approval.Parse(b, h.approvalsPath())
	if err != nil {
		t.Fatal(err)
	}
	main := strings.TrimSpace(h.sb.Git(h.cwd, "rev-parse", "--show-toplevel"))
	if set.Len() != 1 || !set.Approved(filepath.FromSlash(main), "post-create", "npm ci") {
		t.Errorf("approvals.toml:\n%s\nwant one approval of npm ci for %s", b, main)
	}
	for _, want := range []string{"[[approval]]", "command = 'npm ci'", "sha256 = '" + approval.Hash("npm ci") + "'"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("approvals.toml does not contain %q:\n%s", want, b)
		}
	}
}

func TestHookApproveNextToWTConfig(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	dir := h.mkdir(h.sb.Path("x"))
	h.sb.Setenv("WT_CONFIG", filepath.Join(dir, "wt.toml"))
	h.repoHooks(h.cwd, hookLine("post-create", "npm ci"))
	h.approve(h.cwd)
	if !exists(filepath.Join(dir, "approvals.toml")) {
		t.Errorf("no approvals.toml next to WT_CONFIG in %s", dir)
	}
}

func TestHookHelp(t *testing.T) {
	h := newHarness(t)
	r := h.run("config", "--help")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "wt hook approve") {
		t.Errorf("wt config --help does not name wt hook approve:\n%s", r.stdout)
	}
	r = h.run("hook", "--help")
	r.mustCode(t, 0)
	for _, want := range []string{"list", "approve", "approvals.toml", "SHA-256"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("wt hook --help does not contain %q", want)
		}
	}
}

// --- 5.1: wt config path ---

// Scenario "Path of the approvals file".
func TestConfigPathApprovals(t *testing.T) {
	h := newHarness(t)
	dir := h.mkdir(h.sb.Path("x"))
	h.sb.Setenv("WT_CONFIG", filepath.Join(dir, "wt.toml"))
	want := filepath.Join(dir, "approvals.toml")
	r := h.run("config", "path")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "approvals: "+want+" (not found)") {
		t.Errorf("stdout = %q, want the approvals line for %s", r.stdout, want)
	}
	doc := decodeOne(t, h.run("config", "path", "--json").stdout)
	a, _ := doc["approvals"].(map[string]any)
	if a["path"] != want || a["exists"] != false {
		t.Errorf("approvals = %v, want path %s and exists false", doc["approvals"], want)
	}
}

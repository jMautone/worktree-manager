package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// cloneRepo creates <sandbox>/origin.git and its clone <sandbox>/repo, where
// origin/HEAD points to origin/main, and starts in the clone.
func cloneRepo(h *harness) (repo, origin string) {
	h.t.Helper()
	origin = h.sb.Path("origin.git")
	h.sb.InitRemote(origin)
	repo = h.sb.Path("repo")
	h.sb.Clone(origin, repo)
	h.cwd = repo
	return repo, origin
}

// branchHead is the commit of the local branch, or "" when it does not
// exist.
func (h *harness) branchHead(repo, branch string) string {
	h.t.Helper()
	return h.sb.Git(repo, "for-each-ref", "--format=%(objectname)", "refs/heads/"+branch)
}

// worktreeCount is how many worktrees git has registered for repo.
func (h *harness) worktreeCount(repo string) int {
	h.t.Helper()
	return strings.Count(h.sb.Git(repo, "worktree", "list", "--porcelain")+"\n", "worktree ")
}

// createdPath is the path in the "created worktree" line of stdout.
func createdPath(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(line, "created worktree "); ok {
			path, _, _ := strings.Cut(rest, " on new branch ")
			return path
		}
	}
	t.Fatalf("no created worktree line in\n%s", stdout)
	return ""
}

// mustSamePath checks that got, a path wt reported, is want: native,
// absolute and clean, and the same directory once symbolic links are
// resolved.
func mustSamePath(t *testing.T, got, want string) {
	t.Helper()
	if !filepath.IsAbs(got) || got != filepath.Clean(got) {
		t.Errorf("path %q is not a clean native absolute path", got)
	}
	if testutil.Comparable(t, got) != testutil.Comparable(t, want) {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// mustCreate runs wt create and checks that it created the worktree with
// branch checked out; it returns the worktree's path.
func (h *harness) mustCreate(branch string, args ...string) string {
	h.t.Helper()
	r := h.run(append([]string{"create"}, args...)...)
	r.mustCode(h.t, 0)
	path := createdPath(h.t, r.stdout)
	if got := h.sb.Git(path, "branch", "--show-current"); got != branch {
		h.t.Fatalf("branch in %s = %q, want %q", path, got, branch)
	}
	return path
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// --- 6.1: arguments and usage errors ---

func TestCreateUsageErrors(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"create"}, "<name>"},
		{[]string{"create", "a", "b"}, `unexpected argument "b"`},
		{[]string{"create", ""}, "empty"},
		{[]string{"create", "../x", "-b", "fix"}, "../x"},
		{[]string{"create", `a\..\x`}, `a\\..\\x`},
		{[]string{"create", "feat", "--cd", "--no-cd"}, "--no-cd"},
		{[]string{"create", "feat", "-x", ""}, "-x"},
		{[]string{"create", "feat", "-x", "  "}, "-x"},
		{[]string{"create", "feat", "---x", "true"}, "---x"},
		{[]string{"create", "feat", "--x", "true"}, "--x"},
		{[]string{"create", "feat", "--exec", "true"}, "--exec"},
	} {
		r := h.run(tc.args...)
		if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, tc.want) {
			t.Errorf("wt %q: exit %d, stdout %q, stderr %q; want 2 and stderr naming %q", tc.args, r.code, r.stdout, r.stderr, tc.want)
		}
	}
	for _, b := range []string{"feat", "fix", "x"} {
		if h.branchHead(repo, b) != "" {
			t.Errorf("branch %s was created", b)
		}
	}
	if n := h.worktreeCount(repo); n != 1 || exists(h.sb.Path("x")) || exists(h.sb.Path("repo.worktrees")) {
		t.Errorf("something was created: %d worktrees", n)
	}
}

func TestCreateCommandWithJSON(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	r := h.run("create", "feat", "-x", "claude", "--json")
	r.mustCode(t, 2)
	if doc := decodeOne(t, r.stderr); doc["schema"] != "wt.error.v1" || doc["code"] != float64(2) || r.stdout != "" {
		t.Errorf("stdout %q, error %v", r.stdout, doc)
	}
	if h.branchHead(repo, "feat") != "" || h.worktreeCount(repo) != 1 {
		t.Error("something was created")
	}
}

func TestCreateOutsideARepository(t *testing.T) {
	h := newHarness(t)
	r := h.run("create", "feat")
	r.mustCode(t, 3)
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
}

func TestCreateHelp(t *testing.T) {
	r := newHarness(t).run("create", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{
		"wt create <name> [flags]", "-x cmd ", "-b, --branch branch", "--base ref", "--cd", "--no-cd",
		"branch_prefix", "default_base", "fetch_before_create", "worktree_path", "{name|sanitize}", "create_cd",
		"sh -c", "cmd.exe", "PowerShell", "single quotes",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not mention %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "---x") || strings.Contains(r.stdout, "-x, ") {
		t.Errorf("help shows a long form of -x:\n%s", r.stdout)
	}
	// The help lines of the other commands keep -C as it was.
	if r := newHarness(t).run("list", "-h"); !strings.Contains(r.stdout, "  -C dir           run as if") {
		t.Errorf("wt list -h:\n%s", r.stdout)
	}
}

// --- 6.2: branch and path ---

func TestCreateBranchFromTheName(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	path := h.mustCreate("feature/abc1", "feature/abc1")
	// path-templates, "Branch with a slash".
	mustSamePath(t, path, h.sb.Path("repo.worktrees", "feature-abc1"))
}

func TestCreateBranchPrefix(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.userConfig("branch_prefix = \"jm/\"\n")

	path := h.mustCreate("jm/auth", "auth")
	if filepath.Base(path) != "auth" {
		t.Errorf("path %s, want the last component auth", path)
	}
	// An explicit branch ignores the prefix.
	path = h.mustCreate("feature/api-v2", "api", "-b", "feature/api-v2")
	if filepath.Base(path) != "api" {
		t.Errorf("path %s, want the last component api", path)
	}
}

func TestCreateInvalidBranch(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)

	r := h.run("create", "a b")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, `invalid branch name "a b"`) || strings.Contains(r.stderr, "hint:") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if exists(h.sb.Path("repo.worktrees")) || h.worktreeCount(repo) != 1 {
		t.Error("something was created")
	}

	// -b "" is a branch, and not a valid one.
	h.run("create", "x", "-b", "").mustCode(t, 2)
	// @{-1} is valid for git, which expands it; wt does not.
	h.sb.Git(repo, "checkout", "-q", "-b", "other")
	h.sb.Git(repo, "checkout", "-q", "main")
	if r := h.run("create", "x", "-b", "@{-1}"); r.code != 2 || !strings.Contains(r.stderr, "@{-1}") {
		t.Errorf("-b @{-1}: exit %d, stderr %q", r.code, r.stderr)
	}

	h.userConfig("branch_prefix = \"jm/\"\n")
	r = h.run("create", "a b")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, `invalid branch name "jm/a b"`) || !strings.Contains(r.stderr, "hint: ") || !strings.Contains(r.stderr, `branch_prefix ("jm/")`) {
		t.Errorf("with a prefix: stderr = %q", r.stderr)
	}
	// With -b the prefix plays no part.
	if r := h.run("create", "x", "-b", "a b"); r.code != 2 || strings.Contains(r.stderr, "branch_prefix") {
		t.Errorf("-b 'a b': exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCreateNameThatIsNotABranch(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	path := h.mustCreate("task", "my task", "-b", "task")
	if filepath.Base(path) != "my task" {
		t.Errorf("path %s, want the last component 'my task'", path)
	}
}

func TestCreateDirectoryAlreadyThere(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	dir := h.sb.Path("repo.worktrees", "feat")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := h.run("create", "feat")
	r.mustCode(t, 5)
	if got, want := firstLine(r.stderr), "wt: path already exists: "+testutil.Comparable(t, dir); got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
	if h.branchHead(repo, "feat") != "" {
		t.Error("branch feat was created")
	}

	// A file and a dangling symbolic link exist too.
	h.sb.WriteFile(h.sb.Path("repo.worktrees", "file"), "")
	h.run("create", "file").mustCode(t, 5)
	if runtime.GOOS != "windows" {
		if err := os.Symlink(h.sb.Path("missing"), h.sb.Path("repo.worktrees", "link")); err != nil {
			t.Fatal(err)
		}
		h.run("create", "link").mustCode(t, 5)
	}
}

func TestCreateLetterCaseDiffers(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	if err := os.MkdirAll(h.sb.Path("repo.worktrees", "feat"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := h.run("create", "Feat")
	switch runtime.GOOS {
	case "darwin", "windows":
		r.mustCode(t, 5)
	default:
		r.mustCode(t, 0)
		mustSamePath(t, createdPath(t, r.stdout), h.sb.Path("repo.worktrees", "Feat"))
	}
}

func TestCreateStaleRegistration(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	stale := h.sb.Path("repo.worktrees", "feat")
	h.sb.AddWorktree(repo, stale, "feat")
	h.sb.MakePrunable(stale)

	r := h.run("create", "feat", "-b", "feat2")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, testutil.Comparable(t, stale)) || !strings.Contains(r.stderr, "hint: ") || !strings.Contains(r.stderr, "git worktree prune") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.branchHead(repo, "feat2") != "" {
		t.Error("branch feat2 was created")
	}
}

func TestCreateLocalBranchExists(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.Git(repo, "branch", "feat")

	r := h.run("create", "feat")
	r.mustCode(t, 5)
	if got := firstLine(r.stderr); got != `wt: branch "feat" already exists` {
		t.Errorf("first line = %q", got)
	}
	if exists(h.sb.Path("repo.worktrees")) || h.worktreeCount(repo) != 1 {
		t.Error("something was created")
	}
}

func TestCreateBranchCheckedOutInAWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.AddWorktree(repo, h.sb.Path("elsewhere", "feat"), "feat")

	r := h.run("create", "feat")
	r.mustCode(t, 5)
	if !strings.Contains(r.stderr, "\nhint: ") || !strings.Contains(r.stderr, "wt cd feat") {
		t.Errorf("stderr = %q, want a hint naming wt cd feat", r.stderr)
	}
}

// Scenarios of path-templates.
func TestCreatePaths(t *testing.T) {
	for _, tc := range []struct {
		name, template string
		args           []string
		want           func(h *harness) string
	}{
		{"default template", "", []string{"feat"},
			func(h *harness) string { return h.sb.Path("repo.worktrees", "feat") }},
		// -b: on Windows git cannot create the branch nul, whose ref is
		// a file named nul; the scenario is about the directory.
		{"reserved name on every OS", "", []string{"nul", "-b", "reserved"},
			func(h *harness) string { return h.sb.Path("repo.worktrees", "nul-") }},
		{"filters applied in order", "{repo_parent}/{branch|sanitize|lower}", []string{"x", "-b", "Feature/ABC"},
			func(h *harness) string { return h.sb.Path("feature-abc") }},
		{"whitespace inside an expression", "{repo_parent}/{ name | sanitize }", []string{"a/b"},
			func(h *harness) string { return h.sb.Path("a-b") }},
		{"lower-case directory", "{repo_parent}/{name|lower}", []string{"Feat"},
			func(h *harness) string { return h.sb.Path("feat") }},
		{"home directory", "~/wt/{repo}/{name|sanitize}", []string{"feat"},
			func(h *harness) string { return filepath.Join(h.sb.Getenv("HOME"), "wt", "repo", "feat") }},
		{"relative to the repository", ".worktrees/{name|sanitize}", []string{"feat"},
			func(h *harness) string { return h.sb.Path("repo", ".worktrees", "feat") }},
		{"parent components", "{repo_path}/../{repo}-{name|sanitize}", []string{"feat"},
			func(h *harness) string { return h.sb.Path("repo-feat") }},
		{"missing parent directories", "{repo_parent}/wt/{repo}/{name|sanitize}", []string{"feat"},
			func(h *harness) string { return h.sb.Path("wt", "repo", "feat") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			cloneRepo(h)
			if tc.template != "" {
				h.userConfig("worktree_path = '" + tc.template + "'\n")
			}
			r := h.run(append([]string{"create"}, tc.args...)...)
			r.mustCode(t, 0)
			mustSamePath(t, createdPath(t, r.stdout), tc.want(h))
			if !exists(tc.want(h)) {
				t.Errorf("%s does not exist", tc.want(h))
			}
		})
	}
}

func TestCreateFromALinkedWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	linked := h.sb.Path("repo.worktrees", "feat")
	h.sb.AddWorktree(repo, linked, "feat")
	h.cwd = filepath.Join(linked, "sub")
	if err := os.Mkdir(h.cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	path := h.mustCreate("x", "x")
	mustSamePath(t, path, h.sb.Path("repo.worktrees", "x"))
}

func TestCreateFromABareRepository(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("proj.git")
	h.sb.InitBare(bare)
	h.cwd = bare

	// git-worktrees, "Bare repository": no origin/HEAD, HEAD points to main.
	r := h.run("create", "feat", "--dry-run")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "on new branch feat from main") {
		t.Errorf("--dry-run: stdout = %q", r.stdout)
	}

	path := h.mustCreate("feat", "feat")
	mustSamePath(t, path, h.sb.Path("proj.worktrees", "feat"))
}

func TestCreateValidPathOnlyCheckedOnWindows(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.userConfig("worktree_path = '{repo_parent}/{name}'\n")
	r := h.run("create", "nul")
	if runtime.GOOS != "windows" {
		// "Same template on macOS".
		r.mustCode(t, 0)
		mustSamePath(t, createdPath(t, r.stdout), h.sb.Path("nul"))
		return
	}
	// "Reserved name without sanitize (Windows)".
	r.mustCode(t, 2)
	want := h.sb.Path("nul")
	if !strings.Contains(r.stderr, filepath.Base(filepath.Dir(want))) || !strings.Contains(r.stderr, "sanitize") {
		t.Errorf("stderr = %q, want the path and sanitize", r.stderr)
	}
	// Lstat cannot tell: on Windows, nul exists in every directory.
	entries, err := os.ReadDir(filepath.Dir(want))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), "nul") {
			t.Errorf("%s was created", want)
		}
	}
	if h.branchHead(repo, "nul") != "" || h.worktreeCount(repo) != 1 {
		t.Error("the branch or the worktree was created")
	}
}

// --- 6.3: base and fetch ---

func TestCreateBase(t *testing.T) {
	t.Run("default branch of a clone", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.sb.Git(repo, "checkout", "-q", "-b", "elsewhere")
		h.sb.Commit(repo, "not on main")
		r := h.run("create", "feat")
		r.mustCode(t, 0)
		if !strings.Contains(r.stdout, " from origin/main\n") {
			t.Errorf("stdout = %q", r.stdout)
		}
		if got, want := h.branchHead(repo, "feat"), h.sb.Git(repo, "rev-parse", "origin/main"); got != want {
			t.Errorf("feat at %s, want origin/main %s", got, want)
		}
	})
	t.Run("base from the repository configuration", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.sb.Git(repo, "checkout", "-q", "-b", "develop")
		develop := h.sb.Commit(repo, "develop")
		h.sb.Git(repo, "checkout", "-q", "main")
		h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \"develop\"\n")
		h.mustCreate("feat", "feat")
		if got := h.branchHead(repo, "feat"); got != develop {
			t.Errorf("feat at %s, want develop %s", got, develop)
		}
	})
	t.Run("explicit base", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		tagged := h.sb.Commit(repo, "tagged")
		h.sb.Git(repo, "tag", "-a", "-m", "v1.0", "v1.0")
		h.sb.Commit(repo, "after the tag")
		h.mustCreate("hotfix", "hotfix", "--base", "v1.0")
		if got := h.branchHead(repo, "hotfix"); got != tagged {
			t.Errorf("hotfix at %s, want the tagged commit %s", got, tagged)
		}
		// A commit id is a base too.
		h.mustCreate("fix", "fix", "--base", tagged[:10])
		if got := h.branchHead(repo, "fix"); got != tagged {
			t.Errorf("fix at %s, want %s", got, tagged)
		}
	})
	t.Run("base not found", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		for _, base := range []string{"nope", "-x", "--all"} {
			r := h.run("create", "feat", "--base", base)
			r.mustCode(t, 3)
			if got := firstLine(r.stderr); got != "wt: base not found: "+base {
				t.Errorf("--base %s: first line %q", base, got)
			}
		}
		if h.branchHead(repo, "feat") != "" || exists(h.sb.Path("repo.worktrees")) {
			t.Error("something was created")
		}
	})
	t.Run("no default branch", func(t *testing.T) {
		h := newHarness(t)
		h.cwd = h.repo()
		h.sb.Git(h.cwd, "checkout", "-q", "--detach")
		for _, args := range [][]string{{"create", "feat"}, {"create", "feat", "--dry-run"}} {
			r := h.run(args...)
			r.mustCode(t, 3)
			if got := firstLine(r.stderr); got != "wt: the repository has no default branch" {
				t.Errorf("first line = %q", got)
			}
			if !strings.Contains(r.stderr, "hint: pass --base") || !strings.Contains(r.stderr, "hint: or set default_base") {
				t.Errorf("stderr = %q, want hints naming --base and default_base", r.stderr)
			}
		}
	})
	t.Run("no upstream", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.mustCreate("feat", "feat", "--base", "origin/main")
		if got := h.sb.Git(repo, "for-each-ref", "--format=%(upstream)", "refs/heads/feat"); got != "" {
			t.Errorf("feat has the upstream %q", got)
		}
	})
}

// git-worktrees, "Default branch".
func TestCreateDefaultBranch(t *testing.T) {
	t.Run("clone", func(t *testing.T) {
		h := newHarness(t)
		cloneRepo(h)
		if r := h.run("create", "feat", "--dry-run"); !strings.Contains(r.stdout, "on new branch feat from origin/main") {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
	t.Run("no remote", func(t *testing.T) {
		h := newHarness(t)
		h.cwd = h.repo()
		h.sb.Git(h.cwd, "branch", "-m", "trunk")
		if r := h.run("create", "feat", "--dry-run"); !strings.Contains(r.stdout, "on new branch feat from trunk") {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
}

func TestCreateFetchesBeforeCreating(t *testing.T) {
	t.Run("remote advanced since the last fetch", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		pushed := h.sb.PushFromAnotherClone(origin, "main")
		h.mustCreate("feat", "feat")
		if got := h.branchHead(repo, "feat"); got != pushed {
			t.Errorf("feat at %s, want the pushed commit %s", got, pushed)
		}
	})
	t.Run("base that only exists on the remote", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		pushed := h.sb.PushFromAnotherClone(origin, "release")
		h.mustCreate("fix", "fix", "--base", "origin/release")
		if got := h.branchHead(repo, "fix"); got != pushed {
			t.Errorf("fix at %s, want %s", got, pushed)
		}
	})
	t.Run("unreachable remote", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		local := h.sb.Git(repo, "rev-parse", "origin/main")
		h.sb.BreakRemote(repo, "origin")
		r := h.run("create", "feat", "--base", "origin/main", "--no-cd")
		r.mustCode(t, 0)
		if !strings.Contains(r.stderr, "wt: warning: cannot fetch origin (") || !strings.Contains(r.stderr, "using the references already here") {
			t.Errorf("stderr = %q, want a warning naming origin", r.stderr)
		}
		if got := h.branchHead(repo, "feat"); got != local {
			t.Errorf("feat at %s, want the local origin/main %s", got, local)
		}
	})
	t.Run("local base", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		before := h.sb.Git(repo, "rev-parse", "origin/main")
		h.sb.PushFromAnotherClone(origin, "main")
		h.mustCreate("feat", "feat", "--base", "main")
		if got := h.sb.Git(repo, "rev-parse", "origin/main"); got != before {
			t.Error("origin was fetched for a local base")
		}
	})
	t.Run("fetching disabled", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		before := h.sb.Git(repo, "rev-parse", "origin/main")
		h.sb.PushFromAnotherClone(origin, "main")
		h.sb.Setenv("WT_FETCH_BEFORE_CREATE", "false")
		h.mustCreate("feat", "feat", "--base", "origin/main")
		if got := h.sb.Git(repo, "rev-parse", "origin/main"); got != before {
			t.Error("origin was fetched with fetching disabled")
		}
		if got := h.branchHead(repo, "feat"); got != before {
			t.Errorf("feat at %s, want the local origin/main %s", got, before)
		}
	})
	t.Run("longest remote wins", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		// git remote add refuses a name nested in another's; the
		// configuration does not.
		h.sb.Git(repo, "config", "remote.origin/fork.url", origin)
		h.sb.Git(repo, "config", "remote.origin/fork.fetch", "+refs/heads/*:refs/remotes/origin/fork/*")
		h.sb.BreakRemote(repo, "origin")
		r := h.run("create", "feat", "--base", "origin/fork/main", "--no-cd")
		r.mustCode(t, 0)
		if strings.Contains(r.stderr, "warning") {
			t.Errorf("fetched origin instead of origin/fork: %q", r.stderr)
		}
	})
}

func TestCreateBranchExistsOnARemote(t *testing.T) {
	h := newHarness(t)
	repo, origin := cloneRepo(h)
	h.sb.PushFromAnotherClone(origin, "feat")
	h.sb.Git(repo, "fetch", "-q", "origin")

	r := h.run("create", "feat")
	r.mustCode(t, 5)
	if got := firstLine(r.stderr); got != `wt: branch "feat" already exists on remote origin` {
		t.Errorf("first line = %q", got)
	}
	if !strings.Contains(r.stderr, "hint: choose another branch with -b") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.branchHead(repo, "feat") != "" {
		t.Error("a local branch feat was created")
	}
}

func TestCreateBranchPushedAfterTheLastFetch(t *testing.T) {
	h := newHarness(t)
	repo, origin := cloneRepo(h)
	h.sb.PushFromAnotherClone(origin, "feat")

	h.run("create", "feat", "--base", "origin/main").mustCode(t, 5)
	if h.branchHead(repo, "feat") != "" || exists(h.sb.Path("repo.worktrees")) || h.worktreeCount(repo) != 1 {
		t.Error("something was created")
	}
}

// --- 6.4: creating, JSON and dry run ---

func TestCreateFromTheMainWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.cwd = filepath.Join(repo, "sub")
	if err := os.Mkdir(h.cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	r := h.run("create", "feat")
	r.mustCode(t, 0)
	want := "created worktree " + testutil.Comparable(t, h.sb.Path("repo.worktrees", "feat")) + " on new branch feat from origin/main\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}

	doc := decodeOne(t, h.run("list", "--json").stdout)
	found := false
	for _, w := range doc["worktrees"].([]any) {
		if w := w.(map[string]any); w["name"] == "feat" && w["branch"] == "feat" {
			found = true
		}
	}
	if !found {
		t.Errorf("wt list does not show feat: %v", doc)
	}
}

func TestCreateGitFailsWhileCreating(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.WriteFile(h.sb.Path("blocker"), "")
	h.userConfig("worktree_path = '{repo_parent}/blocker/{name|sanitize}'\n")

	r := h.run("create", "feat")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "git worktree add") || !strings.Contains(r.stderr, "fatal:") {
		t.Errorf("stderr = %q, want git's message", r.stderr)
	}
	if h.branchHead(repo, "feat") != "" || h.worktreeCount(repo) != 1 {
		t.Error("the branch or the worktree was left behind")
	}
}

func TestCreateJSON(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)

	preview := h.run("create", "feat", "--json", "--dry-run")
	preview.mustCode(t, 0)
	if h.branchHead(repo, "feat") != "" {
		t.Fatal("--dry-run created the branch")
	}

	r := h.run("create", "feat", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.create.v1" || doc["name"] != "feat" || doc["branch"] != "feat" || doc["base"] != "origin/main" {
		t.Errorf("document = %v", doc)
	}
	if head := h.sb.Git(repo, "rev-parse", "origin/main"); doc["head"] != head {
		t.Errorf("head = %v, want %s", doc["head"], head)
	}
	path, _ := doc["path"].(string)
	mustSamePath(t, path, h.sb.Path("repo.worktrees", "feat"))
	if preview.stdout != r.stdout {
		t.Errorf("--dry-run --json printed\n%s\nand --json\n%s", preview.stdout, r.stdout)
	}
	// Warnings are not written with --json.
	if r.stderr != "" {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestCreateDryRun(t *testing.T) {
	t.Run("errors under dry run", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		h.sb.Git(repo, "branch", "feat")
		h.run("create", "feat", "--dry-run").mustCode(t, 5)
	})
	t.Run("nothing fetched under dry run", func(t *testing.T) {
		h := newHarness(t)
		repo, origin := cloneRepo(h)
		before := h.sb.Git(repo, "rev-parse", "origin/main")
		h.sb.PushFromAnotherClone(origin, "main")
		r := h.run("create", "feat", "--dry-run", "--no-cd")
		r.mustCode(t, 0)
		path := testutil.Comparable(t, h.sb.Path("repo.worktrees", "feat"))
		want := "would fetch origin\nwould create worktree " + path + " on new branch feat from origin/main\n"
		if r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
		if got := h.sb.Git(repo, "rev-parse", "origin/main"); got != before {
			t.Error("origin was fetched")
		}
	})
	t.Run("a base pushed after the last fetch is not seen", func(t *testing.T) {
		h := newHarness(t)
		_, origin := cloneRepo(h)
		h.sb.PushFromAnotherClone(origin, "release")
		h.run("create", "fix", "--base", "origin/release", "--dry-run").mustCode(t, 3)
	})
}

func TestCreateInAnotherRepository(t *testing.T) {
	h := newHarness(t)
	a := h.sb.Path("a")
	h.sb.InitRepo(a)
	b := h.sb.Path("b")
	h.sb.InitRepo(b)
	h.cwd = a
	directive := h.activate()

	r := h.run("-C", b, "create", "feat")
	r.mustCode(t, 0)
	path := createdPath(t, r.stdout)
	mustSamePath(t, path, h.sb.Path("b.worktrees", "feat"))
	if h.branchHead(b, "feat") == "" || h.branchHead(a, "feat") != "" {
		t.Error("the branch was not created in b only")
	}
	mustDirective(t, directive, path)
}

// --- 6.5: moving the shell ---

func TestCreateMovesTheShell(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	directive := h.activate()

	r := h.run("create", "feat")
	r.mustCode(t, 0)
	mustDirective(t, directive, createdPath(t, r.stdout))
	if r.stderr != "" {
		t.Errorf("stderr = %q", r.stderr)
	}

	// The function creates an empty directive file for every run.
	directive = h.activate()
	r = h.run("create", "stay", "--no-cd")
	r.mustCode(t, 0)
	if !exists(createdPath(t, r.stdout)) || readDirective(t, directive) != "" {
		t.Errorf("--no-cd: directive %q", readDirective(t, directive))
	}

	h.userConfig("create_cd = false\n")
	directive = h.activate()
	h.run("create", "off").mustCode(t, 0)
	if got := readDirective(t, directive); got != "" {
		t.Errorf("create_cd = false: directive %q", got)
	}
	directive = h.activate()
	r = h.run("create", "fix", "--cd")
	r.mustCode(t, 0)
	mustDirective(t, directive, createdPath(t, r.stdout))
}

func TestCreateWithoutTheFunction(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	r := h.run("create", "feat")
	r.mustCode(t, 0)
	if !strings.Contains(r.stderr, "wt: warning: shell integration is not active") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.branchHead(repo, "feat") == "" {
		t.Error("the worktree was not created")
	}
	// Not moving the shell needs no function.
	if r := h.run("create", "fix", "--no-cd"); r.code != 0 || r.stderr != "" {
		t.Errorf("--no-cd: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCreateDirectiveCannotBeWritten(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.Setenv("WT_DIRECTIVE_CD_FILE", h.sb.Path("missing", "directive"))
	r := h.run("create", "feat", "-x", "touch ran")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "directive") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if h.worktreeCount(repo) != 2 || !exists(h.sb.Path("repo.worktrees", "feat")) {
		t.Error("the worktree did not remain created")
	}
	if exists(h.sb.Path("repo.worktrees", "feat", "ran")) {
		t.Error("-x ran")
	}
	if exists(h.sb.Path("missing")) {
		t.Error("the directive file was created")
	}
}

func TestCreateDryRunChangesDirectoryOnlyWithTheFunction(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.sb.Setenv("WT_FETCH_BEFORE_CREATE", "0")
	r := h.run("create", "feat", "--dry-run")
	r.mustCode(t, 0)
	if strings.Contains(r.stdout, "would change directory") {
		t.Errorf("without the function: %q", r.stdout)
	}
	directive := h.activate()
	r = h.run("create", "feat", "--dry-run")
	if !strings.Contains(r.stdout, "\nwould change directory to ") || readDirective(t, directive) != "" {
		t.Errorf("with the function: %q", r.stdout)
	}
	if r := h.run("create", "feat", "--dry-run", "--no-cd"); strings.Contains(r.stdout, "would change directory") {
		t.Errorf("--no-cd: %q", r.stdout)
	}
}

// --- 6.6: -x ---

func TestCreateRunsACommand(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	r := h.run("create", "feat", "-x", "git branch --show-current", "--no-cd")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "\nfeat\n") {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestCreateCommandExitCode(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.activate()
	exit3, exit4, ops, want := "exit 3", "exit 4", "echo a && echo b", "a\nb\n"
	if runtime.GOOS == "windows" {
		exit3, exit4, ops, want = "exit /b 3", "exit /b 4", "echo a& echo b", "a\r\nb\r\n"
	}

	r := h.run("create", "three", "-x", exit3)
	r.mustCode(t, 3)
	if h.branchHead(repo, "three") == "" || r.stderr != "" {
		t.Errorf("the worktree was not created, or something was printed: %q", r.stderr)
	}
	// cli-contract, "Exit code of a user command": 4 is not an ambiguous name.
	h.run("create", "four", "-x", exit4).mustCode(t, 4)

	r = h.run("create", "ops", "-x", ops)
	r.mustCode(t, 0)
	if !strings.HasSuffix(r.stdout, "\n"+want) {
		t.Errorf("stdout = %q", r.stdout)
	}

	if runtime.GOOS != "windows" {
		h.run("create", "sig", "-x", "kill -INT $$").mustCode(t, 130)
	}
}

func TestCreateCommandNotRunWhenCreationFails(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.Git(repo, "branch", "feat")
	h.run("create", "feat", "-x", "echo ran > "+h.sb.Path("ran")).mustCode(t, 5)
	if exists(h.sb.Path("ran")) {
		t.Error("the command ran")
	}
}

func TestCreateCommandDoesNotSeeTheProtocolVariables(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	h.activate()
	h.sb.Setenv("WT_PREVIOUS_DIR", h.sb.Root)
	h.sb.Setenv("WT_SEEN", "yes")
	record := h.sb.Path("env.txt")
	cmd := "env > " + record
	if runtime.GOOS == "windows" {
		cmd = "set > " + record
	}
	h.run("create", "feat", "-x", cmd).mustCode(t, 0)
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	env := strings.ToUpper(string(b))
	if strings.Contains(env, "WT_DIRECTIVE_CD_FILE") || strings.Contains(env, "WT_PREVIOUS_DIR") || !strings.Contains(env, "WT_SEEN=YES") {
		t.Errorf("environment of -x:\n%s", b)
	}
}

func TestCreateCommandThatDoesNotStart(t *testing.T) {
	restore := cli.FailToStart(errors.New("no interpreter"))
	defer restore()
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	r := h.run("create", "feat", "-x", "claude", "--no-cd")
	r.mustCode(t, 1)
	if got := firstLine(r.stderr); got != "wt: cannot run claude: no interpreter" {
		t.Errorf("first line = %q", got)
	}
	if h.branchHead(repo, "feat") == "" {
		t.Error("the worktree was not created")
	}
}

func TestCreatePreview(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	directive := h.activate()
	r := h.run("create", "feat", "--dry-run", "-x", "claude")
	r.mustCode(t, 0)
	path := testutil.Comparable(t, h.sb.Path("repo.worktrees", "feat"))
	want := strings.Join([]string{
		"would fetch origin",
		"would create worktree " + path + " on new branch feat from origin/main",
		"would change directory to " + path,
		"would run claude",
	}, "\n") + "\n"
	if r.stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, want)
	}
	if h.branchHead(repo, "feat") != "" || exists(path) || h.worktreeCount(repo) != 1 || readDirective(t, directive) != "" {
		t.Error("--dry-run changed something")
	}
}

// --- 6.7: completions ---

func TestCreateCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	h.sb.Git(repo, "branch", "develop")
	h.sb.Git(repo, "tag", "v1")
	h.userConfig("colour = \"blue\"\n")

	got, directive, r := h.complete("create", "feat", "--base", "")
	if want := []string{"develop", "main", "origin/main"}; !slices.Equal(got, want) || directive != ":4" || r.stderr != "" {
		t.Errorf("--base: offered %q (directive %s, stderr %q), want %q", got, directive, r.stderr, want)
	}
	if got, _, _ := h.complete("create", "feat", "--base", "origin/"); !slices.Equal(got, []string{"origin/main"}) {
		t.Errorf("--base origin/: offered %q", got)
	}
	for _, args := range [][]string{{"create", ""}, {"create", "feat", "-b", ""}, {"create", "feat", "-x", ""}} {
		if got, directive, _ := h.complete(args...); len(got) != 0 || directive != ":4" {
			t.Errorf("%q: offered %q, directive %s", args, got, directive)
		}
	}

	h.cwd = h.sb.Path("outside")
	if got, _, r := h.complete("create", "feat", "--base", ""); len(got) != 0 || r.stdout != ":4\n" || r.stderr != "" {
		t.Errorf("outside: stdout %q, stderr %q", r.stdout, r.stderr)
	}
}

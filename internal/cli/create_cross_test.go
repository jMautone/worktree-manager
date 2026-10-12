package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// createWorkspace creates under <sandbox>/root api, a clone of
// <sandbox>/origin.git whose default branch is origin/main, and web, a
// repository without a remote; sets repos_root to the root; and starts
// outside any repository.
func createWorkspace(h *harness) (root, api, web string) {
	h.t.Helper()
	root = h.mkdir(h.sb.Path("root"))
	origin := h.sb.Path("origin.git")
	h.sb.InitRemote(origin)
	api, web = filepath.Join(root, "api"), filepath.Join(root, "web")
	h.sb.Clone(origin, api)
	h.sb.InitRepo(web)
	h.roots(0, root)
	return root, api, web
}

// Scenarios "Repository without a name" and "Name without a repository".
func TestCreateQualifiedUsageErrors(t *testing.T) {
	h := newHarness(t)
	_, api, _ := createWorkspace(h)
	for _, tc := range []struct {
		target, want string
	}{
		{"api:", "the name may not be empty"},
		{":feat", `":feat"`},
		{":", `":"`},
		{"api:../x", `"../x"`},
	} {
		r := h.run("create", tc.target)
		r.mustCode(t, 2)
		if !strings.Contains(firstLine(r.stderr), tc.want) {
			t.Errorf("wt create %s: first line %q, want it to contain %q", tc.target, firstLine(r.stderr), tc.want)
		}
	}
	if n := h.worktreeCount(api); n != 1 {
		t.Errorf("api has %d worktrees, want 1", n)
	}
}

// Scenarios of "Repository" in another repository.
func TestCreateQualifiedInAnotherRepository(t *testing.T) {
	t.Run("from outside any repository", func(t *testing.T) {
		h := newHarness(t)
		root, api, _ := createWorkspace(h)
		d := h.activate()
		r := h.run("create", "api:feat")
		r.mustCode(t, 0)
		want := filepath.Join(root, "api.worktrees", "feat")
		mustSamePath(t, createdPath(t, r.stdout), want)
		if !strings.Contains(r.stdout, " on new branch feat from origin/main\n") {
			t.Errorf("stdout = %q", r.stdout)
		}
		mustDirective(t, d, want)
		if got := h.sb.Git(want, "branch", "--show-current"); got != "feat" || h.branchHead(api, "feat") == "" {
			t.Errorf("branch in the new worktree = %q; want feat, a branch of api", got)
		}
	})
	t.Run("from inside a repository", func(t *testing.T) {
		h := newHarness(t)
		_, api, web := createWorkspace(h)
		h.cwd = web
		h.run("create", "api:feat", "--no-cd").mustCode(t, 0)
		if h.worktreeCount(api) != 2 || h.branchHead(api, "feat") == "" {
			t.Error("the worktree does not belong to api")
		}
		if h.worktreeCount(web) != 1 || h.branchHead(web, "feat") != "" {
			t.Error("web has a new worktree or branch")
		}
	})
	t.Run("command in another repository", func(t *testing.T) {
		h := newHarness(t)
		_, api, _ := createWorkspace(h)
		r := h.run("create", "api:feat", "--no-cd", "-x", "git branch --show-current")
		r.mustCode(t, 0)
		if !strings.Contains(r.stdout, "\nfeat\n") || h.worktreeCount(api) != 2 {
			t.Errorf("stdout = %q, worktrees of api %d", r.stdout, h.worktreeCount(api))
		}
	})
	t.Run("repository by prefix", func(t *testing.T) {
		h := newHarness(t)
		root, _, _ := createWorkspace(h)
		trend := filepath.Join(root, "TrendFisher")
		h.sb.InitRepo(trend)
		h.run("create", "trend:feat", "--no-cd").mustCode(t, 0)
		if h.branchHead(trend, "feat") == "" {
			t.Error("the worktree does not belong to TrendFisher")
		}
	})
	t.Run("current repository, qualified", func(t *testing.T) {
		h := newHarness(t)
		_, api, _ := createWorkspace(h)
		h.cwd = api
		h.sb.WriteFile(filepath.Join(api, ".wt.toml"), "branch_prefix = \"me/\"\n")
		path := h.mustCreate("me/feat", "api:feat", "--no-cd")
		mustSamePath(t, path, filepath.Join(filepath.Dir(api), "api.worktrees", "feat"))
	})
}

func TestCreateQualifiedErrors(t *testing.T) {
	h := newHarness(t)
	_, api, web := createWorkspace(h)

	r := h.run("create", "nope:feat")
	r.mustCode(t, 3)
	if got := firstLine(r.stderr); got != `wt: no repository named "nope"` {
		t.Errorf("first line = %q", got)
	}

	// Outside a repository, with roots, the hint names the qualified form.
	r = h.run("create", "feat")
	r.mustCode(t, 3)
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
	mustHint(t, r, "<repo>:feat")

	h.userConfig("")
	r = h.run("create", "feat")
	r.mustCode(t, 3)
	if strings.Contains(r.stderr, "hint:") {
		t.Errorf("without roots: stderr = %q, want no hint", r.stderr)
	}
	r = h.run("create", "api:feat")
	r.mustCode(t, 1)
	if got := firstLine(r.stderr); got != "wt: no repository roots configured" {
		t.Errorf("first line = %q", got)
	}
	for _, repo := range []string{api, web} {
		if h.worktreeCount(repo) != 1 {
			t.Errorf("%s has a new worktree", repo)
		}
	}
}

func TestCreateInAnotherRepositoryJSONAndDryRun(t *testing.T) {
	h := newHarness(t)
	root, api, _ := createWorkspace(h)
	want := filepath.Join(root, "api.worktrees", "feat")

	r := h.run("create", "api:feat", "--dry-run")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "would fetch origin\n") || !strings.Contains(r.stdout, "would create worktree ") ||
		!strings.Contains(r.stdout, filepath.Join("api.worktrees", "feat")+" on new branch feat from origin/main\n") {
		t.Errorf("--dry-run: stdout = %q", r.stdout)
	}
	if h.worktreeCount(api) != 1 {
		t.Error("--dry-run created the worktree")
	}

	r = h.run("create", "api:feat", "--json", "--no-cd")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	p, _ := doc["path"].(string)
	if doc["schema"] != "wt.create.v1" || doc["name"] != "feat" || doc["branch"] != "feat" {
		t.Errorf("document = %v", doc)
	}
	mustSamePath(t, p, want)
}

// Scenarios "File of the other repository" and "Other repository without a
// file" of config-layers.
func TestCreateUsesTheOtherRepositoryLayer(t *testing.T) {
	h := newHarness(t)
	_, api, web := createWorkspace(h)
	h.cwd = web
	h.sb.Git(api, "branch", "develop")

	h.sb.WriteFile(filepath.Join(web, ".wt.toml"), "default_base = \"trunk\"\nbranch_prefix = \"web/\"\n")
	r := h.run("create", "api:feat", "--dry-run")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, " on new branch feat from origin/main\n") {
		t.Errorf("api without a file: stdout = %q", r.stdout)
	}

	h.sb.WriteFile(filepath.Join(api, ".wt.toml"), "default_base = \"develop\"\n")
	r = h.run("create", "api:feat", "--dry-run")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, " on new branch feat from develop\n") {
		t.Errorf("api with a file: stdout = %q", r.stdout)
	}

	// An invalid .wt.toml in the working directory does not matter.
	h.sb.WriteFile(filepath.Join(web, ".wt.toml"), "default_base = \n")
	h.run("create", "api:feat", "--dry-run").mustCode(t, 0)
	h.run("create", "feat", "--dry-run").mustCode(t, 1)
}

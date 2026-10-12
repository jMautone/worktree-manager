package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// Scenarios of "Target" of remove-worktree for worktrees of other
// repositories. In crossWorkspace, feat of api is clean and merged into
// main, api's default branch.
func TestRemoveWorktreeOfAnotherRepository(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cwd    func(root string) string
		target string
	}{
		{"from another repository", func(root string) string { return filepath.Join(root, "web") }, "feat"},
		{"from outside any repository", func(string) string { return "" }, "feat"},
		{"qualified", func(root string) string { return filepath.Join(root, "web") }, "api:feat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			root, feat := crossWorkspace(h)
			api := filepath.Join(root, "api")
			if cwd := tc.cwd(root); cwd != "" {
				h.cwd = cwd
			}
			d := h.activate()
			r := h.run("remove", tc.target)
			r.mustCode(t, 0)
			if want := "removed worktree " + testutil.Comparable(t, feat); firstLine(r.stdout) != want {
				t.Errorf("stdout = %q, want first line %q", r.stdout, want)
			}
			if !strings.Contains(r.stdout, "deleted branch feat") || r.stderr != "" {
				t.Errorf("stdout %q, stderr %q; want the branch deleted and no warning", r.stdout, r.stderr)
			}
			h.mustBeRemoved(api, feat)
			if h.branchHead(api, "feat") != "" {
				t.Error("the branch feat of api still exists")
			}
			if readDirective(t, d) != "" {
				t.Error("the shell was moved")
			}
		})
	}
}

func TestRemoveQualifiedLeavesTheLocalWorktree(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	web := filepath.Join(root, "web")
	webFeat := filepath.Join(root, "web.worktrees", "feat")
	h.sb.AddWorktree(web, webFeat, "feat")
	h.cwd = web

	dry := h.run("remove", "--dry-run", "api:feat")
	dry.mustCode(t, 0)
	if want := "would remove worktree " + testutil.Comparable(t, feat); firstLine(dry.stdout) != want {
		t.Errorf("--dry-run: stdout = %q, want %q", dry.stdout, want)
	}
	h.mustBeIntact(filepath.Join(root, "api"), feat, "feat")

	h.run("remove", "api:feat").mustCode(t, 0)
	h.mustBeRemoved(filepath.Join(root, "api"), feat)
	h.mustBeIntact(web, webFeat, "feat")
}

func TestRemoveTargetErrorsAcrossRepositories(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	h.sb.InitRepo(filepath.Join(root, "feature-flags"))

	t.Run("qualified repository", func(t *testing.T) {
		r := h.run("remove", "api:")
		r.mustCode(t, 2)
		if got := firstLine(r.stderr); got != "wt: cannot remove the main worktree" {
			t.Errorf("first line = %q", got)
		}
	})
	t.Run("a repository is not a target", func(t *testing.T) {
		h.cwd = web
		r := h.run("remove", "feature-flags")
		r.mustCode(t, 3)
		if got := firstLine(r.stderr); got != `wt: no worktree named "feature-flags"` {
			t.Errorf("first line = %q", got)
		}
		mustHint(t, r, "wt list --all-repos")
	})
	t.Run("same name in two repositories", func(t *testing.T) {
		webFeat := filepath.Join(root, "web.worktrees", "feat")
		h.sb.AddWorktree(web, webFeat, "web-feat")
		h.cwd = h.sb.Path("outside")
		r := h.run("remove", "feat")
		r.mustCode(t, 4)
		mustHint(t, r, "api:feat", testutil.Comparable(t, feat))
		mustHint(t, r, "web:feat", testutil.Comparable(t, webFeat))
		h.mustBeIntact(api, feat, "feat")
		h.mustBeIntact(web, webFeat, "web-feat")
	})
	t.Run("current repository first", func(t *testing.T) {
		// From web, feat is web's own worktree, whatever api has.
		h.cwd = web
		h.run("remove", "--dry-run", "feat").mustCode(t, 0)
		r := h.run("lock", "--dry-run", "feat")
		if want := "would lock worktree " + testutil.Comparable(t, filepath.Join(root, "web.worktrees", "feat")) + "\n"; r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
	})
}

// Scenarios "Directory gone" of cross-repo-resolution, and "Outside a
// repository" and "No such worktree" with roots.
func TestRemoveGoneWorktreeOfAnotherRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api := filepath.Join(root, "api")
	h.sb.MakePrunable(feat)

	r := h.run("remove", "feat")
	r.mustCode(t, 0)
	if h.isListed(api, feat) {
		t.Errorf("wt -C api list still shows feat")
	}

	r = h.run("remove", "nope")
	r.mustCode(t, 3)
	if got := firstLine(r.stderr); got != `wt: no worktree named "nope"` {
		t.Errorf("first line = %q", got)
	}
	mustHint(t, r, "wt list --all-repos")
}

func TestLockInAnotherRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api := filepath.Join(root, "api")
	h.cwd = filepath.Join(root, "web")

	r := h.run("lock", "api:feat", "review")
	r.mustCode(t, 0)
	if want := "locked worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if locked, reason := h.lockState(api, feat); !locked || reason != "review" {
		t.Errorf("locked %v, reason %v", locked, reason)
	}

	r = h.run("unlock", "feat")
	r.mustCode(t, 0)
	if locked, _ := h.lockState(api, feat); locked {
		t.Error("feat is still locked")
	}
}

// Scenarios "Merged into the other repository's base" and "Inspecting from
// another repository" of config-layers.
func TestRemoveUsesTheOtherRepositoryLayer(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	// feat has a commit that develop has and main does not.
	h.commitFile(feat, "f.txt", "feat\n")
	h.sb.Git(api, "branch", "develop", "feat")
	h.sb.WriteFile(filepath.Join(api, ".wt.toml"), "default_base = \"develop\"\n")
	// web's layer would name a base api does not have.
	h.sb.WriteFile(filepath.Join(web, ".wt.toml"), "default_base = \"trunk\"\n")
	h.cwd = web

	r := h.run("config", "get", "default_base")
	r.mustCode(t, 0)
	if r.stdout != "trunk\n" {
		t.Errorf("wt config get default_base = %q, want trunk", r.stdout)
	}

	r = h.run("remove", "api:feat")
	r.mustCode(t, 0)
	if h.branchHead(api, "feat") != "" || r.stderr != "" {
		t.Errorf("stdout %q, stderr %q; want the branch feat of api deleted", r.stdout, r.stderr)
	}
}

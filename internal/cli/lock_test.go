package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// lockState is the lock of the worktree at path as wt list --json reports
// it, run from repo.
func (h *harness) lockState(repo, path string) (locked bool, reason any) {
	h.t.Helper()
	saved := h.cwd
	h.cwd = repo
	defer func() { h.cwd = saved }()
	ws, _ := listed(h.t, h)
	w, ok := ws[testutil.Comparable(h.t, path)]
	if !ok {
		h.t.Fatalf("wt list does not show %s", path)
	}
	return w["locked"] == true, w["locked_reason"]
}

// --- 5.1: lock and unlock ---

func TestLock(t *testing.T) {
	t.Run("with a reason", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		r := h.run("lock", "feat", "on usb drive")
		r.mustCode(t, 0)
		if want := "locked worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want || r.stderr != "" {
			t.Errorf("stdout %q, stderr %q; want %q", r.stdout, r.stderr, want)
		}
		if locked, reason := h.lockState(repo, feat); !locked || reason != "on usb drive" {
			t.Errorf("locked %v, reason %v", locked, reason)
		}
	})
	t.Run("without a reason", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.run("lock", "feat").mustCode(t, 0)
		if locked, reason := h.lockState(repo, feat); !locked || reason != nil {
			t.Errorf("locked %v, reason %v", locked, reason)
		}
	})
	t.Run("with an empty reason", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.run("lock", "feat", "").mustCode(t, 0)
		if locked, reason := h.lockState(repo, feat); !locked || reason != nil {
			t.Errorf("locked %v, reason %v", locked, reason)
		}
	})
	t.Run("already locked", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.sb.LockWorktree(repo, feat, "review")
		r := h.run("lock", "feat", "other")
		r.mustCode(t, 5)
		if first := firstLine(r.stderr); first != `wt: worktree "feat" is already locked: review` {
			t.Errorf("first line %q", first)
		}
		if _, reason := h.lockState(repo, feat); reason != "review" {
			t.Errorf("reason changed to %v", reason)
		}
	})
	t.Run("directory gone", func(t *testing.T) {
		h := newHarness(t)
		_, feat := removeRepo(h)
		h.sb.MakePrunable(feat)
		h.run("lock", "feat").mustCode(t, 0)
		r := h.run("list")
		r.mustCode(t, 0)
		if state := findRow(t, parseTable(t, r.stdout), "feat").cols["STATE"]; !strings.Contains(state, "locked") || strings.Contains(state, "prunable") {
			t.Errorf("STATE %q, want locked and not prunable", state)
		}
	})
}

func TestLockErrors(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.LockWorktree(repo, feat, "review")
	free := h.sb.Path("repo.worktrees", "free")
	h.sb.AddWorktree(repo, free, "free")
	for _, tc := range []struct {
		args  []string
		code  int
		first string
	}{
		{[]string{"lock", "^"}, 2, "wt: the main worktree cannot be locked"},
		{[]string{"unlock", "^"}, 2, "wt: the main worktree cannot be unlocked"},
		{[]string{"lock"}, 2, `wt: missing argument <target> for "wt lock"`},
		{[]string{"lock", "free", "a", "b"}, 2, `wt: unexpected argument "b" for "wt lock"`},
		{[]string{"unlock"}, 2, `wt: missing argument <target> for "wt unlock"`},
		{[]string{"unlock", "free", "x"}, 2, `wt: unexpected argument "x" for "wt unlock"`},
		{[]string{"unlock", "free"}, 5, `wt: worktree "free" is not locked`},
		{[]string{"unlock", "free", "--dry-run"}, 5, `wt: worktree "free" is not locked`},
		{[]string{"lock", "feat", "--dry-run"}, 5, `wt: worktree "feat" is already locked: review`},
		{[]string{"lock", "nope"}, 3, `wt: no worktree named "nope"`},
	} {
		r := h.run(tc.args...)
		if r.code != tc.code || firstLine(r.stderr) != tc.first || r.stdout != "" {
			t.Errorf("wt %q: exit %d, stdout %q, stderr %q; want %d and %q", tc.args, r.code, r.stdout, r.stderr, tc.code, tc.first)
		}
	}
	if locked, _ := h.lockState(repo, free); locked {
		t.Error("free was locked")
	}
	h.cwd = h.sb.Path("outside")
	h.run("lock", "free").mustCode(t, 3)
	h.run("unlock", "free").mustCode(t, 3)
}

func TestUnlock(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.LockWorktree(repo, feat, "on usb drive")
	r := h.run("unlock", "feat")
	r.mustCode(t, 0)
	if want := "unlocked worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}
	if locked, _ := h.lockState(repo, feat); locked {
		t.Error("still locked")
	}
	h.run("remove", "feat").mustCode(t, 0)
}

func TestLockJSONAndDryRun(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)

	r := h.run("lock", "feat", "--dry-run")
	r.mustCode(t, 0)
	if want := "would lock worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}
	if locked, _ := h.lockState(repo, feat); locked {
		t.Fatal("--dry-run locked the worktree")
	}

	preview := h.run("lock", "feat", "review", "--json", "--dry-run")
	preview.mustCode(t, 0)
	r = h.run("lock", "feat", "review", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.lock.v1" || doc["name"] != "feat" || doc["reason"] != "review" || len(doc) != 4 {
		t.Errorf("document %v", doc)
	}
	mustSamePath(t, doc["path"].(string), feat)
	if preview.stdout != r.stdout {
		t.Errorf("--dry-run --json printed %q, --json %q", preview.stdout, r.stdout)
	}

	r = h.run("unlock", "feat", "--dry-run")
	r.mustCode(t, 0)
	if want := "would unlock worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}
	if locked, _ := h.lockState(repo, feat); !locked {
		t.Fatal("--dry-run unlocked the worktree")
	}
	r = h.run("unlock", "feat", "--json")
	r.mustCode(t, 0)
	doc = decodeOne(t, r.stdout)
	if doc["schema"] != "wt.unlock.v1" || doc["name"] != "feat" || len(doc) != 3 {
		t.Errorf("document %v", doc)
	}

	r = h.run("lock", "feat", "--json")
	r.mustCode(t, 0)
	if doc := decodeOne(t, r.stdout); doc["reason"] != nil {
		t.Errorf("without a reason: %v", doc)
	}
}

// --- 5.2: prune ---

// pruneRepo creates removeRepo's repository, with feat's directory deleted.
func pruneRepo(h *harness) (repo, feat string) {
	h.t.Helper()
	repo, feat = removeRepo(h)
	h.sb.MakePrunable(feat)
	return repo, feat
}

func TestPrune(t *testing.T) {
	h := newHarness(t)
	repo, feat := pruneRepo(h)
	alpha := h.sb.Path("repo.worktrees", "alpha")
	h.sb.AddWorktree(repo, alpha, "alpha")
	h.sb.MakePrunable(alpha)
	live := h.sb.Path("repo.worktrees", "live")
	h.sb.AddWorktree(repo, live, "live")

	r := h.run("prune")
	r.mustCode(t, 0)
	want := "pruned worktree " + testutil.Comparable(t, alpha) + "\npruned worktree " + testutil.Comparable(t, feat) + "\n"
	if r.stdout != want || r.stderr != "" {
		t.Errorf("stdout %q, stderr %q; want %q", r.stdout, r.stderr, want)
	}
	if h.isListed(repo, feat) || h.isListed(repo, alpha) || !h.isListed(repo, live) {
		t.Error("wrong worktrees pruned")
	}
	if h.branchHead(repo, "feat") == "" || h.branchHead(repo, "alpha") == "" {
		t.Error("a branch was deleted")
	}

	r = h.run("prune")
	r.mustCode(t, 0)
	if r.stdout != "nothing to prune\n" {
		t.Errorf("nothing to prune: stdout %q", r.stdout)
	}
}

func TestPruneKeepsLockedWorktrees(t *testing.T) {
	h := newHarness(t)
	repo, feat := pruneRepo(h)
	h.sb.LockWorktree(repo, feat, "")
	r := h.run("prune")
	r.mustCode(t, 0)
	if r.stdout != "nothing to prune\n" || !h.isListed(repo, feat) {
		t.Errorf("stdout %q; feat listed: %v", r.stdout, h.isListed(repo, feat))
	}
}

func TestPruneJSONAndDryRun(t *testing.T) {
	h := newHarness(t)
	repo, feat := pruneRepo(h)

	r := h.run("prune", "--dry-run")
	r.mustCode(t, 0)
	if want := "would prune worktree " + testutil.Comparable(t, feat) + "\n"; r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}
	if !h.isListed(repo, feat) {
		t.Fatal("--dry-run pruned")
	}

	preview := h.run("prune", "--dry-run", "--json")
	preview.mustCode(t, 0)
	r = h.run("prune", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	pruned, _ := doc["pruned"].([]any)
	if doc["schema"] != "wt.prune.v1" || len(pruned) != 1 {
		t.Fatalf("document %v", doc)
	}
	w := pruned[0].(map[string]any)
	if w["name"] != "feat" || w["reason"] == "" || len(w) != 3 {
		t.Errorf("element %v", w)
	}
	mustSamePath(t, w["path"].(string), feat)
	if preview.stdout != r.stdout {
		t.Errorf("--dry-run --json printed %q, --json %q", preview.stdout, r.stdout)
	}

	r = h.run("prune", "--json")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, `"pruned": []`) {
		t.Errorf("nothing pruned: %s", r.stdout)
	}
	h.run("prune", "--dry-run").mustCode(t, 0)
}

func TestPruneErrors(t *testing.T) {
	h := newHarness(t)
	removeRepo(h)
	r := h.run("prune", "x")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, `unexpected argument "x"`) {
		t.Errorf("stderr %q", r.stderr)
	}
	h.cwd = h.sb.Path("outside")
	r = h.run("prune")
	r.mustCode(t, 3)
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("first line %q, want %q", got, want)
	}
}

func TestPruneRunsNoGitWhenNothingIsPrunable(t *testing.T) {
	h := newHarness(t)
	removeRepo(h)
	calls, restore := cli.RecordGit()
	defer restore()
	h.run("prune").mustCode(t, 0)
	for _, args := range *calls {
		if slices.Contains(args, "prune") {
			t.Errorf("git %q ran", args)
		}
	}
	if len(*calls) == 0 {
		t.Error("no git command recorded")
	}
}

// --- 6.1: completions ---

func TestRemoveLockUnlockCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _ := removeRepo(h)
	gone := h.sb.Path("repo.worktrees", "gone")
	h.sb.AddWorktree(repo, gone, "gone")
	h.sb.MakePrunable(gone)
	usb := h.sb.Path("repo.worktrees", "usb")
	h.sb.AddWorktree(repo, usb, "usb")
	h.sb.LockWorktree(repo, usb, "on usb drive")
	// A warning while typing would land in the middle of the command line.
	h.userConfig("colour = \"blue\"\n")

	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"remove", ""}, []string{"feat", "gone"}},
		{[]string{"remove", "--force", ""}, []string{"feat", "gone"}},
		{[]string{"remove", "g"}, []string{"gone"}},
		{[]string{"lock", ""}, []string{"feat", "gone"}},
		{[]string{"unlock", ""}, []string{"usb"}},
		{[]string{"lock", "feat", ""}, nil},
		{[]string{"remove", "feat", ""}, nil},
		{[]string{"unlock", "usb", ""}, nil},
		{[]string{"prune", ""}, nil},
		// wt cd still leaves out the worktrees whose directory is gone.
		{[]string{"cd", ""}, []string{"repo", "feat", "usb"}},
	} {
		got, directive, r := h.complete(tc.args...)
		if !slices.Equal(got, tc.want) || directive != ":4" || r.stderr != "" {
			t.Errorf("%q: offered %q (directive %s, stderr %q), want %q", tc.args, got, directive, r.stderr, tc.want)
		}
	}

	h.cwd = h.sb.Path("outside")
	for _, args := range [][]string{{"remove", ""}, {"unlock", ""}, {"lock", ""}} {
		got, directive, r := h.complete(args...)
		if len(got) != 0 || directive != ":4" || r.stderr != "" {
			t.Errorf("outside a repository, %q: offered %q (directive %s, stderr %q)", args, got, directive, r.stderr)
		}
	}
}

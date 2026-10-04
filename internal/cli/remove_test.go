package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// recordChdir makes Env.Chdir record its calls instead of changing the test
// process's working directory. at, when not nil, runs on each call.
func (h *harness) recordChdir(at func(dir string)) *[]string {
	var calls []string
	h.chdir = func(dir string) error {
		calls = append(calls, dir)
		if at != nil {
			at(dir)
		}
		return nil
	}
	return &calls
}

// removeRepo creates <sandbox>/origin.git, its clone <sandbox>/repo and the
// linked worktree <sandbox>/repo.worktrees/feat on the branch feat, at
// origin/main: merged. It starts in the main worktree.
func removeRepo(h *harness) (repo, feat string) {
	h.t.Helper()
	repo, _ = cloneRepo(h)
	feat = h.sb.Path("repo.worktrees", "feat")
	h.sb.AddWorktree(repo, feat, "feat")
	return repo, feat
}

// isListed reports whether wt list, run in repo, shows the worktree at
// path.
func (h *harness) isListed(repo, path string) bool {
	h.t.Helper()
	saved := h.cwd
	h.cwd = repo
	defer func() { h.cwd = saved }()
	ws, _ := listed(h.t, h)
	_, ok := ws[testutil.Comparable(h.t, path)]
	return ok
}

// mustBeRemoved checks that the worktree at path is gone: directory and
// record.
func (h *harness) mustBeRemoved(repo, path string) {
	h.t.Helper()
	if exists(path) {
		h.t.Errorf("the directory %s still exists", path)
	}
	if h.isListed(repo, path) {
		h.t.Errorf("wt list still shows %s", path)
	}
}

// mustBeIntact checks that the worktree at path and its branch are still
// there.
func (h *harness) mustBeIntact(repo, path, branch string) {
	h.t.Helper()
	if !exists(path) || !h.isListed(repo, path) {
		h.t.Errorf("the worktree %s was removed", path)
	}
	if branch != "" && h.branchHead(repo, branch) == "" {
		h.t.Errorf("the branch %s was deleted", branch)
	}
}

// commitFile commits a file in dir and returns the commit's id.
func (h *harness) commitFile(dir, name, content string) string {
	h.t.Helper()
	h.sb.WriteFile(filepath.Join(dir, name), content)
	h.sb.Git(dir, "add", name)
	h.sb.Git(dir, "commit", "-q", "-m", "add "+name)
	return h.sb.Git(dir, "rev-parse", "HEAD")
}

// --- 3.2: Env.Chdir ---

func TestCommandsThatDoNotRemoveNeverChdir(t *testing.T) {
	h := newHarness(t)
	cloneRepo(h)
	calls := h.recordChdir(nil)
	h.activate()
	h.run("list").mustCode(t, 0)
	path := h.mustCreate("feat", "feat")
	h.run("cd", "feat").mustCode(t, 0)
	h.cwd = path
	h.run("cd", "^").mustCode(t, 0)
	h.run("list", "--json").mustCode(t, 0)
	if len(*calls) != 0 {
		t.Errorf("Chdir called with %q", *calls)
	}
}

// --- 4.1: arguments and flags ---

func TestRemoveUsageErrors(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	for _, tc := range []struct {
		args []string
		want string
	}{
		// "No target".
		{[]string{"remove"}, "<target>"},
		{[]string{"remove", "feat", "x"}, `unexpected argument "x"`},
		// "Contradictory branch flags".
		{[]string{"remove", "feat", "--keep-branch", "-D"}, "--keep-branch"},
		{[]string{"remove", "feat", "--keep-branch", "--force-delete-branch", "--dry-run"}, "-D"},
		{[]string{"remove", "feat", "--delete-branch"}, "--delete-branch"},
	} {
		r := h.run(tc.args...)
		if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, tc.want) {
			t.Errorf("wt %q: exit %d, stdout %q, stderr %q; want 2 and stderr naming %q", tc.args, r.code, r.stdout, r.stderr, tc.want)
		}
	}
	h.mustBeIntact(repo, feat, "feat")
}

func TestRemoveHelp(t *testing.T) {
	r := newHarness(t).run("remove", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{
		"wt remove <target> [flags]", "--keep-branch", "-D, --force-delete-branch", "-f, --force",
		"merged", "default_base", "upstream", "squash merge", "independent", "ignore", "main worktree", "wt unlock",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not mention %q:\n%s", want, r.stdout)
		}
	}
}

// --- 4.2: target and blocks ---

func TestRemoveTarget(t *testing.T) {
	t.Run("by branch", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		abc := h.sb.Path("repo.worktrees", "feature-abc1")
		h.sb.AddWorktree(repo, abc, "feature/abc1")
		h.run("remove", "feature/abc1").mustCode(t, 0)
		h.mustBeRemoved(repo, abc)
	})
	t.Run("current worktree", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.cwd = filepath.Join(feat, "src")
		if err := os.Mkdir(h.cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		h.run("remove", "@").mustCode(t, 0)
		h.mustBeRemoved(repo, feat)
	})
}

func TestRemoveResolutionErrors(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	a, b := h.sb.Path("a", "x"), h.sb.Path("b", "x")
	h.sb.AddWorktree(repo, a, "feature/x")
	h.sb.AddWorktree(repo, b, "fix/x")

	for _, tc := range []struct {
		args  []string
		code  int
		first string
	}{
		{[]string{"remove", "^"}, 2, "wt: cannot remove the main worktree"},
		{[]string{"remove", "^", "--force", "-D"}, 2, "wt: cannot remove the main worktree"},
		{[]string{"remove", "nope"}, 3, `wt: no worktree named "nope"`},
		{[]string{"remove", "x"}, 4, `wt: "x" matches more than one worktree`},
		{[]string{"remove", "@"}, 2, "wt: cannot remove the main worktree"},
	} {
		r := h.run(tc.args...)
		if r.code != tc.code || firstLine(r.stderr) != tc.first || r.stdout != "" {
			t.Errorf("wt %q: exit %d, stderr %q; want %d and %q", tc.args, r.code, r.stderr, tc.code, tc.first)
		}
	}
	for _, p := range []string{repo, feat, a, b} {
		if !exists(p) || !h.isListed(repo, p) {
			t.Errorf("%s was removed", p)
		}
	}

	h.cwd = h.sb.Path("outside")
	r := h.run("remove", "feat")
	r.mustCode(t, 3)
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
}

func TestRemoveBareMainWorktree(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("repo.git")
	h.sb.InitBare(bare)
	h.cwd = bare
	r := h.run("remove", "^")
	r.mustCode(t, 2)
	if got := firstLine(r.stderr); got != "wt: cannot remove the main worktree" {
		t.Errorf("first line = %q", got)
	}
}

func TestRemoveLocked(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.LockWorktree(repo, feat, "on usb drive")

	r := h.run("remove", "feat")
	r.mustCode(t, 5)
	if first := firstLine(r.stderr); !strings.Contains(first, "locked") || !strings.Contains(first, "on usb drive") {
		t.Errorf("first line = %q", first)
	}
	if !strings.Contains(r.stderr, "\nhint: ") || !strings.Contains(r.stderr, "wt unlock feat") {
		t.Errorf("stderr = %q, want a hint naming wt unlock feat", r.stderr)
	}
	// "Locked and forced".
	h.run("remove", "feat", "--force", "-D").mustCode(t, 5)
	h.mustBeIntact(repo, feat, "feat")
}

func TestRemoveNestedWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	outer := h.sb.Path("outer")
	inner := filepath.Join(outer, "inner")
	h.sb.AddWorktree(repo, outer, "outer")
	h.sb.AddWorktree(repo, inner, "inner")
	untracked := filepath.Join(inner, "work.txt")
	h.sb.WriteFile(untracked, "unsaved")

	for _, args := range [][]string{{"remove", "outer", "--force"}, {"remove", "outer"}, {"remove", "outer", "--dry-run"}} {
		r := h.run(args...)
		r.mustCode(t, 5)
		if first := firstLine(r.stderr); !strings.Contains(first, `"outer"`) || !strings.Contains(first, `"inner"`) {
			t.Errorf("wt %q: first line %q, want it to name outer and inner", args, first)
		}
	}
	if !exists(untracked) {
		t.Error("the untracked file in inner is gone")
	}
	h.mustBeIntact(repo, outer, "outer")
}

func TestRemoveDirtyWorktree(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dirty func(h *harness, repo, feat string)
	}{
		{"untracked file", func(h *harness, repo, feat string) {
			h.sb.WriteFile(filepath.Join(feat, "new.txt"), "")
		}},
		{"modified file", func(h *harness, repo, feat string) {
			h.sb.WriteFile(filepath.Join(feat, "tracked.txt"), "changed\n")
		}},
		{"staged file", func(h *harness, repo, feat string) {
			h.sb.WriteFile(filepath.Join(feat, "tracked.txt"), "changed\n")
			h.sb.Git(feat, "add", "tracked.txt")
		}},
		{"untracked files hidden by the configuration", func(h *harness, repo, feat string) {
			h.sb.Git(repo, "config", "status.showUntrackedFiles", "no")
			h.sb.WriteFile(filepath.Join(feat, "new.txt"), "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			repo, feat := removeRepo(h)
			h.commitFile(feat, "tracked.txt", "one\n")
			tc.dirty(h, repo, feat)

			for _, args := range [][]string{{"remove", "feat"}, {"remove", "feat", "-D"}, {"remove", "feat", "--dry-run"}} {
				r := h.run(args...)
				r.mustCode(t, 5)
				if got := firstLine(r.stderr); got != `wt: worktree "feat" has modified or untracked files` {
					t.Errorf("wt %q: first line %q", args, got)
				}
				if !strings.Contains(r.stderr, "\nhint: ") || !strings.Contains(r.stderr, "--force") || r.stdout != "" {
					t.Errorf("wt %q: stdout %q, stderr %q; want a hint naming --force", args, r.stdout, r.stderr)
				}
			}
			h.mustBeIntact(repo, feat, "feat")
		})
	}
}

// --- 4.3 and 4.4: merged branch ---

// mergedCase is a scenario of "Merged branch" (git-worktrees): setup
// prepares the branch feat of the worktree at feat.
type mergedCase struct {
	name   string
	setup  func(h *harness, repo, feat string) // nil: feat at origin/main
	plain  bool                                // a repository without origin, instead of removeRepo's clone
	merged bool
	keep   string // the line when kept
	warn   string // a warning stderr contains
}

var mergedCases = []mergedCase{
	{name: "no commits of its own", merged: true},
	{name: "merged into the base", merged: true, setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
		h.sb.Push(feat, "origin", "HEAD:main")
		h.sb.PushFromAnotherClone(h.sb.Path("origin.git"), "main")
		h.sb.Git(repo, "fetch", "-q", "origin")
	}},
	{name: "commit not in the base", keep: "branch feat: not merged into origin/main", setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
	}},
	{name: "pushed to its upstream", merged: true, setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
		h.sb.Push(feat, "origin", "feat")
		h.sb.SetUpstream(feat, "feat", "origin/feat")
	}},
	{name: "ahead of its upstream", keep: "branch feat: not merged into origin/main", setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
		h.sb.Push(feat, "origin", "feat")
		h.sb.SetUpstream(feat, "feat", "origin/feat")
		h.sb.Commit(feat, "more work")
	}},
	{name: "upstream deleted", keep: "branch feat: not merged into origin/main", setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
		h.sb.Push(feat, "origin", "feat")
		h.sb.SetUpstream(feat, "feat", "origin/feat")
		h.sb.Push(feat, "origin", "--delete", "feat")
	}},
	{name: "squash merge", keep: "branch feat: not merged into origin/main", setup: func(h *harness, repo, feat string) {
		h.commitFile(feat, "feat.txt", "feat\n")
		h.sb.SquashMerge(repo, "feat")
		h.sb.Push(repo, "origin", "main")
	}},
	{name: "base from the configuration", merged: true, setup: func(h *harness, repo, feat string) {
		h.sb.Commit(feat, "feat work")
		h.sb.Git(repo, "branch", "develop", "feat")
		h.userConfig("default_base = \"develop\"\n")
	}},
	{name: "base not found", keep: "branch feat: not merged", warn: "wt: warning: cannot tell whether branch feat is merged: base not found: origin/develop", setup: func(h *harness, repo, feat string) {
		h.userConfig("default_base = \"origin/develop\"\n")
	}},
	{name: "no base", plain: true, keep: "branch feat: not merged", warn: "wt: warning: cannot tell whether branch feat is merged: the repository has no default branch", setup: func(h *harness, repo, feat string) {
		h.sb.Git(repo, "checkout", "-q", "--detach")
	}},
}

// setUp builds the repository of tc and returns it and feat's path.
func (tc mergedCase) setUp(h *harness) (repo, feat string) {
	h.t.Helper()
	if tc.plain {
		repo = h.repo()
		h.cwd = repo
		feat = h.sb.Path("repo.worktrees", "feat")
		h.sb.AddWorktree(repo, feat, "feat")
	} else {
		repo, feat = removeRepo(h)
	}
	if tc.setup != nil {
		tc.setup(h, repo, feat)
	}
	return repo, feat
}

func TestRemoveMergedBranchPreview(t *testing.T) {
	for _, tc := range mergedCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			repo, feat := tc.setUp(h)
			r := h.run("remove", "feat", "--dry-run")
			r.mustCode(t, 0)
			want := "would delete branch feat"
			if !tc.merged {
				want = "would keep " + tc.keep
			}
			lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
			if len(lines) != 2 || lines[1] != want {
				t.Errorf("stdout %q, want the second line %q", r.stdout, want)
			}
			if tc.warn != "" && !strings.Contains(r.stderr, tc.warn+"\n") || tc.warn == "" && r.stderr != "" {
				t.Errorf("stderr %q, want warning %q", r.stderr, tc.warn)
			}
			if tc.warn != "" && strings.Count(r.stderr, "warning") != 1 {
				t.Errorf("stderr %q, want the warning once", r.stderr)
			}
			h.mustBeIntact(repo, feat, "feat")
		})
	}
}

func TestRemoveMergedBranch(t *testing.T) {
	for _, tc := range mergedCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			repo, feat := tc.setUp(h)
			r := h.run("remove", "feat")
			r.mustCode(t, 0)
			h.mustBeRemoved(repo, feat)
			if deleted := h.branchHead(repo, "feat") == ""; deleted != tc.merged {
				t.Errorf("branch deleted: %v, want %v", deleted, tc.merged)
			}
			want := "deleted branch feat"
			if !tc.merged {
				want = "kept " + tc.keep
			}
			if !strings.Contains(r.stdout, "\n"+want+"\n") {
				t.Errorf("stdout %q, want %q", r.stdout, want)
			}
			if tc.warn != "" && !strings.Contains(r.stderr, tc.warn) {
				t.Errorf("stderr %q, want warning %q", r.stderr, tc.warn)
			}
		})
	}
}

// --- 4.3: dry run ---

func TestRemoveErrorsUnderDryRun(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.WriteFile(filepath.Join(feat, "new.txt"), "")
	h.run("remove", "feat", "--dry-run").mustCode(t, 5)
	h.mustBeIntact(repo, feat, "feat")
}

func TestRemovePreview(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	directive := h.activate()
	h.cwd = feat
	r := h.run("remove", "@", "--dry-run")
	r.mustCode(t, 0)
	want := strings.Join([]string{
		"would remove worktree " + testutil.Comparable(t, feat),
		"would delete branch feat",
		"would change directory to " + testutil.Comparable(t, repo),
	}, "\n") + "\n"
	if r.stdout != want || r.stderr != "" {
		t.Errorf("stdout %q, stderr %q; want stdout %q", r.stdout, r.stderr, want)
	}
	if got := readDirective(t, directive); got != "" {
		t.Errorf("directive %q, want empty", got)
	}
	h.mustBeIntact(repo, feat, "feat")

	// Without the function: no directory line, and the warning.
	h.sb.Setenv("WT_DIRECTIVE_CD_FILE", "")
	r = h.run("remove", "@", "--dry-run")
	r.mustCode(t, 0)
	if strings.Contains(r.stdout, "would change directory") || !strings.Contains(r.stderr, "shell integration is not active") {
		t.Errorf("without the function: stdout %q, stderr %q", r.stdout, r.stderr)
	}
	// From elsewhere: no directory line.
	h.activate()
	h.cwd = repo
	if r := h.run("remove", "feat", "--dry-run"); strings.Contains(r.stdout, "would change directory") {
		t.Errorf("from elsewhere: stdout %q", r.stdout)
	}
	h.mustBeIntact(repo, feat, "feat")
}

func TestRemoveJSON(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	scratch := h.sb.Path("repo.worktrees", "scratch")
	h.sb.AddDetachedWorktree(repo, scratch)

	preview := h.run("remove", "feat", "--dry-run", "--json")
	preview.mustCode(t, 0)
	r := h.run("remove", "feat", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.remove.v1" || doc["name"] != "feat" || doc["branch"] != "feat" || doc["branch_deleted"] != true || len(doc) != 5 {
		t.Errorf("document %v", doc)
	}
	mustSamePath(t, doc["path"].(string), feat)
	if pdoc := decodeOne(t, preview.stdout); !reflect.DeepEqual(pdoc, doc) {
		t.Errorf("--dry-run --json printed %v, --json %v", pdoc, doc)
	}
	h.mustBeRemoved(repo, feat)

	r = h.run("remove", "scratch", "--json")
	r.mustCode(t, 0)
	if doc := decodeOne(t, r.stdout); doc["branch"] != nil || doc["branch_deleted"] != false {
		t.Errorf("detached: %v", doc)
	}
}

// --- 4.4: removing ---

func TestRemoveALinkedWorktree(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	r := h.run("remove", "feat")
	r.mustCode(t, 0)
	want := "removed worktree " + testutil.Comparable(t, feat) + "\ndeleted branch feat\n"
	if r.stdout != want || r.stderr != "" {
		t.Errorf("stdout %q, stderr %q; want stdout %q", r.stdout, r.stderr, want)
	}
	h.mustBeRemoved(repo, feat)
}

func TestRemoveDirectoryAlreadyGone(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.MakePrunable(feat)
	r := h.run("remove", "feat")
	r.mustCode(t, 0)
	if !strings.HasPrefix(r.stdout, "removed worktree "+testutil.Comparable(t, feat)+"\n") {
		t.Errorf("stdout %q", r.stdout)
	}
	h.mustBeRemoved(repo, feat)
}

func TestRemoveForced(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.sb.Commit(feat, "not merged")
	h.sb.WriteFile(filepath.Join(feat, "new.txt"), "")
	h.sb.WriteFile(filepath.Join(feat, "deep", "er.txt"), "")
	r := h.run("remove", "feat", "--force")
	r.mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
	if h.branchHead(repo, "feat") == "" {
		t.Error("--force deleted a branch that is not merged")
	}
	if !strings.Contains(r.stdout, "\nkept branch feat: not merged into origin/main\n") {
		t.Errorf("stdout %q", r.stdout)
	}
}

func TestRemoveIgnoredFiles(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.commitFile(feat, ".gitignore", "node_modules/\n*.env\n")
	h.sb.WriteFile(filepath.Join(feat, "node_modules", "x", "index.js"), "")
	h.sb.WriteFile(filepath.Join(feat, "secret.env"), "")
	h.run("remove", "feat").mustCode(t, 0)
	h.mustBeRemoved(repo, feat)
}

func TestRemoveFreshWorktree(t *testing.T) {
	h := newHarness(t)
	repo, _ := cloneRepo(h)
	path := h.mustCreate("feat", "feat")
	r := h.run("remove", "feat")
	r.mustCode(t, 0)
	if h.branchHead(repo, "feat") != "" || !strings.Contains(r.stdout, "deleted branch feat") {
		t.Errorf("stdout %q; branch feat at %q", r.stdout, h.branchHead(repo, "feat"))
	}
	h.mustBeRemoved(repo, path)
}

func TestRemoveBranchFlags(t *testing.T) {
	t.Run("unmerged branch is kept", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.sb.Commit(feat, "not merged")
		r := h.run("remove", "feat")
		r.mustCode(t, 0)
		h.mustBeRemoved(repo, feat)
		if h.branchHead(repo, "feat") == "" || !strings.Contains(r.stdout, "kept branch feat: not merged into origin/main") {
			t.Errorf("stdout %q", r.stdout)
		}
	})
	t.Run("keeping a merged branch", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := removeRepo(h)
		r := h.run("remove", "feat", "--keep-branch")
		r.mustCode(t, 0)
		if h.branchHead(repo, "feat") == "" || !strings.HasSuffix(r.stdout, "\nkept branch feat\n") {
			t.Errorf("stdout %q", r.stdout)
		}
	})
	t.Run("deleting an unmerged branch", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.sb.Commit(feat, "not merged")
		r := h.run("remove", "feat", "-D")
		r.mustCode(t, 0)
		if h.branchHead(repo, "feat") != "" || !strings.HasSuffix(r.stdout, "\ndeleted branch feat\n") {
			t.Errorf("stdout %q", r.stdout)
		}
	})
	t.Run("detached worktree", func(t *testing.T) {
		h := newHarness(t)
		repo, _ := cloneRepo(h)
		scratch := h.sb.Path("repo.worktrees", "scratch")
		h.sb.AddDetachedWorktree(repo, scratch)
		before := h.sb.Git(repo, "for-each-ref", "refs/heads")
		r := h.run("remove", "scratch", "-D")
		r.mustCode(t, 0)
		h.mustBeRemoved(repo, scratch)
		if after := h.sb.Git(repo, "for-each-ref", "refs/heads"); after != before {
			t.Errorf("branches changed:\n%s\n%s", before, after)
		}
		if r.stdout != "removed worktree "+testutil.Comparable(t, scratch)+"\n" {
			t.Errorf("stdout %q", r.stdout)
		}
	})
}

func TestRemoveFromABareRepository(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("repo.git")
	h.sb.InitBare(bare)
	feat := h.sb.Path("repo.worktrees", "feat")
	h.sb.AddWorktree(bare, feat, "feat")
	h.cwd = bare
	calls := h.recordChdir(nil)
	r := h.run("remove", "feat")
	r.mustCode(t, 0)
	h.mustBeRemoved(bare, feat)
	if h.branchHead(bare, "feat") != "" {
		t.Error("the merged branch was kept")
	}
	if len(*calls) != 1 || testutil.Comparable(t, (*calls)[0]) != testutil.Comparable(t, bare) {
		t.Errorf("Chdir called with %q, want the bare repository", *calls)
	}
}

func TestRemoveLeavesTheDirectoryFirst(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.cwd = feat
	stillThere := false
	calls := h.recordChdir(func(string) { stillThere = exists(feat) })
	h.run("remove", "@").mustCode(t, 0)
	if len(*calls) != 1 || testutil.Comparable(t, (*calls)[0]) != testutil.Comparable(t, repo) || !stillThere {
		t.Errorf("Chdir called with %q (worktree there at the time: %v), want the main worktree before the removal", *calls, stillThere)
	}
	// --dry-run removes nothing, and leaves nothing.
	h.cwd = repo
	feat2 := h.sb.Path("repo.worktrees", "feat2")
	h.sb.AddWorktree(repo, feat2, "feat2")
	*calls = nil
	h.run("remove", "feat2", "--dry-run").mustCode(t, 0)
	if len(*calls) != 0 {
		t.Errorf("--dry-run called Chdir with %q", *calls)
	}
}

func TestRemoveDirectoryCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	h.commitFile(feat, filepath.Join("sub", "file.txt"), "x")
	h.sb.Push(feat, "origin", "HEAD:main") // merged: only the directory stops it
	h.sb.ReadOnlyDir(filepath.Join(feat, "sub"))
	directive := h.activate()
	h.cwd = feat

	r := h.run("remove", "@")
	r.mustCode(t, 1)
	if r.stdout != "" || !strings.Contains(r.stderr, "git worktree remove") || !strings.Contains(r.stderr, "Permission denied") {
		t.Errorf("stdout %q, stderr %q; want git's message", r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "\nhint: the directory is still there: "+testutil.Comparable(t, feat)+"\n") {
		t.Errorf("stderr %q, want a hint naming the directory", r.stderr)
	}
	if h.branchHead(repo, "feat") == "" {
		t.Error("the branch was deleted")
	}
	if got := readDirective(t, directive); got != "" {
		t.Errorf("the shell moved: directive %q", got)
	}
}

// --- 4.5: moving the shell ---

func TestRemoveMovesTheShell(t *testing.T) {
	t.Run("from inside", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		directive := h.activate()
		h.cwd = filepath.Join(feat, "src")
		if err := os.Mkdir(h.cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		r := h.run("remove", "@")
		r.mustCode(t, 0)
		mustDirective(t, directive, repo)
		if r.stderr != "" {
			t.Errorf("stderr %q", r.stderr)
		}
	})
	t.Run("from elsewhere", func(t *testing.T) {
		h := newHarness(t)
		_, _ = removeRepo(h)
		directive := h.activate()
		h.run("remove", "feat").mustCode(t, 0)
		if got := readDirective(t, directive); got != "" {
			t.Errorf("directive %q, want empty", got)
		}
	})
	t.Run("working directory override", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		directive := h.activate()
		h.cwd = feat
		h.run("-C", repo, "remove", "feat").mustCode(t, 0)
		mustDirective(t, directive, repo)
	})
	t.Run("without the function", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		h.cwd = feat
		r := h.run("remove", "@")
		r.mustCode(t, 0)
		h.mustBeRemoved(repo, feat)
		want := "wt: warning: shell integration is not active, so the shell stays in a directory that no longer exists; see 'wt shell init --help'\n"
		if r.stderr != want {
			t.Errorf("stderr %q, want %q", r.stderr, want)
		}
	})
	t.Run("directive cannot be written", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		missing := h.sb.Path("missing", "directive")
		h.sb.Setenv("WT_DIRECTIVE_CD_FILE", missing)
		h.cwd = feat
		r := h.run("remove", "@")
		r.mustCode(t, 1)
		if r.stdout != "" || !strings.Contains(r.stderr, "directive") || !strings.Contains(r.stderr, "removed worktree") {
			t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
		}
		h.mustBeRemoved(repo, feat)
		if exists(missing) {
			t.Error("the directive file was created")
		}
	})
	t.Run("branch cannot be deleted", func(t *testing.T) {
		h := newHarness(t)
		repo, feat := removeRepo(h)
		// The same branch checked out twice: git refuses to delete it.
		twin := h.sb.Path("twin")
		h.sb.Git(repo, "worktree", "add", "-q", "--force", twin, "feat")
		directive := h.activate()
		h.cwd = feat
		r := h.run("remove", "feat")
		r.mustCode(t, 1)
		if r.stdout != "" || !strings.Contains(r.stderr, "but cannot delete branch feat") {
			t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
		}
		h.mustBeRemoved(repo, feat)
		mustDirective(t, directive, repo)
	})
}

// --- 4.6 and 7.3: another process inside the worktree ---

func TestRemoveDirectoryInUseByAnotherProcess(t *testing.T) {
	h := newHarness(t)
	repo, feat := removeRepo(h)
	var child *exec.Cmd
	if runtime.GOOS == "windows" {
		child = exec.Command("cmd", "/c", "pause")
		stdin, err := child.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer stdin.Close()
	} else {
		child = exec.Command("sleep", "30")
	}
	child.Dir = feat
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})

	r := h.run("remove", "feat")
	if runtime.GOOS != "windows" {
		r.mustCode(t, 0)
		h.mustBeRemoved(repo, feat)
		return
	}
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "git worktree remove") || !strings.Contains(r.stderr, "\nhint: the directory is still there: "+testutil.Comparable(t, feat)+"\n") {
		t.Errorf("stderr %q, want git's message and a hint naming the directory", r.stderr)
	}
}

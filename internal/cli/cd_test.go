package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// activate stands in for the shell function: it creates an empty directive
// file in the sandbox and points WT_DIRECTIVE_CD_FILE at it.
func (h *harness) activate() string {
	h.t.Helper()
	f, err := os.CreateTemp(h.sb.Root, "wt.directive.")
	if err != nil {
		h.t.Fatal(err)
	}
	f.Close()
	h.sb.Setenv("WT_DIRECTIVE_CD_FILE", f.Name())
	return f.Name()
}

func readDirective(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mustDirective checks that the directive names want, which must be the
// path exactly: the comparison resolves symbolic links on both sides only to
// tell which directory each one is.
func mustDirective(t *testing.T, path, want string) {
	t.Helper()
	got := readDirective(t, path)
	if got == "" || testutil.Comparable(t, got) != testutil.Comparable(t, want) {
		t.Fatalf("directive = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) || got != filepath.Clean(got) || strings.ContainsAny(got, "\r\n") {
		t.Errorf("directive %q is not a clean native absolute path", got)
	}
}

// cdRepo creates <sandbox>/repo with the linked worktree
// <sandbox>/repo.worktrees/feat, and starts in a subdirectory of the main one.
func cdRepo(h *harness) (repo, feat string) {
	h.t.Helper()
	repo = h.repo()
	feat = h.sb.Path("repo.worktrees", "feat")
	h.sb.AddWorktree(repo, feat, "feat")
	h.cwd = filepath.Join(repo, "sub")
	if err := os.Mkdir(h.cwd, 0o755); err != nil {
		h.t.Fatal(err)
	}
	return repo, feat
}

func TestCdArguments(t *testing.T) {
	h := newHarness(t)
	cdRepo(h)
	h.activate()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"cd"}, "<target>"},
		{[]string{"cd", "feat", "fix"}, `unexpected argument "fix"`},
	} {
		r := h.run(tc.args...)
		r.mustCode(t, 2)
		if !strings.Contains(r.stderr, tc.want) || r.stdout != "" {
			t.Errorf("wt %v: stdout %q, stderr %q; want stderr to contain %q", tc.args, r.stdout, r.stderr, tc.want)
		}
	}
}

func TestCdWithoutTheFunction(t *testing.T) {
	h := newHarness(t)
	cdRepo(h)
	for _, args := range [][]string{
		{"cd", "feat"},
		{"cd", "does-not-exist"},
		{"cd", "-"},
		{"cd", "--dry-run", "feat"},
	} {
		r := h.run(args...)
		r.mustCode(t, 1)
		if r.stdout != "" {
			t.Errorf("wt %v: stdout = %q, want empty", args, r.stdout)
		}
		if got := firstLine(r.stderr); got != "wt: shell integration is not active" {
			t.Errorf("wt %v: first line = %q", args, got)
		}
		if !strings.Contains(r.stderr, "\nhint: ") || !strings.Contains(r.stderr, "wt shell init") {
			t.Errorf("wt %v: stderr = %q, want a hint naming wt shell init", args, r.stderr)
		}
	}

	r := h.run("cd", "feat", "--json")
	r.mustCode(t, 1)
	doc := decodeOne(t, r.stderr)
	if doc["schema"] != "wt.error.v1" || doc["message"] != "shell integration is not active" || r.stdout != "" {
		t.Errorf("--json: stdout %q, error %v", r.stdout, doc)
	}

	// An empty variable is not an active integration.
	h.sb.Setenv("WT_DIRECTIVE_CD_FILE", "")
	h.run("cd", "feat").mustCode(t, 1)

	// -C is validated before the integration.
	h.run("-C", h.sb.Path("missing"), "cd", "feat").mustCode(t, 2)
}

func TestCdResolvesNames(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)
	abc := h.sb.Path("repo.worktrees", "feature-abc1")
	h.sb.AddWorktree(repo, abc, "feature/abc1")
	api := h.sb.Path("repo.worktrees", "api")
	h.sb.AddWorktree(repo, api, "fix")
	fix := h.sb.Path("repo.worktrees", "fix")
	h.sb.AddWorktree(repo, fix, "hotfix")

	for _, tc := range []struct {
		name, target, want string
	}{
		{"by name", "feat", feat},
		{"by branch", "feature/abc1", abc},
		{"name before branch", "fix", fix},
		{"main by its name", "repo", repo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := h.activate()
			r := h.run("cd", tc.target)
			r.mustCode(t, 0)
			if r.stdout != "" || r.stderr != "" {
				t.Errorf("stdout %q, stderr %q; want both empty", r.stdout, r.stderr)
			}
			mustDirective(t, d, tc.want)
		})
	}
}

func TestCdResolutionErrors(t *testing.T) {
	h := newHarness(t)
	repo, _ := cdRepo(h)
	a, b := h.sb.Path("a", "x"), h.sb.Path("b", "x")
	h.sb.AddWorktree(repo, a, "feature/x")
	h.sb.AddWorktree(repo, b, "fix/x")

	t.Run("ambiguous", func(t *testing.T) {
		d := h.activate()
		r := h.run("cd", "x")
		r.mustCode(t, 4)
		if got := firstLine(r.stderr); got != `wt: "x" matches more than one worktree` {
			t.Errorf("first line = %q", got)
		}
		for _, p := range []string{a, b} {
			if !strings.Contains(r.stderr, "hint: "+testutil.Comparable(t, p)+" (branch ") {
				t.Errorf("stderr has no hint with %s:\n%s", p, r.stderr)
			}
		}
		if readDirective(t, d) != "" {
			t.Error("the directive was written")
		}
	})
	for _, target := range []string{"nope", "Feat"} {
		t.Run("not found "+target, func(t *testing.T) {
			d := h.activate()
			r := h.run("cd", target)
			r.mustCode(t, 3)
			if got, want := firstLine(r.stderr), `wt: no worktree named "`+target+`"`; got != want {
				t.Errorf("first line = %q, want %q", got, want)
			}
			if !strings.Contains(r.stderr, "hint: run 'wt list'") {
				t.Errorf("stderr = %q, want a hint to run wt list", r.stderr)
			}
			if readDirective(t, d) != "" {
				t.Error("the directive was written")
			}
		})
	}
	t.Run("outside a repository", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		h.activate()
		r := h.run("cd", "feat")
		r.mustCode(t, 3)
		if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
			t.Errorf("first line = %q, want %q", got, want)
		}
	})
}

func TestCdMainAndCurrent(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)
	pkg := filepath.Join(feat, "src", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	h.cwd = pkg

	d := h.activate()
	h.run("cd", "^").mustCode(t, 0)
	mustDirective(t, d, repo)

	d = h.activate()
	h.run("cd", "@").mustCode(t, 0)
	mustDirective(t, d, feat)
}

func TestCdMainOfBareRepository(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("bare.git")
	h.sb.InitBare(bare)
	linked := h.sb.Path("bare-linked")
	h.sb.AddWorktree(bare, linked, "feature/x")
	h.cwd = linked

	d := h.activate()
	h.run("cd", "^").mustCode(t, 0)
	mustDirective(t, d, bare)
}

func TestCdDestinationMustExist(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)
	h.sb.MakePrunable(feat)

	d := h.activate()
	r := h.run("cd", "feat")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, testutil.Comparable(t, feat)) {
		t.Errorf("stderr %q does not name %s", r.stderr, feat)
	}
	if readDirective(t, d) != "" {
		t.Error("the directive was written")
	}
}

func TestCdMissingDirectiveFile(t *testing.T) {
	h := newHarness(t)
	cdRepo(h)
	missing := h.sb.Path("no-such-directive")
	h.sb.Setenv("WT_DIRECTIVE_CD_FILE", missing)

	r := h.run("cd", "feat")
	r.mustCode(t, 1)
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file exists at %s (stat: %v)", missing, err)
	}
}

func TestCdPrevious(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)
	prev := h.sb.Path("outside", "before")
	if err := os.Mkdir(prev, 0o755); err != nil {
		t.Fatal(err)
	}

	// The sandbox starts without WT_PREVIOUS_DIR; then it is set but empty.
	t.Run("no previous directory", func(t *testing.T) {
		for _, state := range []string{"unset", "empty"} {
			if state == "empty" {
				h.sb.Setenv("WT_PREVIOUS_DIR", "")
			}
			d := h.activate()
			r := h.run("cd", "-")
			r.mustCode(t, 3)
			if got := firstLine(r.stderr); got != "wt: no previous directory" {
				t.Errorf("%s: first line = %q", state, got)
			}
			if readDirective(t, d) != "" {
				t.Errorf("%s: the directive was written", state)
			}
		}
	})
	t.Run("outside a repository", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		h.sb.Setenv("WT_PREVIOUS_DIR", prev)
		d := h.activate()
		r := h.run("cd", "-")
		r.mustCode(t, 0)
		if got := readDirective(t, d); got != prev {
			t.Errorf("directive = %q, want %q", got, prev)
		}
	})
	t.Run("deleted", func(t *testing.T) {
		gone := h.sb.Path("gone")
		h.sb.Setenv("WT_PREVIOUS_DIR", gone)
		d := h.activate()
		r := h.run("cd", "-")
		r.mustCode(t, 3)
		if !strings.Contains(r.stderr, gone) || readDirective(t, d) != "" {
			t.Errorf("stderr = %q, directive %q; want the path named and no directive", r.stderr, readDirective(t, d))
		}
	})
	t.Run("-C does not change it", func(t *testing.T) {
		h.cwd = feat
		h.sb.Setenv("WT_PREVIOUS_DIR", prev)
		d := h.activate()
		h.run("-C", h.sb.Path("outside"), "cd", "-").mustCode(t, 0)
		if got := readDirective(t, d); got != prev {
			t.Errorf("directive = %q, want %q", got, prev)
		}
	})
}

func TestCdMainOfAnotherRepository(t *testing.T) {
	h := newHarness(t)
	cdRepo(h)
	other := h.sb.Path("other")
	h.sb.InitRepo(other)
	h.sb.AddWorktree(other, h.sb.Path("other-linked"), "x")

	d := h.activate()
	h.run("-C", h.sb.Path("other-linked"), "cd", "^").mustCode(t, 0)
	mustDirective(t, d, other)
}

func TestCdJSON(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)

	d := h.activate()
	r := h.run("cd", "feat", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	p, _ := doc["path"].(string)
	if doc["schema"] != "wt.cd.v1" || testutil.Comparable(t, p) != testutil.Comparable(t, feat) || len(doc) != 2 {
		t.Errorf("document = %v", doc)
	}
	if p != filepath.Clean(p) || !filepath.IsAbs(p) {
		t.Errorf("path %q is not a native absolute path", p)
	}
	if readDirective(t, d) != p {
		t.Errorf("directive = %q, want the JSON path %q", readDirective(t, d), p)
	}

	d = h.activate()
	dry := h.run("cd", "feat", "--json", "--dry-run")
	dry.mustCode(t, 0)
	if dry.stdout != r.stdout {
		t.Errorf("--dry-run --json = %q, want the same document %q", dry.stdout, r.stdout)
	}
	if readDirective(t, d) != "" {
		t.Error("--dry-run wrote the directive")
	}
}

func TestCdDryRun(t *testing.T) {
	h := newHarness(t)
	_, feat := cdRepo(h)

	d := h.activate()
	r := h.run("cd", "--dry-run", "feat")
	r.mustCode(t, 0)
	path, ok := strings.CutPrefix(strings.TrimSuffix(r.stdout, "\n"), "would change directory to ")
	if !ok || testutil.Comparable(t, path) != testutil.Comparable(t, feat) {
		t.Errorf("stdout = %q, want would change directory to %s", r.stdout, feat)
	}
	if readDirective(t, d) != "" {
		t.Error("--dry-run wrote the directive")
	}

	for _, tc := range []struct {
		target string
		code   int
	}{{"nope", 3}, {"-", 3}} {
		r := h.run("cd", "--dry-run", tc.target)
		r.mustCode(t, tc.code)
		if r.stdout != "" {
			t.Errorf("--dry-run %s: stdout = %q", tc.target, r.stdout)
		}
	}
}

func TestCdHelp(t *testing.T) {
	r := newHarness(t).run("cd", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"wt cd <target>", "EXTENDED_GLOB", "wt cd '^'", "wt cd '@'", "wt shell init"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not contain %q:\n%s", want, r.stdout)
		}
	}
}

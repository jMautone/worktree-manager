package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// mustPath runs wt path with args and checks that stdout is want, compared
// as a directory, followed by a newline, and that stderr is empty.
func mustPath(t *testing.T, h *harness, want string, args ...string) {
	t.Helper()
	r := h.run(append([]string{"path"}, args...)...)
	r.mustCode(t, 0)
	got, ok := strings.CutSuffix(r.stdout, "\n")
	if !ok || strings.Contains(got, "\n") || testutil.Comparable(t, got) != testutil.Comparable(t, want) {
		t.Errorf("wt path %v: stdout = %q, want %s and a newline", args, r.stdout, want)
	}
	if !filepath.IsAbs(got) || got != filepath.Clean(got) {
		t.Errorf("wt path %v: %q is not a clean native absolute path", args, got)
	}
	if r.stderr != "" {
		t.Errorf("wt path %v: stderr = %q", args, r.stderr)
	}
}

// Scenarios of "Printing a path" and "JSON output and dry run of path".
func TestPath(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)

	t.Run("path of a worktree", func(t *testing.T) {
		mustPath(t, h, feat, "feat")
		mustPath(t, h, repo, "^")
	})
	t.Run("without the function", func(t *testing.T) {
		// The sandbox has no WT_DIRECTIVE_CD_FILE: nothing is written.
		if h.sb.Getenv("WT_DIRECTIVE_CD_FILE") != "" {
			t.Fatal("the shell integration is active")
		}
		mustPath(t, h, feat, "feat")
	})
	t.Run("with the function, no directive", func(t *testing.T) {
		d := h.activate()
		mustPath(t, h, feat, "feat")
		if readDirective(t, d) != "" {
			t.Error("wt path wrote the directive")
		}
		h.sb.Setenv("WT_DIRECTIVE_CD_FILE", "")
	})
	t.Run("dash is a name", func(t *testing.T) {
		h.sb.Setenv("WT_PREVIOUS_DIR", h.sb.Path("outside"))
		r := h.run("path", "-")
		r.mustCode(t, 3)
		if got := firstLine(r.stderr); got != `wt: no worktree named "-"` || r.stdout != "" {
			t.Errorf("stdout %q, first line %q", r.stdout, got)
		}
	})
	t.Run("no target", func(t *testing.T) {
		r := h.run("path")
		r.mustCode(t, 2)
		if !strings.Contains(r.stderr, "<target>") {
			t.Errorf("stderr = %q", r.stderr)
		}
		h.run("path", "feat", "fix").mustCode(t, 2)
	})
	t.Run("JSON", func(t *testing.T) {
		r := h.run("path", "feat", "--json")
		r.mustCode(t, 0)
		doc := decodeOne(t, r.stdout)
		p, _ := doc["path"].(string)
		if doc["schema"] != "wt.path.v1" || len(doc) != 2 || testutil.Comparable(t, p) != testutil.Comparable(t, feat) {
			t.Errorf("document = %v", doc)
		}
		dry := h.run("path", "feat", "--json", "--dry-run")
		dry.mustCode(t, 0)
		if dry.stdout != r.stdout {
			t.Errorf("--dry-run --json = %q, want %q", dry.stdout, r.stdout)
		}
	})
	t.Run("dry run", func(t *testing.T) {
		mustPath(t, h, feat, "feat", "--dry-run")
	})
	t.Run("directory gone", func(t *testing.T) {
		h.sb.MakePrunable(feat)
		r := h.run("path", "feat")
		r.mustCode(t, 3)
		if r.stdout != "" || !strings.Contains(r.stderr, testutil.Comparable(t, feat)) {
			t.Errorf("stdout %q, stderr %q; want nothing printed and the path named", r.stdout, r.stderr)
		}
	})
}

func TestPathAcrossRepositories(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)

	t.Run("path of a repository", func(t *testing.T) {
		r := h.run("path", "web")
		r.mustCode(t, 0)
		if want := filepath.Join(root, "web") + "\n"; r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
	})
	t.Run("path of a worktree of another repository", func(t *testing.T) {
		h.cwd = filepath.Join(root, "web")
		mustPath(t, h, feat, "api:feat")
		mustPath(t, h, feat, "feat")
	})
	t.Run("repository itself in the .bare layout", func(t *testing.T) {
		proj := filepath.Join(root, "proj")
		h.sb.InitDotBare(proj)
		r := h.run("path", "proj:")
		r.mustCode(t, 0)
		if r.stdout != proj+"\n" {
			t.Errorf("wt path proj: = %q, want %s", r.stdout, proj)
		}
		mustPath(t, h, filepath.Join(proj, ".bare"), "proj:^")
	})
	t.Run("warning not written as JSON", func(t *testing.T) {
		broken := filepath.Join(root, "broken")
		h.sb.InitRepo(broken)
		h.sb.BreakHEAD(broken)
		h.cwd = h.sb.Path("outside")
		r := h.run("path", "feat", "--json")
		r.mustCode(t, 0)
		if r.stderr != "" {
			t.Errorf("stderr = %q, want empty", r.stderr)
		}
		if r := h.run("path", "feat"); !strings.Contains(r.stderr, "warning") {
			t.Errorf("without --json: stderr = %q, want the warning", r.stderr)
		}
	})
}

func TestPathHelp(t *testing.T) {
	r := newHarness(t).run("path", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"wt path <target>", `"wt cd <target>"`, "- is a name"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not contain %q:\n%s", want, r.stdout)
		}
	}
}

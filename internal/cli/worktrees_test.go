package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// listed runs `wt list --json` and returns the worktrees by comparable path.
func listed(t *testing.T, h *harness) (map[string]map[string]any, []string) {
	t.Helper()
	r := h.run("list", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	byPath := map[string]map[string]any{}
	var order []string
	for _, w := range doc["worktrees"].([]any) {
		w := w.(map[string]any)
		p := testutil.Comparable(t, w["path"].(string))
		byPath[p] = w
		order = append(order, p)
	}
	return byPath, order
}

func currentPath(ws map[string]map[string]any) string {
	cur := ""
	for p, w := range ws {
		if w["current"] == true {
			if cur != "" {
				return "MORE THAN ONE"
			}
			cur = p
		}
	}
	return cur
}

func TestRepositoryFromLinkedWorktree(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	linked := h.sb.Path("linked")
	h.sb.AddWorktree(repo, linked, "feature/abc1")
	// On macOS the sandbox is under /var, a symbolic link to /private/var,
	// which is what git reports: the working directory is given unresolved on
	// purpose ("Symbolic link in the working directory").
	h.cwd = filepath.Join(linked, "src")
	if err := os.Mkdir(h.cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	ws, order := listed(t, h)
	if len(ws) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(ws))
	}
	if got, want := currentPath(ws), testutil.Comparable(t, linked); got != want {
		t.Errorf("current = %s, want %s", got, want)
	}
	if order[0] != testutil.Comparable(t, repo) || ws[order[0]]["main"] != true {
		t.Errorf("main = %v, want %s first", ws[order[0]], repo)
	}
}

func TestRepositoryFromSubdirectoryOfMain(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.sb.AddWorktree(repo, h.sb.Path("linked"), "feature/abc1")
	h.cwd = filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(h.cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	ws, _ := listed(t, h)
	if got, want := currentPath(ws), testutil.Comparable(t, repo); got != want {
		t.Errorf("current = %s, want %s", got, want)
	}
	if ws[testutil.Comparable(t, repo)]["main"] != true {
		t.Error("main worktree not marked main")
	}
}

func TestRepositoryFromBare(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("bare.git")
	h.sb.InitBare(bare)
	linked := h.sb.Path("bare-linked")
	h.sb.AddWorktree(bare, linked, "feature/x")
	h.cwd = bare

	ws, order := listed(t, h)
	b := ws[testutil.Comparable(t, bare)]
	if b == nil || order[0] != testutil.Comparable(t, bare) {
		t.Fatalf("bare repository not listed first: %v", order)
	}
	if b["bare"] != true || b["main"] != true || b["current"] != true || b["branch"] != nil || b["head"] != nil {
		t.Errorf("bare entry = %v", b)
	}
	if l := ws[testutil.Comparable(t, linked)]; l == nil || l["branch"] != "feature/x" {
		t.Errorf("linked worktree of the bare repository = %v", l)
	}
}

func TestRepositoryOutside(t *testing.T) {
	h := newHarness(t)
	r := h.run("list")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, "not a git repository: "+h.cwd) {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestNestedWorktreeIsCurrent(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	nested := filepath.Join(repo, ".worktrees", "x")
	h.sb.AddWorktree(repo, nested, "x")
	h.cwd = nested

	ws, _ := listed(t, h)
	if got, want := currentPath(ws), testutil.Comparable(t, nested); got != want {
		t.Errorf("current = %s, want the nested worktree %s", got, want)
	}
}

func TestCurrentWorktreeWithDifferentCase(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("case-insensitive comparison applies to macOS and Windows")
	}
	h := newHarness(t)
	repo := h.repo()
	linked := h.sb.Path("linked")
	h.sb.AddWorktree(repo, linked, "feature/x")
	h.cwd = filepath.Join(filepath.Dir(linked), "LINKED")

	ws, _ := listed(t, h)
	if got, want := currentPath(ws), testutil.Comparable(t, linked); got != want {
		t.Errorf("current = %s, want %s", got, want)
	}
}

func TestNativePaths(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	r := h.run("list", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	p := doc["worktrees"].([]any)[0].(map[string]any)["path"].(string)
	if p != filepath.FromSlash(p) || !filepath.IsAbs(p) {
		t.Errorf("path %q is not a native absolute path", p)
	}
	if runtime.GOOS == "windows" && strings.Contains(p, "/") {
		t.Errorf("path %q uses forward slashes on Windows", p)
	}
}

func TestGitMissing(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	t.Setenv("PATH", "")

	r := h.run("list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "git not found") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestGitFailureCarriesGitMessage(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.sb.WriteFile(filepath.Join(h.cwd, ".git", "config"), "[core\nbroken")

	r := h.run("list")
	r.mustCode(t, 1)
	if !strings.HasPrefix(r.stderr, "wt: ") || !strings.Contains(r.stderr, "bad config") {
		t.Errorf("stderr = %q, want git's own message", r.stderr)
	}
	if lines := strings.Split(strings.TrimRight(r.stderr, "\n"), "\n"); len(lines) != 1 {
		t.Errorf("stderr has %d lines, want the message on one line: %q", len(lines), r.stderr)
	}
}

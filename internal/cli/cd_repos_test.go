package cli_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// cdWorkspace creates under <sandbox>/root the repositories api, web,
// api-gateway, TrendFisher, cv-scorer-backend and cv-scorer-front, sets
// repos_root to it, and starts outside any repository.
func cdWorkspace(h *harness) (root string) {
	h.t.Helper()
	root = h.mkdir(h.sb.Path("root"))
	for _, name := range []string{"api", "web", "api-gateway", "TrendFisher", "cv-scorer-backend", "cv-scorer-front"} {
		h.sb.InitRepo(filepath.Join(root, name))
	}
	h.roots(0, root)
	return root
}

// mustCd runs wt cd target and checks that the directive names want and
// nothing is written to the terminal.
func mustCd(t *testing.T, h *harness, target, want string) {
	t.Helper()
	d := h.activate()
	r := h.run("cd", target)
	r.mustCode(t, 0)
	if r.stdout != "" || r.stderr != "" {
		t.Errorf("wt cd %s: stdout %q, stderr %q; want both empty", target, r.stdout, r.stderr)
	}
	mustDirective(t, d, want)
}

// Scenarios of "Resolving a name" that end in a repository.
func TestCdToARepository(t *testing.T) {
	h := newHarness(t)
	root := cdWorkspace(h)
	for _, tc := range []struct {
		name, target, want string
	}{
		{"repository from outside any repository", "web", "web"},
		{"exact name before prefix", "api", "api"},
		{"letter case ignored", "trendfisher", "TrendFisher"},
		{"unique prefix", "trend", "TrendFisher"},
		{"unique prefix in upper case", "TREND", "TrendFisher"},
		{"unique prefix of a longer name", "api-", "api-gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustCd(t, h, tc.target, filepath.Join(root, tc.want))
		})
	}

	t.Run("repository from inside another", func(t *testing.T) {
		h.cwd = h.mkdir(root, "api", "src")
		mustCd(t, h, "web", filepath.Join(root, "web"))
	})
	t.Run("repository from a linked worktree of another", func(t *testing.T) {
		feat := h.sb.Path("x", "feat")
		h.sb.AddWorktree(filepath.Join(root, "api"), feat, "feat")
		h.cwd = feat
		mustCd(t, h, "web", filepath.Join(root, "web"))
		// The main worktree is still found by its name, in the repository.
		mustCd(t, h, "api", filepath.Join(root, "api"))
	})
}

func TestCdExactCaseBeforeIgnoringCase(t *testing.T) {
	h := newHarness(t)
	one, two := h.mkdir(h.sb.Path("one")), h.mkdir(h.sb.Path("two"))
	h.sb.InitRepo(filepath.Join(one, "Api"))
	h.sb.InitRepo(filepath.Join(two, "api"))
	h.roots(0, one, two)
	mustCd(t, h, "api", filepath.Join(two, "api"))
	mustCd(t, h, "Api", filepath.Join(one, "Api"))
}

// The destination of a repository with the .bare layout is its container,
// as wt repos lists it.
func TestCdToADotBareRepository(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	proj := filepath.Join(root, "proj")
	h.sb.InitDotBare(proj)
	h.roots(0, root)
	mustCd(t, h, "proj", proj)
}

// Scenarios "Current repository first" and "Local ambiguity is not resolved
// in the workspace": a name that resolves in the current repository never
// reaches the workspace, which a missing root would warn about.
func TestCdCurrentRepositoryFirst(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	web := filepath.Join(root, "web")
	h.sb.InitRepo(web)
	h.sb.InitRepo(filepath.Join(root, "x"))
	api := h.sb.Path("api")
	h.sb.InitRepo(api)
	localWeb := h.sb.Path("api.worktrees", "web")
	h.sb.AddWorktree(api, localWeb, "feature/web")
	a, b := h.sb.Path("a", "x"), h.sb.Path("b", "x")
	h.sb.AddWorktree(api, a, "feature/x")
	h.sb.AddWorktree(api, b, "fix/x")
	missing := h.sb.Path("usb", "repos")
	h.roots(0, missing, root)
	h.cwd = api

	mustCd(t, h, "web", localWeb)
	mustCd(t, h, "feature/web", localWeb)

	d := h.activate()
	r := h.run("cd", "x")
	r.mustCode(t, 4)
	if got := firstLine(r.stderr); got != `wt: "x" matches more than one worktree` {
		t.Errorf("first line = %q", got)
	}
	if strings.Contains(r.stderr, "warning") || readDirective(t, d) != "" {
		t.Errorf("stderr = %q, directive %q; want no warning and no directive", r.stderr, readDirective(t, d))
	}

	// A name that falls to the workspace walks it, and warns about the
	// missing root.
	r = h.run("cd", "--dry-run", "web-nope")
	r.mustCode(t, 3)
	if !strings.Contains(r.stderr, "wt: warning: repository root not found: "+missing) {
		t.Errorf("stderr = %q, want the warning about %s", r.stderr, missing)
	}
}

// Scenarios "Ambiguous prefix", "Two repositories with the same name" and
// "Ambiguity as JSON".
func TestCdRepositoryAmbiguity(t *testing.T) {
	h := newHarness(t)
	root := cdWorkspace(h)
	backend, front := filepath.Join(root, "cv-scorer-backend"), filepath.Join(root, "cv-scorer-front")

	d := h.activate()
	r := h.run("cd", "cv")
	r.mustCode(t, 4)
	if got := firstLine(r.stderr); got != `wt: "cv" matches more than one repository` {
		t.Errorf("first line = %q", got)
	}
	for _, p := range []string{backend, front} {
		if !strings.Contains(r.stderr, "\nhint: "+p+"\n") {
			t.Errorf("stderr has no hint with %s:\n%s", p, r.stderr)
		}
	}
	if r.stdout != "" || readDirective(t, d) != "" {
		t.Errorf("stdout %q, directive %q; want both empty", r.stdout, readDirective(t, d))
	}

	r = h.run("cd", "cv", "--json")
	r.mustCode(t, 4)
	doc := decodeOne(t, r.stderr)
	if r.stdout != "" || doc["schema"] != "wt.error.v1" || doc["code"] != 4.0 ||
		!reflect.DeepEqual(doc["hints"], []any{backend, front}) {
		t.Errorf("--json: stdout %q, error %v", r.stdout, doc)
	}

	t.Run("same name", func(t *testing.T) {
		other := h.mkdir(h.sb.Path("other"))
		h.sb.InitRepo(filepath.Join(other, "work", "api"))
		h.sb.InitRepo(filepath.Join(other, "oss", "api"))
		h.roots(2, other)
		r := h.run("cd", "api")
		r.mustCode(t, 4)
		if got := firstLine(r.stderr); got != `wt: "api" matches more than one repository` {
			t.Errorf("first line = %q", got)
		}
	})
}

// Scenarios "No worktree or repository" and "No repository".
func TestCdNoRepository(t *testing.T) {
	h := newHarness(t)
	cdWorkspace(h)

	d := h.activate()
	r := h.run("cd", "nope")
	r.mustCode(t, 3)
	if got := firstLine(r.stderr); got != `wt: no repository named "nope"` {
		t.Errorf("outside: first line = %q", got)
	}
	if !strings.Contains(r.stderr, "\nhint: ") || !strings.Contains(r.stderr, "wt repos") || strings.Contains(r.stderr, "wt list") {
		t.Errorf("outside: stderr = %q, want only a hint naming wt repos", r.stderr)
	}
	if readDirective(t, d) != "" {
		t.Error("the directive was written")
	}

	h.cwd = h.repo()
	r = h.run("cd", "nope")
	r.mustCode(t, 3)
	if got := firstLine(r.stderr); got != `wt: no worktree or repository named "nope"` {
		t.Errorf("inside: first line = %q", got)
	}
	for _, want := range []string{"hint: run 'wt list'", "hint: run 'wt repos'"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("inside: stderr = %q, want %q", r.stderr, want)
		}
	}

	// An empty name matches no repository.
	h.run("cd", "").mustCode(t, 3)
}

func TestCdToARepositoryDryRunAndJSON(t *testing.T) {
	h := newHarness(t)
	root := cdWorkspace(h)
	want := filepath.Join(root, "TrendFisher")

	d := h.activate()
	r := h.run("cd", "--dry-run", "trend")
	r.mustCode(t, 0)
	if r.stdout != "would change directory to "+want+"\n" || readDirective(t, d) != "" {
		t.Errorf("--dry-run: stdout %q, directive %q", r.stdout, readDirective(t, d))
	}

	r = h.run("cd", "--json", "trend")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.cd.v1" || doc["path"] != want {
		t.Errorf("--json: %v, want path %s", doc, want)
	}
	if got := readDirective(t, d); testutil.Comparable(t, got) != testutil.Comparable(t, want) {
		t.Errorf("directive = %q, want %q", got, want)
	}
}

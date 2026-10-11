package cli_test

import (
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

var listAllColumns = append([]string{"REPO"}, listColumns...)

// listAll runs wt list --all-repos with args and returns its rows.
func (h *harness) listAll(args ...string) ([]row, result) {
	h.t.Helper()
	r := h.run(append([]string{"list", "--all-repos"}, args...)...)
	r.mustCode(h.t, 0)
	if r.stdout == "" {
		return nil, r
	}
	return parseColumns(h.t, r.stdout, listAllColumns), r
}

func column(rows []row, name string) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.cols[name])
	}
	return out
}

// Scenarios of "Listing every repository".
func TestListAllRepos(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)

	t.Run("two repositories", func(t *testing.T) {
		rows, r := h.listAll()
		if got, want := column(rows, "NAME"), []string{"api", "feat", "web"}; !reflect.DeepEqual(got, want) {
			t.Errorf("NAME = %q, want %q", got, want)
		}
		if got, want := column(rows, "REPO"), []string{"api", "api", "web"}; !reflect.DeepEqual(got, want) {
			t.Errorf("REPO = %q, want %q", got, want)
		}
		if got, want := []string{rows[0].markers, rows[1].markers, rows[2].markers}, []string{"^", "", "^"}; !reflect.DeepEqual(got, want) {
			t.Errorf("markers = %q, want %q", got, want)
		}
		if p := rows[1].cols["PATH"]; p != testutil.Comparable(t, feat) {
			t.Errorf("PATH of feat = %q", p)
		}
		if r.stderr != "" {
			t.Errorf("stderr = %q", r.stderr)
		}
		dry := h.run("list", "--all-repos", "--dry-run")
		dry.mustCode(t, 0)
		if dry.stdout != r.stdout {
			t.Errorf("--dry-run = %q, want %q", dry.stdout, r.stdout)
		}
	})
	t.Run("current worktree", func(t *testing.T) {
		h.cwd = filepath.Join(feat, "sub")
		h.mkdir(h.cwd)
		rows, _ := h.listAll()
		var current []string
		for _, r := range rows {
			if strings.Contains(r.markers, "@") {
				current = append(current, r.cols["NAME"])
			}
		}
		if !reflect.DeepEqual(current, []string{"feat"}) || rows[1].markers != "@" {
			t.Errorf("current rows %q, markers of feat %q", current, rows[1].markers)
		}
		h.cwd = h.sb.Path("outside")
	})
	t.Run("aligned with color", func(t *testing.T) {
		plain := h.run("list", "--all-repos")
		h.tty = true
		colored := h.run("list", "--all-repos")
		h.tty = false
		if !strings.Contains(colored.stdout, "\x1b[") {
			t.Fatalf("no color in %q", colored.stdout)
		}
		if got := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(colored.stdout, ""); got != plain.stdout {
			t.Errorf("without its colors, the table is\n%s\nwant\n%s", got, plain.stdout)
		}
	})
	t.Run("repository outside the roots", func(t *testing.T) {
		elsewhere := h.sb.Path("elsewhere")
		h.sb.InitRepo(elsewhere)
		h.cwd = elsewhere
		rows, _ := h.listAll()
		for _, r := range rows {
			if r.cols["NAME"] == "elsewhere" || r.markers != "^" && r.markers != "" {
				t.Errorf("row %v", r)
			}
		}
		h.cwd = h.sb.Path("outside")
	})
	t.Run("repository that git cannot read", func(t *testing.T) {
		broken := filepath.Join(root, "broken")
		h.sb.InitRepo(broken)
		h.sb.BreakHEAD(broken)
		rows, r := h.listAll()
		if got, want := column(rows, "REPO"), []string{"api", "api", "web"}; !reflect.DeepEqual(got, want) {
			t.Errorf("REPO = %q, want %q", got, want)
		}
		if !strings.Contains(r.stderr, "wt: warning: cannot read the worktrees of broken: ") {
			t.Errorf("stderr = %q, want a warning naming broken", r.stderr)
		}

		r = h.run("list", "--all-repos", "--json")
		r.mustCode(t, 0)
		if r.stderr != "" {
			t.Errorf("--json: stderr = %q", r.stderr)
		}
		doc := decodeOne(t, r.stdout)
		unreadable, _ := doc["unreadable"].([]any)
		if len(unreadable) != 1 {
			t.Fatalf("unreadable = %v, want one repository", doc["unreadable"])
		}
		u, _ := unreadable[0].(map[string]any)
		if msg, _ := u["message"].(string); u["repo"] != "broken" || u["path"] != broken || !strings.Contains(msg, "not a git repository") {
			t.Errorf("unreadable[0] = %v", u)
		}
	})
}

func TestListAllReposSameName(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	h.sb.InitRepo(filepath.Join(root, "work", "api"))
	h.sb.InitRepo(filepath.Join(root, "oss", "api"))
	h.roots(2, root)
	rows, _ := h.listAll()
	if got, want := column(rows, "REPO"), []string{"oss/api", "work/api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("REPO = %q, want %q", got, want)
	}
}

func TestListAllReposErrors(t *testing.T) {
	h := newHarness(t)

	r := h.run("list", "--all-repos")
	r.mustCode(t, 1)
	if got := firstLine(r.stderr); got != "wt: no repository roots configured" {
		t.Errorf("first line = %q", got)
	}

	empty := h.mkdir(h.sb.Path("empty"))
	h.roots(0, empty)
	if r := h.run("list", "--all-repos"); r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Errorf("no repositories: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	r = h.run("list", "--all-repos", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if !reflect.DeepEqual(doc["worktrees"], []any{}) || !reflect.DeepEqual(doc["unreadable"], []any{}) {
		t.Errorf("no repositories as JSON: %v", doc)
	}

	// Scenario "Outside a repository with roots configured".
	r = h.run("list")
	r.mustCode(t, 3)
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
	mustHint(t, r, "wt list --all-repos")

	h.userConfig("")
	if r := h.run("list"); strings.Contains(r.stderr, "hint:") {
		t.Errorf("without roots: stderr = %q, want no hint", r.stderr)
	}
}

// Scenarios of "JSON output of every repository".
func TestListAllReposJSON(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)

	r := h.run("list", "--all-repos", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.list.v1" || !reflect.DeepEqual(doc["unreadable"], []any{}) {
		t.Errorf("document = %v", doc)
	}
	ws, _ := doc["worktrees"].([]any)
	var names []string
	for _, w := range ws {
		m, _ := w.(map[string]any)
		names = append(names, m["name"].(string))
		if m["name"] != "feat" {
			continue
		}
		if m["repo"] != "api" || m["repo_path"] != filepath.Join(root, "api") || m["path"] != testutil.Comparable(t, feat) {
			t.Errorf("element of feat = %v", m)
		}
		for _, field := range []string{"branch", "head", "main", "current", "locked", "prunable"} {
			if _, ok := m[field]; !ok {
				t.Errorf("element of feat has no %s", field)
			}
		}
	}
	if !reflect.DeepEqual(names, []string{"api", "feat", "web"}) {
		t.Errorf("worktrees %q, want the order of the table", names)
	}

	// wt list --json is the document it always was.
	h.cwd = filepath.Join(root, "api")
	r = h.run("list", "--json")
	r.mustCode(t, 0)
	doc = decodeOne(t, r.stdout)
	if _, ok := doc["unreadable"]; ok || len(doc) != 2 {
		t.Errorf("wt list --json = %v", doc)
	}
	for _, w := range doc["worktrees"].([]any) {
		m := w.(map[string]any)
		if _, ok := m["repo"]; ok || len(m) != 12 {
			t.Errorf("element %v has fields of --all-repos", m)
		}
	}
}

func TestListHelpNamesAllRepos(t *testing.T) {
	r := newHarness(t).run("list", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"--all-repos", "REPO", "repos_root"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not contain %q:\n%s", want, r.stdout)
		}
	}
}

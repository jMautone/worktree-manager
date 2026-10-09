package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// roots writes repos_root to the user configuration file and, when depth is
// not 0, repos_depth.
func (h *harness) roots(depth int, roots ...string) {
	h.t.Helper()
	quoted := make([]string, len(roots))
	for i, r := range roots {
		quoted[i] = strconv.Quote(r)
	}
	content := "repos_root = [" + strings.Join(quoted, ", ") + "]\n"
	if depth != 0 {
		content += fmt.Sprintf("repos_depth = %d\n", depth)
	}
	h.userConfig(content)
}

// mkdir creates dir and its parents.
func (h *harness) mkdir(elem ...string) string {
	h.t.Helper()
	dir := filepath.Join(elem...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	return dir
}

// reposTable runs `wt repos` and returns its rows.
func (h *harness) reposTable(args ...string) []row {
	h.t.Helper()
	r := h.run(append(args, "repos")...)
	r.mustCode(h.t, 0)
	if r.stdout == "" {
		return nil
	}
	return parseColumns(h.t, r.stdout, []string{"NAME", "PATH"})
}

// rowPaths returns the PATH of every row, in order.
func rowPaths(rows []row) []string {
	ps := []string{}
	for _, r := range rows {
		ps = append(ps, r.cols["PATH"])
	}
	return ps
}

func mustPaths(t *testing.T, rows []row, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := rowPaths(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("wt repos lists %q, want %q", got, want)
	}
	for _, r := range rows {
		if r.cols["NAME"] != filepath.Base(r.cols["PATH"]) {
			t.Errorf("row %v: NAME is not the last component of PATH", r.cols)
		}
	}
}

// marked returns the PATH of the rows marked with @.
func marked(rows []row) []string {
	var ps []string
	for _, r := range rows {
		if r.markers == "@" {
			ps = append(ps, r.cols["PATH"])
		}
	}
	return ps
}

func TestReposNothingConfigured(t *testing.T) {
	h := newHarness(t)
	r := h.run("repos")
	r.mustCode(t, 1)
	if got := firstLine(r.stderr); got != "wt: no repository roots configured" {
		t.Errorf("first line = %q", got)
	}
	if !regexp.MustCompile(`(?m)^hint: .*repos_root.*` + regexp.QuoteMeta(h.userConfigPath())).MatchString(r.stderr) {
		t.Errorf("stderr has no hint naming repos_root and %s:\n%s", h.userConfigPath(), r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q", r.stdout)
	}

	r = h.run("repos", "--json")
	r.mustCode(t, 1)
	doc := decodeOne(t, r.stderr)
	if r.stdout != "" || doc["schema"] != "wt.error.v1" || doc["code"] != 1.0 {
		t.Errorf("--json: stdout %q, error %v", r.stdout, doc)
	}

	// An empty WT_REPOS_ROOT is unset, and one with only separators is an
	// empty list.
	h.sb.Setenv("WT_REPOS_ROOT", string(os.PathListSeparator))
	h.run("repos").mustCode(t, 1)
}

func TestReposArgumentsAndHelp(t *testing.T) {
	h := newHarness(t)
	r := h.run("repos", "api")
	r.mustCode(t, 2)
	if !strings.Contains(r.stderr, `unexpected argument "api"`) || r.stdout != "" {
		t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
	}

	r = h.run("repos", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"wt repos", "repos_root", "repos_depth", "WT_REPOS_ROOT", "wt cd"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("wt repos -h does not contain %q:\n%s", want, r.stdout)
		}
	}
	if r := h.run("-h"); !regexp.MustCompile(`(?m)^\s+repos\s`).MatchString(r.stdout) {
		t.Errorf("wt -h does not list repos:\n%s", r.stdout)
	}
}

// Scenarios of "What counts as a repository", plus "Hidden directory".
func TestReposKindsOfRepository(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	api := filepath.Join(root, "api")
	h.sb.InitRepo(api)
	h.sb.AddWorktree(api, filepath.Join(root, "api.worktrees", "feat"), "feat")
	proj := filepath.Join(root, "proj")
	h.sb.InitDotBare(proj)
	h.sb.InitBare(filepath.Join(root, "api.git"))
	h.sb.WriteFile(filepath.Join(root, "notes", "todo.txt"), "x")
	dotfiles := filepath.Join(root, ".dotfiles")
	h.sb.InitRepo(dotfiles)
	h.roots(2, root)

	mustPaths(t, h.reposTable(), dotfiles, api, proj)
}

// Scenarios of "Search depth".
func TestReposDepth(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	api := filepath.Join(root, "org", "api")
	h.sb.InitRepo(api)
	outer := filepath.Join(root, "outer")
	h.sb.InitRepo(outer)
	h.sb.InitRepo(filepath.Join(outer, "vendor", "lib"))

	h.roots(0, root)
	mustPaths(t, h.reposTable(), outer)
	h.roots(2, root)
	mustPaths(t, h.reposTable(), api, outer)
	h.roots(3, root)
	mustPaths(t, h.reposTable(), api, outer)

	h.userConfig("")
	h.sb.Setenv("WT_REPOS_ROOT", root)
	h.sb.Setenv("WT_REPOS_DEPTH", "2")
	mustPaths(t, h.reposTable(), api, outer)
}

func TestReposDepthNotAnInteger(t *testing.T) {
	h := newHarness(t)
	h.roots(0, h.mkdir(h.sb.Path("root")))
	h.sb.Setenv("WT_REPOS_DEPTH", "two")
	r := h.run("repos")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "WT_REPOS_DEPTH") || r.stdout != "" {
		t.Errorf("stdout %q, stderr %q; want an error naming WT_REPOS_DEPTH", r.stdout, r.stderr)
	}
}

// Scenarios of "Repository roots" and "Same repository reached twice" that
// work on every OS.
func TestReposRoots(t *testing.T) {
	h := newHarness(t)
	dir := h.mkdir(h.sb.Path("dir"))
	api := filepath.Join(dir, "work", "api")
	h.sb.InitRepo(api)
	missing := h.sb.Path("usb", "repos")

	t.Run("missing root", func(t *testing.T) {
		h.roots(0, missing, filepath.Join(dir, "work"))
		r := h.run("repos")
		r.mustCode(t, 0)
		if !strings.Contains(r.stderr, "wt: warning: ") || !strings.Contains(r.stderr, missing) {
			t.Errorf("stderr = %q, want a warning naming %s", r.stderr, missing)
		}
		mustPaths(t, parseColumns(t, r.stdout, []string{"NAME", "PATH"}), api)

		r = h.run("repos", "--json")
		r.mustCode(t, 0)
		if r.stderr != "" {
			t.Errorf("--json: stderr = %q, want empty", r.stderr)
		}
	})
	t.Run("overlapping roots", func(t *testing.T) {
		h.roots(2, filepath.Join(dir, "work"), dir)
		mustPaths(t, h.reposTable(), api)
		doc := decodeOne(t, h.run("repos", "--json").stdout)
		if repos, _ := doc["repos"].([]any); len(repos) != 1 || repos[0].(map[string]any)["root"] != filepath.Join(dir, "work") {
			t.Errorf("document = %v, want api under the root %s", doc, filepath.Join(dir, "work"))
		}
	})
	t.Run("root given twice", func(t *testing.T) {
		h.roots(2, dir, dir)
		mustPaths(t, h.reposTable(), api)
	})
	t.Run("no repositories found", func(t *testing.T) {
		empty := h.mkdir(h.sb.Path("empty"))
		h.roots(0, empty)
		r := h.run("repos")
		r.mustCode(t, 0)
		if r.stdout != "" || r.stderr != "" {
			t.Errorf("stdout %q, stderr %q; want both empty", r.stdout, r.stderr)
		}
		doc := decodeOne(t, h.run("repos", "--json").stdout)
		if repos, ok := doc["repos"].([]any); !ok || len(repos) != 0 || doc["schema"] != "wt.repos.v1" {
			t.Errorf("--json: %v, want an empty repos array", doc)
		}
	})
}

func TestReposHomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on Windows ~ is USERPROFILE; see TestReposOnWindows")
	}
	h := newHarness(t)
	home := h.sb.Getenv("HOME")
	api := filepath.Join(home, "GIT", "api")
	h.sb.InitRepo(api)
	h.roots(0, "~/GIT")
	mustPaths(t, h.reposTable(), api)

	// Scenario "Repositories as JSON (macOS, Linux)", from inside api.
	h.cwd = api
	r := h.run("repos", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	repos, _ := doc["repos"].([]any)
	if doc["schema"] != "wt.repos.v1" || len(repos) != 1 || len(doc) != 2 {
		t.Fatalf("document = %v", doc)
	}
	want := map[string]any{"name": "api", "path": api, "root": filepath.Join(home, "GIT"), "current": true}
	if !reflect.DeepEqual(repos[0], want) {
		t.Errorf("repos[0] = %v, want %v", repos[0], want)
	}
}

// Scenarios "Same name, different repositories" and "Order ignores case".
func TestReposNamesAndOrder(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	work, oss := filepath.Join(root, "work", "api"), filepath.Join(root, "oss", "api")
	h.sb.InitRepo(work)
	h.sb.InitRepo(oss)
	h.roots(2, root)
	mustPaths(t, h.reposTable(), oss, work)

	other := h.mkdir(h.sb.Path("other"))
	var want []string
	for _, name := range []string{"zeta", "Alpha", "beta"} {
		h.sb.InitRepo(filepath.Join(other, name))
	}
	for _, name := range []string{"Alpha", "beta", "zeta"} {
		want = append(want, filepath.Join(other, name))
	}
	h.roots(0, other)
	mustPaths(t, h.reposTable(), want...)
}

// Scenarios of "Listing repositories" about the @ marker.
func TestReposMarker(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	api := filepath.Join(root, "api")
	h.sb.InitRepo(api)
	web := filepath.Join(root, "web")
	h.sb.InitRepo(web)
	proj := filepath.Join(root, "proj")
	h.sb.InitDotBare(proj)
	feat := h.sb.Path("x", "feat")
	h.sb.AddWorktree(api, feat, "feat")
	projFeat := h.sb.Path("x", "proj-feat")
	h.sb.AddWorktree(proj, projFeat, "proj-feat")
	h.roots(0, root)

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
		want []string
	}{
		{"from a linked worktree", h.mkdir(feat, "src"), nil, []string{api}},
		{"from the main worktree", h.mkdir(api, "sub"), nil, []string{api}},
		{"from the repository directory", filepath.Join(api, ".git"), nil, []string{api}},
		{"outside any repository", h.sb.Path("outside"), nil, nil},
		{"working directory override", api, []string{"-C", web}, []string{web}},
		{"from the container of a .bare", proj, nil, []string{proj}},
		{"from a linked worktree of a .bare", projFeat, nil, []string{proj}},
		{"from a repository not under the roots", h.repo(), nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.cwd = tc.cwd
			rows := h.reposTable(tc.args...)
			mustPaths(t, rows, api, proj, web)
			if got := marked(rows); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("marked rows = %q, want %q", got, tc.want)
			}

			// The JSON document marks the same row.
			doc := decodeOne(t, h.run(append(tc.args, "repos", "--json")...).stdout)
			var current []string
			for _, e := range doc["repos"].([]any) {
				if e := e.(map[string]any); e["current"] == true {
					current = append(current, e["path"].(string))
				}
			}
			if !reflect.DeepEqual(current, tc.want) {
				t.Errorf("--json: current = %q, want %q", current, tc.want)
			}
		})
	}
}

func TestReposDryRunAndColor(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	api := filepath.Join(root, "api")
	h.sb.InitRepo(api)
	h.roots(0, root)
	h.cwd = api

	for _, json := range []bool{false, true} {
		args := []string{"repos"}
		if json {
			args = append(args, "--json")
		}
		want := h.run(args...)
		got := h.run(append(args, "--dry-run")...)
		got.mustCode(t, 0)
		if got.stdout != want.stdout || got.stderr != want.stderr {
			t.Errorf("wt %v --dry-run = %q, want %q", args, got.stdout, want.stdout)
		}
	}

	h.tty = true
	r := h.run("repos")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "\x1b[1;32m@\x1b[0m") || !strings.Contains(r.stdout, "\x1b[1mNAME\x1b[0m") {
		t.Errorf("colored table = %q", r.stdout)
	}
	if strings.Contains(h.run("repos", "--json").stdout, "\x1b[") {
		t.Error("the JSON document has color")
	}
}

// Scenarios "Unreadable directory", "Linked repository" and "Link to a
// repository already found", for macOS and Linux.
func TestReposLinksAndPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows, and permissions differ; see TestReposOnWindows")
	}
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	api := filepath.Join(root, "api")
	h.sb.InitRepo(api)
	frontend := h.sb.Path("elsewhere", "frontend")
	h.sb.InitRepo(frontend)
	web := filepath.Join(root, "web")
	h.sb.Symlink(frontend, web)
	h.sb.Symlink(api, filepath.Join(root, "zz-api"))
	h.roots(2, root)
	mustPaths(t, h.reposTable(), api, web)

	// From the linked repository, its row is marked.
	h.cwd = web
	if got := marked(h.reposTable()); !reflect.DeepEqual(got, []string{web}) {
		t.Errorf("marked from %s = %q", web, got)
	}

	t.Run("unreadable directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		locked := filepath.Dir(h.mkdir(root, "locked", "inner"))
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		h.cwd = h.sb.Path("outside")
		r := h.run("repos")
		r.mustCode(t, 0)
		if r.stderr != "" {
			t.Errorf("stderr = %q, want empty", r.stderr)
		}
		mustPaths(t, parseColumns(t, r.stdout, []string{"NAME", "PATH"}), api, web)
	})
}

// Scenarios of workspace-discovery for Windows: ~\, native paths in the
// table and in JSON, and directory junctions.
func TestReposOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("USERPROFILE, drive letters and junctions are Windows'")
	}
	h := newHarness(t)

	t.Run("home directory", func(t *testing.T) {
		api := filepath.Join(h.sb.Getenv("USERPROFILE"), "GIT", "api")
		h.sb.InitRepo(api)
		h.roots(0, `~\GIT`)
		mustPaths(t, h.reposTable(), api)
	})

	root := h.mkdir(h.sb.Path("Repos"))
	api := filepath.Join(root, "api")
	h.sb.InitRepo(api)
	frontend := h.sb.Path("elsewhere", "frontend")
	h.sb.InitRepo(frontend)
	web := filepath.Join(root, "web")
	h.sb.Junction(frontend, web)
	h.sb.Junction(api, filepath.Join(root, "zz-api"))

	t.Run("native path, junctions", func(t *testing.T) {
		h.roots(0, filepath.ToSlash(root))
		rows := h.reposTable()
		mustPaths(t, rows, api, web)
		for _, p := range rowPaths(rows) {
			if strings.Contains(p, "/") {
				t.Errorf("path %q is not native", p)
			}
		}
	})
	t.Run("JSON", func(t *testing.T) {
		h.roots(0, root)
		h.cwd = web
		doc := decodeOne(t, h.run("repos", "--json").stdout)
		repos, _ := doc["repos"].([]any)
		want := []any{
			map[string]any{"name": "api", "path": api, "root": root, "current": false},
			map[string]any{"name": "web", "path": web, "root": root, "current": true},
		}
		if !reflect.DeepEqual(repos, want) {
			t.Errorf("repos = %v, want %v", repos, want)
		}
	})
}

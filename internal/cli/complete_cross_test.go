package cli_test

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// completeDesc runs cobra's __complete protocol and returns each offered
// value with its description, and the directive line. It fails the test
// when anything is written to stderr or the exit code is not 0.
func (h *harness) completeDesc(args ...string) (offered map[string]string, order []string, directive string) {
	h.t.Helper()
	r := h.run(append([]string{"__complete"}, args...)...)
	r.mustCode(h.t, 0)
	if r.stderr != "" {
		h.t.Errorf("wt %v: stderr = %q, want nothing written", args, r.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	directive = lines[len(lines)-1]
	offered = map[string]string{}
	for _, l := range lines[:len(lines)-1] {
		value, desc, _ := strings.Cut(l, "\t")
		if _, ok := offered[value]; ok {
			h.t.Errorf("wt %v: %s offered twice", args, value)
		}
		offered[value] = desc
		order = append(order, value)
	}
	return offered, order, directive
}

// cortexWorkspace creates under <sandbox>/root cortex, with the linked
// worktrees angry-jang-12fc4f and sweet-roentgen-4a36e6, and web; sets
// repos_root to the root; and starts outside any repository.
func cortexWorkspace(h *harness) (root string) {
	h.t.Helper()
	root = h.mkdir(h.sb.Path("root"))
	cortex := filepath.Join(root, "cortex")
	h.sb.InitRepo(cortex)
	h.sb.InitRepo(filepath.Join(root, "web"))
	h.sb.AddWorktree(cortex, filepath.Join(root, "cortex.worktrees", "angry-jang-12fc4f"), "angry")
	h.sb.AddWorktree(cortex, filepath.Join(root, "cortex.worktrees", "sweet-roentgen-4a36e6"), "sweet")
	h.roots(0, root)
	return root
}

// Scenarios of "Completions" of shell-integration with worktrees of other
// repositories.
func TestCompletionsAcrossRepositories(t *testing.T) {
	h := newHarness(t)
	root := cortexWorkspace(h)
	web := filepath.Join(root, "web")

	t.Run("worktrees of other repositories", func(t *testing.T) {
		h.cwd = web
		got, order, directive := h.completeDesc("cd", "")
		want := map[string]string{
			"web":                   "worktree",
			"cortex":                "repository",
			"angry-jang-12fc4f":     "worktree of cortex",
			"sweet-roentgen-4a36e6": "worktree of cortex",
		}
		if !maps.Equal(got, want) || directive != ":4" {
			t.Errorf("offered %q (directive %s), want %q", got, directive, want)
		}
		// The order is the order Enter tries them in.
		if wantOrder := []string{"web", "cortex", "angry-jang-12fc4f", "sweet-roentgen-4a36e6"}; !slices.Equal(order, wantOrder) {
			t.Errorf("order %q, want %q", order, wantOrder)
		}
		if got, _, _ := h.completeDesc("cd", "an"); !maps.Equal(got, map[string]string{"angry-jang-12fc4f": "worktree of cortex"}) {
			t.Errorf("wt cd an: offered %q", got)
		}
		h.cwd = h.sb.Path("outside")
	})
	t.Run("targets of path", func(t *testing.T) {
		got, _, _ := h.completeDesc("path", "")
		for _, name := range []string{"cortex", "web", "angry-jang-12fc4f", "sweet-roentgen-4a36e6"} {
			if _, ok := got[name]; !ok {
				t.Errorf("wt path: %s not offered in %q", name, got)
			}
		}
	})
	t.Run("qualified target", func(t *testing.T) {
		_, order, directive := h.completeDesc("cd", "cortex:")
		if want := []string{"cortex:angry-jang-12fc4f", "cortex:sweet-roentgen-4a36e6"}; !slices.Equal(order, want) || directive != ":4" {
			t.Errorf("offered %q (directive %s), want %q", order, directive, want)
		}
		if _, order, _ := h.completeDesc("cd", "cortex:s"); !slices.Equal(order, []string{"cortex:sweet-roentgen-4a36e6"}) {
			t.Errorf("wt cd cortex:s: offered %q", order)
		}
	})
	t.Run("repository side as typed", func(t *testing.T) {
		if _, order, _ := h.completeDesc("path", "co:"); !slices.Equal(order, []string{"co:angry-jang-12fc4f", "co:sweet-roentgen-4a36e6"}) {
			t.Errorf("offered %q", order)
		}
	})
	t.Run("ambiguous repository side", func(t *testing.T) {
		h.sb.InitRepo(filepath.Join(root, "cv-scorer-backend"))
		h.sb.InitRepo(filepath.Join(root, "cv-scorer-front"))
		if got, _, directive := h.completeDesc("cd", "cv:"); len(got) != 0 || directive != ":4" {
			t.Errorf("offered %q (directive %s)", got, directive)
		}
		if got, _, _ := h.completeDesc("cd", "nope:"); len(got) != 0 {
			t.Errorf("wt cd nope: offered %q", got)
		}
	})
	t.Run("repository that git cannot read", func(t *testing.T) {
		broken := filepath.Join(root, "broken")
		h.sb.InitRepo(broken)
		h.sb.BreakHEAD(broken)
		got, _, _ := h.completeDesc("cd", "")
		for _, name := range []string{"broken", "cortex", "angry-jang-12fc4f"} {
			if _, ok := got[name]; !ok {
				t.Errorf("%s not offered in %q", name, got)
			}
		}
		if got, _, _ := h.completeDesc("cd", "broken:"); len(got) != 0 {
			t.Errorf("wt cd broken: offered %q", got)
		}
	})
	t.Run("no roots", func(t *testing.T) {
		h.userConfig("")
		if got, _, _ := h.completeDesc("cd", "cortex:"); len(got) != 0 {
			t.Errorf("offered %q", got)
		}
		h.roots(0, root)
	})
}

func TestCompletionsSameWorktreeNameInTwoRepositories(t *testing.T) {
	h := newHarness(t)
	root, _ := crossWorkspace(h)
	h.sb.AddWorktree(filepath.Join(root, "web"), filepath.Join(root, "web.worktrees", "feat"), "web-feat")
	got, _, _ := h.completeDesc("cd", "f")
	if !maps.Equal(got, map[string]string{"feat": "worktree of api"}) {
		t.Errorf("offered %q, want feat once", got)
	}
	// From inside web, its own feat comes first.
	h.cwd = filepath.Join(root, "web")
	if got, _, _ := h.completeDesc("cd", "f"); !maps.Equal(got, map[string]string{"feat": "worktree"}) {
		t.Errorf("from web: offered %q", got)
	}
}

// Scenarios of "Completions" of remove-worktree.
func TestRemoveCompletionsAcrossRepositories(t *testing.T) {
	h := newHarness(t)
	root, _ := crossWorkspace(h)
	api := filepath.Join(root, "api")
	usb := filepath.Join(root, "api.worktrees", "usb")
	h.sb.AddWorktree(api, usb, "usb")
	h.sb.LockWorktree(api, usb, "")
	gone := filepath.Join(root, "api.worktrees", "gone")
	h.sb.AddWorktree(api, gone, "gone")
	h.sb.MakePrunable(gone)
	h.cwd = filepath.Join(root, "web")

	for _, tc := range []struct {
		args []string
		want map[string]string
	}{
		{[]string{"remove", ""}, map[string]string{"feat": "worktree of api", "gone": "worktree of api"}},
		{[]string{"lock", ""}, map[string]string{"feat": "worktree of api", "gone": "worktree of api"}},
		{[]string{"unlock", ""}, map[string]string{"usb": "worktree of api"}},
		{[]string{"remove", "api:"}, map[string]string{"api:feat": "", "api:gone": ""}},
		{[]string{"unlock", "api:"}, map[string]string{"api:usb": ""}},
		{[]string{"lock", "api:feat", ""}, map[string]string{}},
	} {
		got, _, directive := h.completeDesc(tc.args...)
		if !maps.Equal(got, tc.want) || directive != ":4" {
			t.Errorf("%q: offered %q (directive %s), want %q", tc.args, got, directive, tc.want)
		}
	}

	// From inside api, its worktrees are the current repository's.
	h.cwd = api
	if got, _, _ := h.completeDesc("remove", ""); !maps.Equal(got, map[string]string{"feat": "worktree", "gone": "worktree"}) {
		t.Errorf("from api: offered %q", got)
	}
}

// Scenarios of "Completions" of create-worktree.
func TestCreateCompletionsAcrossRepositories(t *testing.T) {
	h := newHarness(t)
	root, api, _ := createWorkspace(h)
	h.sb.Git(api, "branch", "develop")

	for _, tc := range []struct {
		args      []string
		want      []string
		directive string
	}{
		{[]string{"create", "a"}, []string{"api:"}, ":6"},
		{[]string{"create", "W"}, []string{"web:"}, ":6"},
		{[]string{"create", ""}, []string{"api:", "web:"}, ":6"},
		{[]string{"create", "api:"}, nil, ":4"},
		{[]string{"create", "api:feat", "--base", ""}, []string{"develop", "main", "origin/main"}, ":4"},
		{[]string{"create", "api:feat", "--base", "d"}, []string{"develop"}, ":4"},
		{[]string{"create", "feat", "--base", ""}, nil, ":4"},
	} {
		_, order, directive := h.completeDesc(tc.args...)
		if !slices.Equal(order, tc.want) || directive != tc.directive {
			t.Errorf("%q: offered %q (directive %s), want %q (%s)", tc.args, order, directive, tc.want, tc.directive)
		}
	}

	// Repositories with the same name are offered by their display name.
	h.sb.InitRepo(filepath.Join(root, "x", "api"))
	h.roots(2, root)
	if _, order, _ := h.completeDesc("create", ""); !slices.Equal(order, []string{"root/api:", "x/api:", "web:"}) {
		t.Errorf("offered %q", order)
	}

	h.userConfig("")
	if _, order, _ := h.completeDesc("create", ""); len(order) != 0 {
		t.Errorf("without roots: offered %q", order)
	}
}

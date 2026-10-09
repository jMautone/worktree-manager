package cli_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var shells = []string{"zsh", "bash", "fish", "pwsh"}

func TestShellInitPrintsTheScript(t *testing.T) {
	h := newHarness(t) // outside any repository
	for _, tc := range []struct {
		shell, function string
	}{
		{"zsh", "wt() {"},
		{"bash", "wt() {"},
		{"fish", "function wt "},
		{"pwsh", "function global:wt {"},
	} {
		r := h.run("shell", "init", tc.shell)
		r.mustCode(t, 0)
		if !strings.Contains(r.stdout, tc.function) || !strings.Contains(r.stdout, "git-wt") {
			t.Errorf("%s: the script does not define the function:\n%s", tc.shell, r.stdout)
		}
		if r.stderr != "" {
			t.Errorf("%s: stderr = %q", tc.shell, r.stderr)
		}
	}
}

func TestShellInitWithBrokenConfiguration(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.userConfig("default_base = \n")

	r := h.run("shell", "init", "bash")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "wt() {") || r.stderr != "" {
		t.Errorf("stdout:\n%s\nstderr: %q", r.stdout, r.stderr)
	}
	// The configuration really is broken for commands that read it.
	h.run("list").mustCode(t, 1)
}

func TestShellInitUsageErrors(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"shell", "init", "tcsh"}, append([]string{`"tcsh"`}, shells...)},
		{[]string{"shell", "init"}, []string{"<shell>"}},
		{[]string{"shell", "init", "zsh", "bash"}, []string{`unexpected argument "bash"`}},
		{[]string{"shell", "init", "ZSH"}, []string{`"ZSH"`}},
		{[]string{"shell", "bogus"}, []string{`unknown command "bogus"`}},
		{[]string{"shell", "init", "tcsh", "--json"}, []string{`"wt.error.v1"`, "tcsh"}},
	} {
		r := h.run(tc.args...)
		if r.code != 2 || r.stdout != "" {
			t.Errorf("wt %v: exit %d, stdout %q; want 2 and empty", tc.args, r.code, r.stdout)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.stderr, w) {
				t.Errorf("wt %v: stderr %q does not contain %q", tc.args, r.stderr, w)
			}
		}
	}
}

func TestShellInitHelpShowsTheInstallationLines(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"shell", "init", "--help"}, {"help", "shell", "init"}} {
		r := h.run(args...)
		r.mustCode(t, 0)
		for _, want := range []string{
			"~/.zshrc", `eval "$(git-wt shell init zsh)"`,
			"~/.bashrc", `eval "$(git-wt shell init bash)"`,
			"~/.config/fish/config.fish", "git-wt shell init fish | source",
			"$PROFILE", "Invoke-Expression (& git-wt shell init pwsh | Out-String)",
		} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("wt %v: help does not contain %q:\n%s", args, want, r.stdout)
			}
		}
	}
	if r := h.run("shell"); r.code != 0 || !strings.Contains(r.stdout, "init") {
		t.Errorf("wt shell: exit %d, stdout:\n%s", r.code, r.stdout)
	}
}

func TestShellInitJSON(t *testing.T) {
	h := newHarness(t)
	for _, sh := range shells {
		plain := h.run("shell", "init", sh)
		r := h.run("shell", "init", sh, "--json")
		r.mustCode(t, 0)
		doc := decodeOne(t, r.stdout)
		if doc["schema"] != "wt.shell.init.v1" || doc["shell"] != sh || len(doc) != 3 {
			t.Errorf("%s: document = %v", sh, doc)
		}
		if doc["script"] != plain.stdout {
			t.Errorf("%s: script differs from the output of wt shell init %s", sh, sh)
		}
	}
}

func TestShellInitIsTheSameEverywhere(t *testing.T) {
	h := newHarness(t)
	for _, sh := range shells {
		want := h.run("shell", "init", sh).stdout
		dry := h.run("shell", "init", sh, "--dry-run")
		dry.mustCode(t, 0)
		if dry.stdout != want {
			t.Errorf("%s: --dry-run changes the output", sh)
		}
		h.goos = "windows"
		win := h.run("shell", "init", sh)
		h.goos = ""
		win.mustCode(t, 0)
		if win.stdout != want {
			t.Errorf("%s: the script printed on Windows differs", sh)
		}
	}
}

// complete runs cobra's __complete protocol, as the shell scripts do, and
// returns the offered values and the directive line.
func (h *harness) complete(args ...string) (values []string, directive string, r result) {
	h.t.Helper()
	r = h.run(append([]string{"__complete"}, args...)...)
	r.mustCode(h.t, 0)
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	directive = lines[len(lines)-1]
	for _, l := range lines[:len(lines)-1] {
		value, _, _ := strings.Cut(l, "\t")
		values = append(values, value)
	}
	return values, directive, r
}

func TestCompletions(t *testing.T) {
	h := newHarness(t)
	repo, _ := cdRepo(h)
	h.sb.AddWorktree(repo, h.sb.Path("repo.worktrees", "fix"), "fix")
	gone := h.sb.Path("repo.worktrees", "gone")
	h.sb.AddWorktree(repo, gone, "gone")
	h.sb.MakePrunable(gone)
	// A warning while typing would land in the middle of the command line.
	h.userConfig("colour = \"blue\"\n")

	t.Run("worktree names", func(t *testing.T) {
		got, directive, r := h.complete("cd", "")
		if want := []string{"repo", "feat", "fix"}; !slices.Equal(got, want) {
			t.Errorf("wt cd: offered %q, want %q", got, want)
		}
		if directive != ":4" {
			t.Errorf("directive = %q, want :4 (no file completion)", directive)
		}
		// In PowerShell the request goes through the wt function, so stderr
		// would reach the terminal.
		if r.stderr != "" {
			t.Errorf("stderr = %q, want nothing written", r.stderr)
		}
		if got, _, _ := h.complete("cd", "f"); !slices.Equal(got, []string{"feat", "fix"}) {
			t.Errorf("wt cd f: offered %q", got)
		}
		if got, _, _ := h.complete("cd", "feat", ""); len(got) != 0 {
			t.Errorf("second argument of wt cd: offered %q", got)
		}
	})
	t.Run("subcommands", func(t *testing.T) {
		if got, _, _ := h.complete("sh"); !slices.Equal(got, []string{"shell"}) {
			t.Errorf("wt sh: offered %q", got)
		}
		if got, _, _ := h.complete(""); !slices.Contains(got, "cd") || !slices.Contains(got, "list") {
			t.Errorf("wt: offered %q", got)
		}
		if got, _, _ := h.complete("list", "--j"); !slices.Equal(got, []string{"--json"}) {
			t.Errorf("wt list --j: offered %q", got)
		}
	})
	t.Run("shell names", func(t *testing.T) {
		if got, _, _ := h.complete("shell", "init", ""); !slices.Equal(got, shells) {
			t.Errorf("wt shell init: offered %q", got)
		}
	})
	t.Run("outside a repository", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		got, _, r := h.complete("cd", "")
		if len(got) != 0 || r.stdout != ":4\n" {
			t.Errorf("stdout = %q, want only the directive", r.stdout)
		}
	})
}

// Scenarios of "Completions" with repositories under the roots.
func TestCompletionsOfRepositories(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	for _, name := range []string{"api", "web", "TrendFisher", "feat"} {
		h.sb.InitRepo(filepath.Join(root, name))
	}
	h.roots(0, root)

	// complete checks that a completion writes nothing but its values.
	complete := func(t *testing.T, args ...string) []string {
		t.Helper()
		got, directive, r := h.complete(args...)
		if r.stderr != "" || directive != ":4" {
			t.Errorf("wt %v: stderr %q, directive %q", args, r.stderr, directive)
		}
		return got
	}

	t.Run("repository names outside a repository", func(t *testing.T) {
		if got, want := complete(t, "cd", ""), []string{"api", "feat", "TrendFisher", "web"}; !slices.Equal(got, want) {
			t.Errorf("offered %q, want %q", got, want)
		}
	})
	t.Run("repository prefix ignores case", func(t *testing.T) {
		if got := complete(t, "cd", "trend"); !slices.Equal(got, []string{"TrendFisher"}) {
			t.Errorf("wt cd trend: offered %q", got)
		}
		if got := complete(t, "cd", "W"); !slices.Equal(got, []string{"web"}) {
			t.Errorf("wt cd W: offered %q", got)
		}
	})

	repo, _ := cdRepo(h)
	t.Run("worktrees and repositories together", func(t *testing.T) {
		// feat is both a worktree and a repository: offered once.
		if got, want := complete(t, "cd", ""), []string{"repo", "feat", "api", "TrendFisher", "web"}; !slices.Equal(got, want) {
			t.Errorf("offered %q, want %q", got, want)
		}
		if got := complete(t, "cd", "f"); !slices.Equal(got, []string{"feat"}) {
			t.Errorf("wt cd f: offered %q", got)
		}
	})
	t.Run("other commands offer only worktrees", func(t *testing.T) {
		if got := complete(t, "remove", ""); !slices.Equal(got, []string{"feat"}) {
			t.Errorf("wt remove: offered %q", got)
		}
	})
	t.Run("missing root", func(t *testing.T) {
		h.roots(0, h.sb.Path("usb", "repos"), root)
		if got := complete(t, "cd", "a"); !slices.Equal(got, []string{"api"}) {
			t.Errorf("offered %q", got)
		}
		h.cwd = h.sb.Path("outside")
		h.roots(0, h.sb.Path("usb", "repos"))
		if got, _, r := h.complete("cd", ""); len(got) != 0 || r.stdout != ":4\n" || r.stderr != "" {
			t.Errorf("stdout %q, stderr %q; want only the directive", r.stdout, r.stderr)
		}
		h.cwd = filepath.Join(repo, "sub")
	})
	t.Run("invalid configuration", func(t *testing.T) {
		for _, content := range []string{"repos_root = \n", "repos_root = [\"GIT\"]\n", "repos_depth = 9\n"} {
			h.userConfig(content)
			if got := complete(t, "cd", ""); !slices.Equal(got, []string{"repo", "feat"}) {
				t.Errorf("%q: offered %q, want only the worktrees", content, got)
			}
		}
		// A warning is dropped, and the repositories are still offered.
		h.userConfig("repos_root = [" + strconv.Quote(root) + "]\ncolour = \"blue\"\n")
		if got := complete(t, "cd", "a"); !slices.Equal(got, []string{"api"}) {
			t.Errorf("with a warning: offered %q", got)
		}
		h.roots(0, root)
		h.sb.Setenv("WT_REPOS_DEPTH", "two")
		if got := complete(t, "cd", ""); !slices.Equal(got, []string{"repo", "feat"}) {
			t.Errorf("WT_REPOS_DEPTH=two: offered %q", got)
		}
	})
}

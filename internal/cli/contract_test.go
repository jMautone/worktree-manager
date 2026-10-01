package cli_test

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

func TestHelpListsCommands(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{}, {"help"}, {"-h"}, {"--help"}} {
		r := h.run(args...)
		r.mustCode(t, 0)
		for _, cmd := range []string{"list", "cd", "config", "shell", "version"} {
			if !regexp.MustCompile(`(?m)^\s+` + cmd + `\s`).MatchString(r.stdout) {
				t.Errorf("wt %v: help does not list %q:\n%s", args, cmd, r.stdout)
			}
		}
		if regexp.MustCompile(`(?m)^\s+completion\s`).MatchString(r.stdout) {
			t.Errorf("wt %v: help lists cobra's completion command; completions come from wt shell init", args)
		}
		if r.stderr != "" {
			t.Errorf("wt %v: stderr = %q", args, r.stderr)
		}
	}
}

func TestCommandHelp(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"list", "--help"}, "wt list"},
		{[]string{"list", "-h"}, "wt list"},
		{[]string{"help", "list"}, "wt list"},
		{[]string{"help", "config", "get"}, "wt config get"},
		{[]string{"config"}, "wt config"},
	} {
		r := h.run(tc.args...)
		r.mustCode(t, 0)
		if !strings.Contains(r.stdout, tc.want) {
			t.Errorf("wt %v: stdout does not describe %q:\n%s", tc.args, tc.want, r.stdout)
		}
		if !strings.Contains(r.stdout, "-h, --help") {
			t.Errorf("wt %v: usage does not show the help flag:\n%s", tc.args, r.stdout)
		}
	}
}

func TestRootHelpShowsGlobalFlags(t *testing.T) {
	r := newHarness(t).run("-h")
	r.mustCode(t, 0)
	for _, want := range []string{"-C dir", "--json", "--dry-run", "--no-color", "-h, --help", "--version"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help does not show %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "-C, --") {
		t.Errorf("help renders -C with an empty long name:\n%s", r.stdout)
	}
}

func TestRootHelpExplainsGitWtHelp(t *testing.T) {
	r := newHarness(t).run("--help")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "git wt --help") || !strings.Contains(r.stdout, "git wt -h") {
		t.Errorf("root help does not explain that git intercepts `git wt --help`:\n%s", r.stdout)
	}
}

func TestStrictValidation(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"lsit"}, []string{`unknown command "lsit"`, `"list"`}},
		{[]string{"list", "--bogus"}, []string{"--bogus"}},
		{[]string{"list", "extra"}, []string{`"extra"`}},
		{[]string{"-C"}, []string{"-C"}},
		{[]string{"list", "--json=maybe"}, []string{"--json"}},
		{[]string{"help", "nope"}, []string{`"nope"`}},
		{[]string{"config", "bogus"}, []string{`unknown command "bogus"`}},
		{[]string{"config", "get"}, []string{"key"}},
		{[]string{"config", "get", "default_base", "extra"}, []string{`"extra"`}},
		{[]string{"config", "list", "extra"}, []string{`"extra"`}},
		{[]string{"config", "path", "extra"}, []string{`"extra"`}},
		{[]string{"version", "extra"}, []string{`"extra"`}},
		{[]string{"--version", "extra"}, []string{`"extra"`}},
		{[]string{"list", "--version"}, []string{"--version"}},
	} {
		r := h.run(tc.args...)
		if r.code != 2 {
			t.Errorf("wt %v: exit %d, want 2 (stderr %q)", tc.args, r.code, r.stderr)
		}
		if r.stdout != "" {
			t.Errorf("wt %v: stdout = %q, want empty", tc.args, r.stdout)
		}
		if !strings.HasPrefix(r.stderr, "wt: ") {
			t.Errorf("wt %v: stderr = %q, want it to start with \"wt: \"", tc.args, r.stderr)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.stderr, w) {
				t.Errorf("wt %v: stderr %q does not contain %q", tc.args, r.stderr, w)
			}
		}
	}
}

func TestErrorFormatText(t *testing.T) {
	h := newHarness(t)
	r := h.run("list")
	r.mustCode(t, 3)
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
	if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
		t.Errorf("stderr first line = %q, want %q", got, want)
	}
}

func TestErrorFormatTextHints(t *testing.T) {
	r := newHarness(t).run("lsit")
	lines := strings.Split(strings.TrimRight(r.stderr, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("stderr = %q, want a message and hints", r.stderr)
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "hint: ") {
			t.Errorf("line after the message %q does not start with \"hint: \"", l)
		}
	}
}

func TestErrorFormatJSON(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"list", "--json"}, {"--json", "list"}} {
		r := h.run(args...)
		r.mustCode(t, 3)
		if r.stdout != "" {
			t.Errorf("wt %v: stdout = %q, want empty", args, r.stdout)
		}
		doc := decodeOne(t, r.stderr)
		if doc["schema"] != "wt.error.v1" || doc["code"] != float64(3) {
			t.Errorf("wt %v: error document = %v", args, doc)
		}
		if msg, _ := doc["message"].(string); msg != "not a git repository: "+h.cwd {
			t.Errorf("wt %v: message = %q", args, msg)
		}
	}
}

func TestUsageErrorInJSON(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"--json", "lsit"}, {"lsit", "--json"}, {"list", "--bogus", "--json"}} {
		r := h.run(args...)
		r.mustCode(t, 2)
		doc := decodeOne(t, r.stderr)
		if doc["schema"] != "wt.error.v1" || doc["code"] != float64(2) {
			t.Errorf("wt %v: error document = %v", args, doc)
		}
	}
	doc := decodeOne(t, h.run("--json", "lsit").stderr)
	hints, _ := doc["hints"].([]any)
	if len(hints) == 0 || !strings.Contains(hints[0].(string), "list") {
		t.Errorf("hints = %v, want a suggestion of list", doc["hints"])
	}
}

func TestWarningsAreSuppressedWithJSON(t *testing.T) {
	h := newHarness(t)
	h.userConfig("colour = \"blue\"\n")

	r := h.run("config", "list")
	r.mustCode(t, 0)
	if !strings.Contains(r.stderr, "wt: warning: ") || !strings.Contains(r.stderr, "colour") {
		t.Errorf("stderr = %q, want a warning naming colour", r.stderr)
	}

	r = h.run("config", "list", "--json")
	r.mustCode(t, 0)
	if r.stderr != "" {
		t.Errorf("with --json, stderr = %q, want empty", r.stderr)
	}
}

func TestWorkingDirectoryOverride(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()

	t.Run("absolute", func(t *testing.T) {
		r := h.run("-C", repo, "list")
		r.mustCode(t, 0)
		if !strings.Contains(r.stdout, testutil.Comparable(t, repo)) {
			t.Errorf("stdout does not list %s:\n%s", repo, r.stdout)
		}
	})
	t.Run("relative to the working directory", func(t *testing.T) {
		h.cwd = h.sb.Root
		defer func() { h.cwd = h.sb.Path("outside") }()
		h.run("-C", "repo", "list").mustCode(t, 0)
	})
	t.Run("after the command", func(t *testing.T) {
		h.run("list", "-C", repo).mustCode(t, 0)
	})
	t.Run("missing directory", func(t *testing.T) {
		missing := filepath.Join(h.sb.Root, "does", "not", "exist")
		r := h.run("-C", missing, "list")
		r.mustCode(t, 2)
		if !strings.Contains(r.stderr, missing) {
			t.Errorf("stderr %q does not name %s", r.stderr, missing)
		}
	})
	t.Run("not a directory", func(t *testing.T) {
		file := h.sb.Path("file.txt")
		h.sb.WriteFile(file, "x")
		r := h.run("-C", file, "list")
		r.mustCode(t, 2)
		if !strings.Contains(r.stderr, file) {
			t.Errorf("stderr %q does not name %s", r.stderr, file)
		}
	})
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"version"}, {"--version"}} {
		r := h.run(args...)
		r.mustCode(t, 0)
		if !strings.HasPrefix(r.stdout, "wt 1.2.3-test") {
			t.Errorf("wt %v: stdout = %q", args, r.stdout)
		}
	}
	for _, args := range [][]string{{"version", "--json"}, {"--json", "version"}, {"--version", "--json"}} {
		r := h.run(args...)
		r.mustCode(t, 0)
		doc := decodeOne(t, r.stdout)
		if doc["schema"] != "wt.version.v1" || doc["version"] != "1.2.3-test" ||
			doc["os"] != runtime.GOOS || doc["arch"] != runtime.GOARCH {
			t.Errorf("wt %v: document = %v", args, doc)
		}
		if _, ok := doc["commit"].(string); !ok {
			t.Errorf("wt %v: commit = %v, want a string", args, doc["commit"])
		}
	}
}

func TestVersionWithBrokenConfiguration(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.userConfig("default_base = \n")

	for _, args := range [][]string{{"version"}, {"--version"}, {"version", "--json"}} {
		h.run(args...).mustCode(t, 0)
	}
	// The configuration really is broken for commands that read it.
	h.run("list").mustCode(t, 1)
}

func TestColor(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	const esc = "\x1b["

	h.tty = true
	if r := h.run("list"); !strings.Contains(r.stdout, esc) {
		t.Errorf("terminal: no color in\n%q", r.stdout)
	}
	for _, tc := range []struct {
		name    string
		tty     bool
		noColor string
		args    []string
	}{
		{"piped", false, "", []string{"list"}},
		{"NO_COLOR", true, "1", []string{"list"}},
		{"--no-color", true, "", []string{"list", "--no-color"}},
		{"JSON on a terminal", true, "", []string{"list", "--json"}},
	} {
		h.tty = tc.tty
		h.sb.Setenv("NO_COLOR", tc.noColor)
		r := h.run(tc.args...)
		r.mustCode(t, 0)
		if strings.Contains(r.stdout, esc) || strings.Contains(r.stderr, esc) {
			t.Errorf("%s: output contains ANSI sequences:\n%q", tc.name, r.stdout)
		}
	}
}

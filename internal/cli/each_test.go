package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// eachRepo creates <sandbox>/repo with the linked worktrees feat and fix,
// and starts inside feat.
func eachRepo(h *harness) (repo, feat, fix string) {
	h.t.Helper()
	repo = h.repo()
	feat = h.sb.Path("repo.worktrees", "feat")
	fix = h.sb.Path("repo.worktrees", "fix")
	h.sb.AddWorktree(repo, feat, "feat")
	h.sb.AddWorktree(repo, fix, "fix")
	h.cwd = feat
	return repo, feat, fix
}

// unixOnly skips t on Windows: its command is written for sh.
func unixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the command is written for sh; see each_windows_test.go")
	}
}

// outLines splits output into lines, with cmd.exe's \r\n as \n.
func outLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// header is the header wt each writes for label and the worktree at path.
func header(t *testing.T, label, path string) string {
	return "==> " + label + "  " + testutil.Comparable(t, path)
}

// wouldRun is the line wt each --dry-run writes for line and path.
func wouldRun(t *testing.T, line, path string) string {
	return "would run " + line + " in " + testutil.Comparable(t, path)
}

// mustRunNothing runs wt with args and checks that no command ran.
func mustRunNothing(t *testing.T, h *harness, args ...string) result {
	t.Helper()
	dirs, restore := cli.RecordRuns()
	defer restore()
	r := h.run(args...)
	if len(*dirs) != 0 {
		t.Errorf("wt %v ran commands in %q", args, *dirs)
	}
	return r
}

// --- 3.1: reading, usage errors and --dry-run ---

// Scenarios of "Worktrees visited" that fail or read elsewhere.
func TestEachWorktreesVisited(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")

	t.Run("outside a repository", func(t *testing.T) {
		for _, args := range [][]string{{"each", "--dry-run", "git", "status"}, {"each", "git", "status"}} {
			r := mustRunNothing(t, h, args...)
			r.mustCode(t, 3)
			if got, want := firstLine(r.stderr), "wt: not a git repository: "+h.cwd; got != want {
				t.Errorf("wt %v: first line %q, want %q", args, got, want)
			}
			mustHint(t, r, "wt each --all-repos")
			if r.stdout != "" {
				t.Errorf("wt %v: stdout %q", args, r.stdout)
			}
		}
	})
	t.Run("working directory override", func(t *testing.T) {
		r := h.run("-C", api, "each", "--dry-run", "git", "status")
		r.mustCode(t, 0)
		want := []string{wouldRun(t, "git status", api), wouldRun(t, "git status", feat)}
		if got := outLines(r.stdout); !slices.Equal(got, want) {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
	t.Run("repository that git cannot read", func(t *testing.T) {
		broken := filepath.Join(root, "broken")
		h.sb.InitRepo(broken)
		h.sb.BreakHEAD(broken)
		defer os.RemoveAll(broken)

		r := h.run("each", "--all-repos", "--dry-run", "git", "status")
		r.mustCode(t, 0)
		want := []string{wouldRun(t, "git status", api), wouldRun(t, "git status", feat), wouldRun(t, "git status", web)}
		if got := outLines(r.stdout); !slices.Equal(got, want) {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		if !strings.Contains(r.stderr, "wt: warning: cannot read the worktrees of broken: ") {
			t.Errorf("stderr = %q, want a warning naming broken", r.stderr)
		}

		// The warning comes before anything a command writes.
		r = h.run("each", "--all-repos", "echo ran 1>&2")
		r.mustCode(t, 0)
		if errLines := outLines(r.stderr); !strings.HasPrefix(errLines[0], "wt: warning: cannot read the worktrees of broken: ") || len(errLines) != 4 {
			t.Errorf("stderr = %q, want the warning first and then the 3 commands'", errLines)
		}
	})

	h2 := newHarness(t)
	h2.repo()
	t.Run("no roots configured", func(t *testing.T) {
		r := mustRunNothing(t, h2, "each", "--all-repos", "--dry-run", "git", "status")
		r.mustCode(t, 1)
		if got := firstLine(r.stderr); got != "wt: no repository roots configured" {
			t.Errorf("first line %q", got)
		}
	})
	t.Run("outside a repository without roots", func(t *testing.T) {
		r := mustRunNothing(t, h2, "each", "git", "status")
		r.mustCode(t, 3)
		if strings.Contains(r.stderr, "--all-repos") {
			t.Errorf("stderr names --all-repos without roots:\n%s", r.stderr)
		}
	})
	t.Run("invalid repository configuration", func(t *testing.T) {
		repo := h2.sb.Path("repo")
		h2.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \n")
		h2.cwd = repo
		defer func() { h2.cwd = h2.sb.Path("outside") }()
		r := mustRunNothing(t, h2, "each", "git", "status")
		r.mustCode(t, 1)
		if r.stdout != "" {
			t.Errorf("stdout = %q", r.stdout)
		}
	})
}

// Scenarios of "Worktrees skipped".
func TestEachWorktreesSkipped(t *testing.T) {
	t.Run("bare repository", func(t *testing.T) {
		h := newHarness(t)
		proj := h.sb.Path("root", "proj")
		h.sb.InitDotBare(proj)
		feat := filepath.Join(proj, "feat")
		h.sb.AddWorktree(proj, feat, "feat")
		h.cwd = feat
		r := h.run("each", "--dry-run", "git", "status")
		r.mustCode(t, 0)
		if want := wouldRun(t, "git status", feat) + "\n"; strings.ReplaceAll(r.stdout, "\r\n", "\n") != want || r.stderr != "" {
			t.Errorf("stdout %q, stderr %q; want %q and nothing", r.stdout, r.stderr, want)
		}
		dirs, restore := cli.RecordRuns()
		defer restore()
		h.run("each", "git", "status").mustCode(t, 0)
		if len(*dirs) != 1 || testutil.Comparable(t, (*dirs)[0]) != testutil.Comparable(t, feat) {
			t.Errorf("ran in %q, want only feat", *dirs)
		}
	})
	t.Run("directory gone", func(t *testing.T) {
		h := newHarness(t)
		repo, feat, fix := eachRepo(h)
		h.sb.MakePrunable(fix)
		usb := h.sb.Path("repo.worktrees", "usb")
		h.sb.AddWorktree(repo, usb, "usb")
		h.sb.LockWorktree(repo, usb, "on a usb drive")
		h.sb.MakePrunable(usb)

		r := h.run("each", "--dry-run", "git", "status")
		r.mustCode(t, 0)
		want := []string{wouldRun(t, "git status", repo), wouldRun(t, "git status", feat)}
		if got := outLines(r.stdout); !slices.Equal(got, want) {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		wantErr := []string{
			"wt: warning: skipping fix: " + testutil.Comparable(t, fix) + " does not exist",
			"wt: warning: skipping usb: " + testutil.Comparable(t, usb) + " does not exist",
		}
		if got := outLines(r.stderr); !slices.Equal(got, wantErr) {
			t.Errorf("stderr = %q, want %q", got, wantErr)
		}

		// Before the first header, and not counted as commands.
		r = h.run("each", "exit 3")
		r.mustCode(t, 1)
		if !strings.HasPrefix(r.stderr, wantErr[0]+"\n"+wantErr[1]+"\n") || !strings.Contains(r.stderr, "wt: 2 of 2 commands failed\n") {
			t.Errorf("stderr = %q", r.stderr)
		}
	})
}

// Scenarios of "Start of the command" that need no command to run, and
// "JSON requested".
func TestEachUsage(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.cwd = repo

	t.Run("double dash", func(t *testing.T) {
		r := h.run("each", "--dry-run", "--", "git", "log", "--oneline", "-1")
		r.mustCode(t, 0)
		if got, want := outLines(r.stdout), []string{wouldRun(t, "git log --oneline -1", repo)}; !slices.Equal(got, want) {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no command", []string{"each"}, `wt: missing <cmd> for "wt each"`},
		{"no command after --", []string{"each", "--"}, `wt: missing <cmd> for "wt each"`},
		{"empty command", []string{"each", " "}, `wt: empty command for "wt each"`},
		{"empty word", []string{"each", ""}, `wt: empty command for "wt each"`},
		{"unknown flag before the command", []string{"each", "--bogus", "git", "status"}, "wt: unknown flag: --bogus"},
		{"--json of the command does not make the error JSON", []string{"each", "--bogus", "echo", "x", "--json"}, "wt: unknown flag: --bogus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := mustRunNothing(t, h, tc.args...)
			r.mustCode(t, 2)
			if got := firstLine(r.stderr); got != tc.want {
				t.Errorf("first line %q, want %q", got, tc.want)
			}
			if r.stdout != "" {
				t.Errorf("stdout = %q", r.stdout)
			}
		})
	}
	t.Run("usage errors before reading", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		defer func() { h.cwd = repo }()
		for _, args := range [][]string{{"each"}, {"each", " "}, {"each", "--json", "x"}} {
			mustRunNothing(t, h, args...).mustCode(t, 2)
		}
	})
	t.Run("JSON requested", func(t *testing.T) {
		for _, args := range [][]string{{"each", "--json", "git", "status"}, {"--json", "each", "git", "status"}} {
			r := mustRunNothing(t, h, args...)
			r.mustCode(t, 2)
			if r.stdout != "" {
				t.Errorf("wt %v: stdout = %q", args, r.stdout)
			}
			doc := decodeOne(t, r.stderr)
			hints, _ := doc["hints"].([]any)
			if doc["schema"] != "wt.error.v1" || doc["code"] != float64(2) || len(hints) == 0 || !strings.Contains(hints[0].(string), "wt list --json") {
				t.Errorf("wt %v: error document = %v", args, doc)
			}
		}
	})
}

// Scenarios "Line shown by a dry run (macOS, Linux)" and "Preview".
func TestEachDryRun(t *testing.T) {
	unixOnly(t)
	h := newHarness(t)
	repo, _ := cdRepo(h)
	feat := h.sb.Path("repo.worktrees", "feat")

	t.Run("line shown", func(t *testing.T) {
		h2 := newHarness(t)
		h2.cwd = h2.repo()
		r := h2.run("each", "--dry-run", "git", "commit", "-m", "fix bug")
		r.mustCode(t, 0)
		if want := wouldRun(t, "git commit -m 'fix bug'", h2.cwd) + "\n"; r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
	})
	t.Run("preview", func(t *testing.T) {
		r := mustRunNothing(t, h, "each", "--dry-run", "touch ran")
		r.mustCode(t, 0)
		if want := wouldRun(t, "touch ran", repo) + "\n" + wouldRun(t, "touch ran", feat) + "\n"; r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
		for _, dir := range []string{repo, feat, h.cwd} {
			if exists(filepath.Join(dir, "ran")) {
				t.Errorf("ran exists in %s", dir)
			}
		}
	})
	t.Run("no worktree", func(t *testing.T) {
		h3 := newHarness(t)
		root := h3.mkdir(h3.sb.Path("empty-root"))
		h3.roots(0, root)
		for _, args := range [][]string{{"each", "--all-repos", "true"}, {"each", "--all-repos", "--dry-run", "true"}} {
			r := h3.run(args...)
			r.mustCode(t, 0)
			if r.stdout != "" {
				t.Errorf("wt %v: stdout = %q", args, r.stdout)
			}
		}
	})
}

func TestEachHelp(t *testing.T) {
	h := newHarness(t)
	r := h.run("-h")
	r.mustCode(t, 0)
	if !regexp.MustCompile(`(?m)^\s+each\s`).MatchString(r.stdout) {
		t.Errorf("wt -h does not list each:\n%s", r.stdout)
	}
	r = h.run("each", "-h")
	r.mustCode(t, 0)
	for _, want := range []string{"wt each [--all-repos] [--] <cmd>...", "A --\nbefore it is allowed but never needed", "wt each 'npm ci && npm test'", `wt each git commit -m "fix bug"`, "--all-repos"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("wt each -h does not contain %q:\n%s", want, r.stdout)
		}
	}
}

// --- 3.2: running the commands ---

// Scenarios "Every worktree of the current repository" and "Header before
// the output".
func TestEachCurrentRepository(t *testing.T) {
	h := newHarness(t)
	repo, feat, fix := eachRepo(h)
	r := h.run("each", "git", "branch", "--show-current")
	r.mustCode(t, 0)
	want := []string{header(t, "^", repo), "main", header(t, "feat", feat), "feat", header(t, "fix", fix), "fix"}
	if got := outLines(r.stdout); !slices.Equal(got, want) {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if r.stderr != "" {
		t.Errorf("stderr = %q", r.stderr)
	}
}

// Scenarios "Every repository" and "Headers of every repository".
func TestEachEveryRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	r := h.run("each", "--all-repos", "git", "branch", "--show-current")
	r.mustCode(t, 0)
	want := []string{
		header(t, "api:^", filepath.Join(root, "api")), "main",
		header(t, "api:feat", feat), "feat",
		header(t, "web:^", filepath.Join(root, "web")), "main",
	}
	if got := outLines(r.stdout); !slices.Equal(got, want) {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// Scenarios "Flags of the command" and "A flag of wt after the first word"
// of batch-execution, and "Global flag after the words of another command"
// of cli-contract.
func TestEachFlagsOfTheCommand(t *testing.T) {
	h := newHarness(t)
	repo, feat := cdRepo(h)
	for _, args := range [][]string{
		{"each", "git", "log", "--oneline", "-1"},
		{"each", "git", "log", "--no-color", "-1"},
		{"each", "--", "git", "log", "--oneline", "-1"},
	} {
		r := h.run(args...)
		r.mustCode(t, 0)
		if n := strings.Count(r.stdout, "initial commit"); n != 2 {
			t.Errorf("wt %v: %d commits shown, want 2:\n%s", args, n, r.stdout)
		}
	}
	r := h.run("each", "echo", "--dry-run")
	r.mustCode(t, 0)
	want := []string{header(t, "^", repo), "--dry-run", header(t, "feat", feat), "--dry-run"}
	if got := outLines(r.stdout); !slices.Equal(got, want) {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// Scenarios "Interpreter operators in a single word (macOS, Linux)",
// "Working directory of each command" and "Headers in the current
// repository".
func TestEachRunsWithSh(t *testing.T) {
	unixOnly(t)
	h := newHarness(t)
	repo, feat := cdRepo(h) // from repo/sub

	r := h.run("each", "echo a && echo b")
	r.mustCode(t, 0)
	if got, want := outLines(r.stdout), []string{header(t, "^", repo), "a", "b", header(t, "feat", feat), "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("operators: stdout = %q, want %q", got, want)
	}

	r = h.run("each", "pwd")
	r.mustCode(t, 0)
	if got, want := outLines(r.stdout), []string{header(t, "^", repo), testutil.Comparable(t, repo), header(t, "feat", feat), testutil.Comparable(t, feat)}; !slices.Equal(got, want) {
		t.Errorf("pwd: stdout = %q, want %q", got, want)
	}

	r = h.run("each", "true")
	r.mustCode(t, 0)
	if want := header(t, "^", repo) + "\n" + header(t, "feat", feat) + "\n"; r.stdout != want {
		t.Errorf("headers: stdout = %q, want %q", r.stdout, want)
	}
}

// Scenarios of "Failures", and "Failed command in a batch" and "Report of
// a batch" of cli-contract.
func TestEachFailures(t *testing.T) {
	h := newHarness(t)
	_, _, fix := eachRepo(h)

	t.Run("failed command in a batch", func(t *testing.T) {
		r := h.run("each", "exit 4")
		r.mustCode(t, 1)
		want := []string{"wt: 3 of 3 commands failed", "hint: ^ exited with 4", "hint: feat exited with 4", "hint: fix exited with 4"}
		if got := outLines(r.stderr); !slices.Equal(got, want) {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
	t.Run("one command fails", func(t *testing.T) {
		unixOnly(t)
		r := h.run("each", `[ "${PWD##*/}" != feat ] || exit 3`)
		r.mustCode(t, 1)
		if !strings.Contains(r.stdout, header(t, "fix", fix)) {
			t.Errorf("the command did not run in fix:\n%s", r.stdout)
		}
		if want := "wt: 1 of 3 commands failed\nhint: feat exited with 3\n"; r.stderr != want {
			t.Errorf("stderr = %q, want %q", r.stderr, want)
		}
	})
	t.Run("every command succeeds", func(t *testing.T) {
		unixOnly(t)
		r := h.run("each", "true")
		r.mustCode(t, 0)
		if r.stderr != "" {
			t.Errorf("stderr = %q", r.stderr)
		}
	})
	t.Run("terminated by a signal", func(t *testing.T) {
		unixOnly(t)
		h2 := newHarness(t)
		h2.cwd = h2.repo()
		r := h2.run("each", "kill -TERM $$")
		r.mustCode(t, 1)
		mustHint(t, r, "^ exited with 143")
	})
	t.Run("report of a batch", func(t *testing.T) {
		unixOnly(t)
		h2 := newHarness(t)
		h2.cwd = h2.repo()
		r := h2.run("each", "echo ok; exit 3")
		r.mustCode(t, 1)
		if want := header(t, "^", h2.cwd) + "\nok\n"; r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
		if want := "wt: 1 of 1 commands failed\nhint: ^ exited with 3\n"; r.stderr != want {
			t.Errorf("stderr = %q, want %q", r.stderr, want)
		}
	})
	t.Run("could not start", func(t *testing.T) {
		restore := cli.FailToStart(errors.New("no luck"))
		defer restore()
		r := h.run("each", "git", "status")
		r.mustCode(t, 1)
		if n := strings.Count(r.stdout, "==> "); n != 3 {
			t.Errorf("%d headers, want 3:\n%s", n, r.stdout)
		}
		want := []string{"wt: 3 of 3 commands failed", "hint: ^ could not start: no luck", "hint: feat could not start: no luck", "hint: fix could not start: no luck"}
		if got := outLines(r.stderr); !slices.Equal(got, want) {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
}

func TestEachHeaderColor(t *testing.T) {
	h := newHarness(t)
	eachRepo(h)
	plain := h.run("each", "git", "branch", "--show-current")
	h.tty = true
	colored := h.run("each", "git", "branch", "--show-current")
	h.tty = false
	if !strings.Contains(colored.stdout, "\x1b[") {
		t.Fatalf("no color in %q", colored.stdout)
	}
	if got := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(colored.stdout, ""); got != plain.stdout {
		t.Errorf("without its colors, the output is\n%q\nwant\n%q", got, plain.stdout)
	}
	if noColor := h.run("each", "--no-color", "git", "branch", "--show-current"); noColor.stdout != plain.stdout {
		t.Errorf("--no-color: %q", noColor.stdout)
	}
}

// --- 3.4: completions ---

// Scenarios of "Completions".
func TestEachCompletions(t *testing.T) {
	h := newHarness(t)
	cdRepo(h)
	for _, tc := range []struct {
		args      []string
		offered   []string
		directive string
	}{
		{[]string{"each", "--a"}, []string{"--all-repos"}, ":4"},
		{[]string{"each", "--all-repos", "--d"}, []string{"--dry-run"}, ":4"},
		{[]string{"each", "git", "log", "--a"}, nil, ":0"},
		{[]string{"each", "git", "log", "--"}, nil, ":0"},
		{[]string{"each", ""}, nil, ":0"},
		{[]string{"each", "git", ""}, nil, ":0"},
		{[]string{"each", "--", ""}, nil, ":0"},
	} {
		got, directive, r := h.complete(tc.args...)
		if !slices.Equal(got, tc.offered) || directive != tc.directive {
			t.Errorf("wt %q: offered %q with %s, want %q with %s", tc.args, got, directive, tc.offered, tc.directive)
		}
		if r.stderr != "" {
			t.Errorf("wt %q: stderr = %q", tc.args, r.stderr)
		}
	}
}

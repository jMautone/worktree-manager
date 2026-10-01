package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// These tests load the output of `git-wt shell init` in the real shell and
// check what the user sees: the shell's working directory, exit codes, and
// what is left behind. Each one runs a short script without startup files.
//
// A shell that is not installed is skipped, unless it is listed in
// WT_TEST_SHELLS (comma-separated): CI lists the shells of each OS, so none is
// skipped there in silence. On Windows only PowerShell is supported.

// dialect is how a test script is written in one shell.
type dialect struct {
	name string
	// argv runs the script that follows it as the last argument.
	argv []string
	// file runs a script file that follows it; ext is the file's extension.
	file []string
	ext  string
	// load is the installation line of the spec.
	load string
	// report prints "@@ <label> rc=<result of the last command> pwd=<dir>".
	report string
	// isFunction exits 9 unless wt is a shell function.
	isFunction string
	// directiveVar prints "@@ var=<WT_DIRECTIVE_CD_FILE, or unset>".
	directiveVar string
	// leftovers matches the temporary files the function creates.
	leftovers string
}

var dialects = []dialect{
	{
		name:         "zsh",
		argv:         []string{"zsh", "-f", "-c"},
		file:         []string{"zsh", "-f"},
		ext:          ".zsh",
		load:         `eval "$(git-wt shell init zsh)"`,
		report:       `echo "@@ LABEL rc=$? pwd=$PWD"`,
		isFunction:   `(( $+functions[wt] )) || exit 9`,
		directiveVar: `echo "@@ var=${WT_DIRECTIVE_CD_FILE-unset}"`,
		leftovers:    "wt.*",
	},
	{
		name:         "bash",
		argv:         []string{"bash", "--norc", "--noprofile", "-c"},
		file:         []string{"bash", "--norc", "--noprofile"},
		ext:          ".bash",
		load:         `eval "$(git-wt shell init bash)"`,
		report:       `echo "@@ LABEL rc=$? pwd=$PWD"`,
		isFunction:   `declare -F wt >/dev/null || exit 9`,
		directiveVar: `echo "@@ var=${WT_DIRECTIVE_CD_FILE-unset}"`,
		leftovers:    "wt.*",
	},
	{
		name:         "fish",
		argv:         []string{"fish", "--no-config", "-c"},
		file:         []string{"fish", "--no-config"},
		ext:          ".fish",
		load:         `git-wt shell init fish | source`,
		report:       `echo "@@ LABEL rc=$status pwd=$PWD"`,
		isFunction:   `functions -q wt; or exit 9`,
		directiveVar: `echo "@@ var="(set -q WT_DIRECTIVE_CD_FILE; and echo $WT_DIRECTIVE_CD_FILE; or echo unset)`,
		leftovers:    "wt.*",
	},
	{
		name:         "pwsh",
		argv:         []string{"pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command"},
		file:         []string{"pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-File"},
		ext:          ".ps1",
		load:         `Invoke-Expression (& git-wt shell init pwsh | Out-String)`,
		report:       `"@@ LABEL rc=$LASTEXITCODE pwd=$($PWD.ProviderPath)"`,
		isFunction:   `if ((Get-Command wt).CommandType -ne 'Function') { exit 9 }`,
		directiveVar: `"@@ var=$(if (Test-Path Env:WT_DIRECTIVE_CD_FILE) { $env:WT_DIRECTIVE_CD_FILE } else { 'unset' })"`,
		leftovers:    "tmp*.tmp",
	},
}

// rep is d.report for label.
func (d dialect) rep(label string) string {
	return strings.ReplaceAll(d.report, "LABEL", label)
}

// forEachShell runs f as a subtest per shell that is available.
func forEachShell(t *testing.T, f func(t *testing.T, d dialect)) {
	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			requireShell(t, d.name)
			f(t, d)
		})
	}
}

func requireShell(t *testing.T, name string) {
	t.Helper()
	if runtime.GOOS == "windows" && name != "pwsh" {
		t.Skipf("%s is not supported on Windows", name)
	}
	if _, err := exec.LookPath(name); err != nil {
		if slices.Contains(strings.Split(os.Getenv("WT_TEST_SHELLS"), ","), name) {
			t.Fatalf("%s is listed in WT_TEST_SHELLS but is not on the PATH", name)
		}
		t.Skipf("%s is not installed", name)
	}
}

// shellEnv is a sandbox for the shell tests: a repository with a linked
// worktree, git-wt first on the PATH, and the system temporary directory
// inside the sandbox.
type shellEnv struct {
	sb   *testutil.Sandbox
	repo string // <sandbox>/repo, main worktree
	sub  string // <repo>/sub, where scripts start by default
	feat string // <sandbox>/repo.worktrees/feat, branch feat
	tmp  string // the system temporary directory of the shells
	// path is prepended to the PATH, before git-wt's directory.
	path []string
	// env is added to the environment.
	env []string
}

func newShellEnv(t *testing.T) *shellEnv {
	t.Helper()
	sb := testutil.New(t)
	e := &shellEnv{
		sb:   sb,
		repo: sb.Path("repo"),
		feat: sb.Path("repo.worktrees", "feat"),
		tmp:  sb.Path("tmp"),
	}
	e.sub = filepath.Join(e.repo, "sub")
	sb.InitRepo(e.repo)
	sb.AddWorktree(e.repo, e.feat, "feat")
	for _, dir := range []string{e.sub, e.tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// environ is the environment of a shell run.
func (e *shellEnv) environ() []string {
	path := append(append([]string(nil), e.path...), binDir, e.sb.Getenv("PATH"))
	set := map[string]string{"PATH": strings.Join(path, string(os.PathListSeparator))}
	if runtime.GOOS == "windows" {
		set["TEMP"], set["TMP"] = e.tmp, e.tmp
	} else {
		set["TMPDIR"] = e.tmp
	}
	var env []string
	for _, kv := range e.sb.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if _, ok := set[strings.ToUpper(key)]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range set {
		env = append(env, k+"="+v)
	}
	return append(env, e.env...)
}

// run runs script in shell d, in dir.
func (e *shellEnv) run(t *testing.T, d dialect, dir, script string) result {
	t.Helper()
	return e.exec(t, dir, append(d.argv[1:], script), d.argv[0])
}

// runFile runs script as a file in shell d, in dir.
func (e *shellEnv) runFile(t *testing.T, d dialect, dir, script string) result {
	t.Helper()
	path := filepath.Join(e.sb.Root, "script"+d.ext)
	e.sb.WriteFile(path, script)
	return e.exec(t, dir, append(d.file[1:], path), d.file[0])
}

func (e *shellEnv) exec(t *testing.T, dir string, args []string, name string) result {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = e.environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", name, err)
	}
	return result{code, stdout.String(), stderr.String()}
}

// lines joins the lines of a script.
func lines(l ...string) string {
	return strings.Join(l, "\n") + "\n"
}

// mark is one "@@ <label> rc=<rc> pwd=<pwd>" line of a script's output.
type mark struct {
	rc, pwd string
}

// marks parses the report lines of out by label. Other lines, such as the
// output of wt itself, are ignored.
func marks(t *testing.T, r result) map[string]mark {
	t.Helper()
	m := map[string]mark{}
	for _, l := range strings.Split(strings.ReplaceAll(r.stdout, "\r\n", "\n"), "\n") {
		rest, ok := strings.CutPrefix(l, "@@ ")
		if !ok {
			continue
		}
		label, rest, _ := strings.Cut(rest, " ")
		rc, pwd, _ := strings.Cut(strings.TrimPrefix(rest, "rc="), " pwd=")
		m[label] = mark{rc, pwd}
	}
	return m
}

// mustMark checks the result and directory a report recorded.
func mustMark(t *testing.T, r result, label, rc, pwd string) {
	t.Helper()
	m, ok := marks(t, r)[label]
	if !ok {
		t.Fatalf("no report %q\nstdout:\n%s\nstderr:\n%s", label, r.stdout, r.stderr)
	}
	if m.rc != rc {
		t.Errorf("%s: result %q, want %s\nstdout:\n%s\nstderr:\n%s", label, m.rc, rc, r.stdout, r.stderr)
	}
	if pwd != "" && (m.pwd == "" || testutil.Comparable(t, m.pwd) != testutil.Comparable(t, pwd)) {
		t.Errorf("%s: working directory %q, want %s\nstderr:\n%s", label, m.pwd, pwd, r.stderr)
	}
}

func TestShellSilentLoad(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		r := e.run(t, d, e.sub, lines(d.load, d.isFunction))
		if r.code != 0 || r.stdout != "" || r.stderr != "" {
			t.Errorf("loading the script: exit %d, stdout %q, stderr %q; want 0 and nothing written", r.code, r.stdout, r.stderr)
		}
	})
}

// mustNoLeftovers checks that the function removed its temporary files.
func (e *shellEnv) mustNoLeftovers(t *testing.T, d dialect) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(e.tmp, d.leftovers))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("temporary files left behind: %q", left)
	}
}

func TestShellFunction(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		r := e.run(t, d, e.sub, lines(
			d.load,
			"wt list", d.rep("list"),
			"wt cd feat", d.rep("cd"),
			d.directiveVar,
			"wt lsit", d.rep("lsit"),
		))
		// Commands that do not move the shell.
		mustMark(t, r, "list", "0", e.sub)
		// The binary moves the shell.
		mustMark(t, r, "cd", "0", e.feat)
		// Exit code.
		mustMark(t, r, "lsit", "2", e.feat)
		// Variable not left set.
		if !strings.Contains(strings.ReplaceAll(r.stdout, "\r\n", "\n"), "@@ var=unset\n") {
			t.Errorf("WT_DIRECTIVE_CD_FILE is set after wt returns:\n%s", r.stdout)
		}
		// No file left behind.
		e.mustNoLeftovers(t, d)
	})
}

func TestShellOutputIsNotCaptured(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		direct := e.exec(t, e.sub, []string{"list"}, filepath.Join(binDir, exeName()))
		if direct.code != 0 || direct.stdout == "" {
			t.Fatalf("git-wt list: exit %d, stdout %q, stderr %q", direct.code, direct.stdout, direct.stderr)
		}
		r := e.run(t, d, e.sub, lines(d.load, "wt list"))
		if got := strings.ReplaceAll(r.stdout, "\r\n", "\n"); got != direct.stdout {
			t.Errorf("wt list through the function:\n%s\nwant the output of git-wt list:\n%s", got, direct.stdout)
		}
	})
}

func TestShellArguments(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		for _, tc := range []struct {
			arg, want string
		}{
			{`"a b"`, `unknown configuration key "a b"`},
			// An empty argument that got lost would be a missing <key>.
			{`""`, `unknown configuration key ""`},
		} {
			r := e.run(t, d, e.sub, lines(d.load, "wt config get "+tc.arg, d.rep("get")))
			mustMark(t, r, "get", "2", "")
			if !strings.Contains(r.stderr, tc.want) {
				t.Errorf("wt config get %s: stderr %q does not contain %q", tc.arg, r.stderr, tc.want)
			}
		}
	})
}

func TestShellLoadedTwice(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		r := e.run(t, d, e.sub, lines(
			d.load, d.load,
			"wt cd feat", d.rep("cd"),
			"wt cd -", d.rep("back"),
		))
		if r.stderr != "" {
			t.Errorf("stderr = %q", r.stderr)
		}
		mustMark(t, r, "cd", "0", e.feat)
		// Had the directory changed twice, the previous one would be feat.
		mustMark(t, r, "back", "0", e.sub)
		e.mustNoLeftovers(t, d)
	})
}

// Aliases expand when a function is defined, not when it runs: a user's
// alias of rm, cd or mktemp loaded before the script must not get into the
// function.
func TestShellIgnoresAliases(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		if d.name != "zsh" && d.name != "bash" {
			t.Skip("aliases of commands are a zsh and bash concern")
		}
		e := newShellEnv(t)
		r := e.run(t, d, e.sub, lines(
			// Non-interactive bash does not expand aliases by default.
			`[ -n "$BASH_VERSION" ] && shopt -s expand_aliases`,
			`alias rm='rm -i' cd='echo aliased cd' mktemp='echo /nonexistent/wt.XXXXXX'`,
			// The script is loaded with eval, so these aliases are in
			// effect when its function is defined.
			`eval 'echo "@@ alias $(mktemp)"'`,
			d.load,
			"wt cd feat", d.rep("cd"),
		))
		if !strings.Contains(r.stdout, "@@ alias /nonexistent/wt.XXXXXX\n") {
			t.Fatalf("the aliases are not in effect:\n%s\n%s", r.stdout, r.stderr)
		}
		mustMark(t, r, "cd", "0", e.feat)
		e.mustNoLeftovers(t, d)
	})
}

// quote writes s as a literal string in every supported shell.
func quote(s string) string {
	return "'" + s + "'"
}

func TestShellPreviousDirectory(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		t.Run("back and forth", func(t *testing.T) {
			r := e.run(t, d, e.sub, lines(
				d.load,
				"wt cd feat", d.rep("there"),
				"wt cd -", d.rep("back"),
				"wt cd -", d.rep("again"),
			))
			mustMark(t, r, "there", "0", e.feat)
			mustMark(t, r, "back", "0", e.sub)
			mustMark(t, r, "again", "0", e.feat)
		})
		t.Run("outside a repository", func(t *testing.T) {
			outside := e.sb.Path("outside")
			if err := os.Mkdir(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			r := e.run(t, d, outside, lines(
				d.load,
				"wt -C "+quote(e.repo)+" cd feat", d.rep("there"),
				"wt cd -", d.rep("back"),
			))
			mustMark(t, r, "there", "0", e.feat)
			mustMark(t, r, "back", "0", outside)
		})
		t.Run("new session", func(t *testing.T) {
			child := filepath.Join(e.sb.Root, "child"+d.ext)
			e.sb.WriteFile(child, lines(d.load, "wt cd -", d.rep("child")))
			argv := append(append([]string(nil), d.file...), quote(child))
			r := e.run(t, d, e.sub, lines(
				d.load,
				"wt cd feat", d.rep("parent"),
				strings.Join(argv, " "),
			))
			mustMark(t, r, "parent", "0", e.feat)
			mustMark(t, r, "child", "3", e.feat)
			if !strings.Contains(r.stderr, "wt: no previous directory") {
				t.Errorf("stderr = %q, want no previous directory", r.stderr)
			}
		})
	})
}

func TestShellProcessesStartedByWt(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		log := e.sb.Path("git-environment")
		e.path = []string{fakeGitDir}
		e.env = []string{"FAKEPROG_LOG=" + log, "FAKEPROG_GIT=" + realGit}
		// After a jump the previous directory is not empty, so a leak of
		// either variable would show.
		r := e.run(t, d, e.sub, lines(
			d.load,
			"wt cd feat", d.rep("cd"),
			"wt list", d.rep("list"),
		))
		mustMark(t, r, "cd", "0", e.feat)
		mustMark(t, r, "list", "0", e.feat)
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatalf("the fake git recorded nothing: %v", err)
		}
		if !strings.Contains(string(b), "\n--\n") {
			t.Fatalf("unexpected record:\n%s", b)
		}
		for _, kv := range strings.Split(string(b), "\n") {
			key, _, _ := strings.Cut(kv, "=")
			for _, v := range []string{"WT_DIRECTIVE_CD_FILE", "WT_PREVIOUS_DIR"} {
				if strings.EqualFold(key, v) {
					t.Errorf("git received %s", kv)
				}
			}
		}
	})
}

func TestShellShadowsOtherWt(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		e.path = []string{fakeWtDir}
		// Without the script, wt is the other program.
		if r := e.run(t, d, e.sub, lines("wt")); !strings.Contains(r.stdout, "fakeprog: wt") {
			t.Fatalf("the fake wt is not on the PATH: stdout %q, stderr %q", r.stdout, r.stderr)
		}
		r := e.run(t, d, e.sub, lines(d.load, "wt version", d.rep("version")))
		mustMark(t, r, "version", "0", "")
		if !strings.Contains(r.stdout, "wt "+e2eVersion) || strings.Contains(r.stdout, "fakeprog") {
			t.Errorf("wt version ran the other program:\n%s", r.stdout)
		}
	})
}

// bashCompletion is the bash-completion package's main script, or "". On
// macOS the system bash is 3.2, which only bash-completion 1.x supports.
func bashCompletion() string {
	for _, p := range []string{
		"/usr/share/bash-completion/bash_completion",
		"/opt/homebrew/etc/bash_completion",
		"/usr/local/etc/bash_completion",
		"/etc/bash_completion",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// completionScript loads the script with the shell's completion system and
// prints "@@ reply <value>" for each value offered for line, which ends
// where the cursor is.
func completionScript(t *testing.T, d dialect, line string) string {
	t.Helper()
	reply := `"@@ reply $c"`
	switch d.name {
	case "bash":
		bc := bashCompletion()
		if bc == "" {
			if slices.Contains(strings.Split(os.Getenv("WT_TEST_SHELLS"), ","), "bash") {
				t.Fatal("bash is listed in WT_TEST_SHELLS but the bash-completion package is not installed")
			}
			t.Skip("the bash-completion package is not installed")
		}
		return lines(
			// bash-completion 1.x returns early when the shell is not
			// interactive, which it tells by PS1.
			`PS1='$ '`,
			// The package also loads every completion installed on the
			// machine, and some fail in bash 3.2: not ours to report.
			"source "+quote(bc)+" 2>/dev/null",
			d.load,
			// compopt only works while readline runs a completion
			// function, and the test calls __start_wt directly.
			`[[ $(type -t compopt) == builtin ]] && compopt() { :; }`,
			"COMP_LINE="+quote(line)+"; COMP_POINT=${#COMP_LINE}; COMP_TYPE=9",
			"read -ra COMP_WORDS <<< \"$COMP_LINE\"",
			`[[ $COMP_LINE == *' ' ]] && COMP_WORDS+=('')`,
			"COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 ))",
			"__start_wt",
			`for c in "${COMPREPLY[@]}"; do echo `+reply+`; done`,
		)
	case "fish":
		return lines(d.load, "for c in (complete -C "+quote(line)+"); echo "+reply+"; end")
	case "pwsh":
		// With no values, cobra's completer returns "" so that PowerShell
		// does not complete file names: TAB inserts nothing. PSReadLine,
		// which calls TabExpansion2 on TAB, also ignores the exception
		// TabExpansion2 may raise on it.
		return lines(d.load, fmt.Sprintf(
			"try { $r = TabExpansion2 -inputScript %s -cursorColumn %d } catch { $r = $null }",
			quote(line), len(line)),
			fmt.Sprintf("$r.CompletionMatches | Where-Object CompletionText | ForEach-Object { $c = $_.CompletionText; %s }", reply))
	}
	t.Fatalf("no completion request for %s", d.name)
	return ""
}

// replies returns the values a completion script offered, without
// descriptions.
func replies(r result) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(r.stdout, "\r\n", "\n"), "\n") {
		if v, ok := strings.CutPrefix(l, "@@ reply "); ok {
			v, _, _ = strings.Cut(v, "\t")
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func TestShellCompletions(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		e := newShellEnv(t)
		if d.name == "zsh" {
			// The completion function needs the line editor; a real TAB is
			// checked by hand. Here: compinit before the script registers
			// it.
			r := e.run(t, d, e.sub, lines(
				"autoload -Uz compinit && compinit -u -D",
				d.load,
				`echo "@@ comp ${_comps[wt]-none}"`,
			))
			if !strings.Contains(r.stdout, "@@ comp _wt\n") || r.stderr != "" {
				t.Errorf("completion not registered after compinit:\nstdout %q\nstderr %q", r.stdout, r.stderr)
			}
			return
		}
		e.sb.AddWorktree(e.repo, e.sb.Path("repo.worktrees", "fix"), "fix")
		outside := e.sb.Path("outside")
		if err := os.Mkdir(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, dir, line string
			want            []string
		}{
			{"worktree names", e.sub, "wt cd ", []string{"repo", "feat", "fix"}},
			{"subcommands", e.sub, "wt sh", []string{"shell"}},
			{"shell names", e.sub, "wt shell init ", []string{"zsh", "bash", "fish", "pwsh"}},
			{"outside a repository", outside, "wt cd ", nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				r := e.run(t, d, tc.dir, completionScript(t, d, tc.line))
				got := replies(r)
				slices.Sort(got)
				want := slices.Clone(tc.want)
				slices.Sort(want)
				if !slices.Equal(got, want) {
					t.Errorf("%q offered %q, want %q\nstdout:\n%s\nstderr:\n%s", tc.line, got, want, r.stdout, r.stderr)
				}
				if r.stderr != "" {
					t.Errorf("stderr = %q, want nothing written", r.stderr)
				}
				for _, l := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
					if l != "" && !strings.HasPrefix(l, "@@ reply ") {
						t.Errorf("unexpected output %q", l)
					}
				}
				e.mustNoLeftovers(t, d)
			})
		}
	})
}

func TestShellZshWithoutCompinit(t *testing.T) {
	forEachShell(t, func(t *testing.T, d dialect) {
		if d.name != "zsh" {
			t.Skip("compinit is zsh's")
		}
		e := newShellEnv(t)
		r := e.run(t, d, e.sub, lines(d.load, `echo "@@ comp ${_comps[wt]-none}"`, "wt cd feat", d.rep("cd")))
		if r.stderr != "" || !strings.HasPrefix(r.stdout, "@@ comp none\n") {
			t.Errorf("loading without compinit: stdout %q, stderr %q", r.stdout, r.stderr)
		}
		mustMark(t, r, "cd", "0", e.feat)
	})
}

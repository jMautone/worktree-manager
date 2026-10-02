package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// These tests run the compiled binary, for what only a real process shows:
// the exit code the OS sees and git resolving `git wt` to git-wt on the PATH.
// Everything else is tested in process through cli.Run.

const e2eVersion = "9.9.9-e2e"

var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wt-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-ldflags", "-X main.version="+e2eVersion, "-o", filepath.Join(dir, exeName()), ".")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building git-wt: %v\n%s", err, out)
		os.Exit(1)
	}
	binDir = dir
	if err := buildFakes(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func exeName() string {
	return exe("git-wt")
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Directories with testdata/fakeprog built as git and as wt, to put first on
// the PATH; realGit is the git they stand in front of.
var fakeGitDir, fakeWtDir, realGit string

func buildFakes(dir string) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	realGit = git
	fakeGitDir, fakeWtDir = filepath.Join(dir, "fakegit"), filepath.Join(dir, "fakewt")
	for _, out := range []string{filepath.Join(fakeGitDir, exe("git")), filepath.Join(fakeWtDir, exe("wt"))} {
		build := exec.Command("go", "build", "-o", out, "./testdata/fakeprog")
		if b, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("building %s: %v\n%s", out, err, b)
		}
	}
	return nil
}

type result struct {
	code           int
	stdout, stderr string
}

// execute runs name (the binary, or git) in dir with the sandbox environment.
func execute(t *testing.T, sb *testutil.Sandbox, dir, name string, args ...string) result {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = sb.Environ()
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

func wt(t *testing.T, sb *testutil.Sandbox, dir string, args ...string) result {
	t.Helper()
	return execute(t, sb, dir, filepath.Join(binDir, exeName()), args...)
}

func TestProcessExitCodes(t *testing.T) {
	sb := testutil.New(t)
	outside := sb.Path("outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := sb.Path("repo")
	sb.InitRepo(repo)

	for _, tc := range []struct {
		dir  string
		args []string
		code int
	}{
		{outside, []string{"version"}, 0},
		{outside, []string{"lsit"}, 2},
		{outside, []string{"list"}, 3},
		{repo, []string{"list"}, 0},
	} {
		if r := wt(t, sb, tc.dir, tc.args...); r.code != tc.code {
			t.Errorf("git-wt %v: exit %d, want %d\nstderr: %s", tc.args, r.code, tc.code, r.stderr)
		}
	}
}

func TestVersionIsInjectedAtBuildTime(t *testing.T) {
	sb := testutil.New(t)
	r := wt(t, sb, sb.Root, "version")
	if !strings.HasPrefix(r.stdout, "wt "+e2eVersion) {
		t.Errorf("stdout = %q, want the -ldflags version", r.stdout)
	}
}

// go install …@vX.Y.Z does not pass -ldflags; the binary must report the
// module version Go recorded instead of a placeholder.
func TestVersionFallsBackToTheModuleVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), exeName())
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building git-wt: %v\n%s", err, out)
	}
	out, err := exec.Command("go", "version", "-m", bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	module := ""
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == "mod" {
			module = f[2]
		}
	}
	want := "wt " + strings.TrimPrefix(module, "v")
	if module == "" || module == "(devel)" {
		want = "wt 0.0.0-dev"
	}

	sb := testutil.New(t)
	r := execute(t, sb, sb.Root, bin, "version")
	if !strings.HasPrefix(r.stdout, want+" ") && r.stdout != want+"\n" {
		t.Errorf("stdout = %q, want %q (module version %q)", r.stdout, want, module)
	}
}

func TestInvokedThroughGit(t *testing.T) {
	sb := testutil.New(t)
	sb.Setenv("PATH", binDir+string(os.PathListSeparator)+sb.Getenv("PATH"))

	direct := wt(t, sb, sb.Root, "version")
	viaGit := execute(t, sb, sb.Root, "git", "wt", "version")
	if viaGit.code != 0 || viaGit.stdout != direct.stdout {
		t.Errorf("git wt version = %q (exit %d, stderr %q), want %q", viaGit.stdout, viaGit.code, viaGit.stderr, direct.stdout)
	}
}

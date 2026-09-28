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
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "git-wt.exe"
	}
	return "git-wt"
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

func TestInvokedThroughGit(t *testing.T) {
	sb := testutil.New(t)
	sb.Setenv("PATH", binDir+string(os.PathListSeparator)+sb.Getenv("PATH"))

	direct := wt(t, sb, sb.Root, "version")
	viaGit := execute(t, sb, sb.Root, "git", "wt", "version")
	if viaGit.code != 0 || viaGit.stdout != direct.stdout {
		t.Errorf("git wt version = %q (exit %d, stderr %q), want %q", viaGit.stdout, viaGit.code, viaGit.stderr, direct.stdout)
	}
}

//go:build !windows

package process

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir, cmdline string, env []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, err := Run(dir, cmdline, env, strings.NewReader("input\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run(%q): %v", cmdline, err)
	}
	return code, stdout.String(), stderr.String()
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	env := []string{"PATH=" + os.Getenv("PATH"), "WT_TEST_VAR=x y"}

	if code, _, _ := run(t, dir, "exit 3", env); code != 3 {
		t.Errorf("exit 3: code %d", code)
	}
	if code, out, _ := run(t, dir, "echo a && echo b", env); code != 0 || out != "a\nb\n" {
		t.Errorf("&&: code %d, stdout %q", code, out)
	}
	if code, _, _ := run(t, dir, "kill -INT $$", env); code != 130 {
		t.Errorf("SIGINT: code %d, want 130", code)
	}
	if code, _, _ := run(t, dir, "kill -TERM $$", env); code != 128+15 {
		t.Errorf("SIGTERM: code %d, want 143", code)
	}
	if _, out, _ := run(t, dir, "pwd -P", env); strings.TrimSpace(out) != mustReal(t, dir) {
		t.Errorf("working directory %q, want %s", out, dir)
	}
	if _, out, _ := run(t, dir, "env", env); !strings.Contains(out, "WT_TEST_VAR=x y\n") || strings.Contains(out, "HOME=") {
		t.Errorf("environment not passed as is:\n%s", out)
	}
	if _, out, _ := run(t, dir, "cat", env); out != "input\n" {
		t.Errorf("stdin: %q", out)
	}
	if _, _, errOut := run(t, dir, "echo oops >&2", env); errOut != "oops\n" {
		t.Errorf("stderr: %q", errOut)
	}
	if code, _, _ := run(t, dir, "nonexistent-command-wt", env); code != 127 {
		t.Errorf("unknown command: code %d, want sh's 127", code)
	}
}

func TestRunMissingDirectory(t *testing.T) {
	var out bytes.Buffer
	code, err := Run(filepath.Join(t.TempDir(), "missing"), "exit 0", nil, nil, &out, &out)
	if err == nil {
		t.Errorf("Run in a missing directory: code %d, no error", code)
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

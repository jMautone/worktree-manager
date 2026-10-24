//go:build !windows

package process

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// TestDetachedChild is not a test on its own: TestStartDetached runs the
// test binary as the detached process, which prints its session id, waits
// for the file $WT_TEST_RELEASE, and prints done.
func TestDetachedChild(t *testing.T) {
	release := os.Getenv("WT_TEST_RELEASE")
	if release == "" {
		t.Skip("run by TestStartDetached")
	}
	sid, err := getsid()
	if err != nil {
		fmt.Println("getsid:", err)
		os.Exit(1)
	}
	fmt.Println(sid)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(release); err == nil {
			fmt.Println("done")
			os.Exit(0)
		}
	}
	fmt.Println("timed out")
	os.Exit(1)
}

func TestStartDetached(t *testing.T) {
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	out, err := os.Create(filepath.Join(dir, "log"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "WT_TEST_RELEASE="+release)
	if err := StartDetached(self, []string{self, "-test.run=^TestDetachedChild$"}, dir, env, out); err != nil {
		t.Fatal(err)
	}
	// The child waits for release, which only exists once StartDetached
	// returned: had it waited for the child, the child would have timed out.
	log := waitForLines(t, out.Name(), 1)
	if strings.Contains(log, "done") || strings.Contains(log, "timed out") {
		t.Fatalf("the child finished before StartDetached returned:\n%s", log)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	log = waitForLines(t, out.Name(), 2)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 || lines[1] != "done" {
		t.Fatalf("log = %q, want the session id and done", log)
	}
	childSid, err := strconv.Atoi(lines[0])
	if err != nil {
		t.Fatalf("session id %q: %v", lines[0], err)
	}
	ownSid, err := getsid()
	if err != nil {
		t.Fatal(err)
	}
	if childSid == ownSid {
		t.Errorf("the child runs in the test's session %d", ownSid)
	}
}

// waitForLines waits until the file at path has n complete lines, and
// returns its contents.
func waitForLines(t *testing.T, path string, n int) string {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(b), "\n") >= n {
			return string(b)
		}
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s does not have %d lines after 15s:\n%s", path, n, b)
	return ""
}

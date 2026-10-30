package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// Scenario "Ctrl-C in the console (Windows)", as far as a test can go:
// git-wt runs in a process group of its own, which gets a CTRL_BREAK_EVENT
// (Ctrl-C cannot be sent to a group) while the first of two commands runs.
// Go delivers both events as os.Interrupt. The command ends, and git-wt
// runs no other and exits with 130. Pressing Ctrl-C in a real console is
// checked by hand (task 5.3 of batch-execution).
func TestInterruptEachWindows(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat := sb.Path("repo.worktrees", "feat")
	sb.AddWorktree(repo, feat, "feat")

	cmd := exec.Command(filepath.Join(binDir, exeName()), "each", sleeper)
	cmd.Dir = repo
	cmd.Env = sb.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(filepath.Join(repo, "started")) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Skipf("cannot send CTRL_BREAK_EVENT without a console (%v); see task 5.3 of batch-execution", err)
	}

	err := cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 130 {
		t.Fatalf("git-wt: %v, want exit status 130\nstderr:\n%s", err, stderr.String())
	}
	if fileExists(filepath.Join(feat, "started")) {
		t.Error("the command ran in feat after the interrupt")
	}
	if !strings.Contains(stderr.String(), "wt: interrupted: 1 of 2 commands did not run") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// Scenario "Ctrl-C in the console (Windows)" of hooks, as far as a test can
// go: git-wt runs in a process group of its own, which gets a
// CTRL_BREAK_EVENT while a pre-create command waits. git-wt waits for the
// command, creates nothing and exits with 130. Pressing Ctrl-C in a real
// console is checked by hand (task 5.3 of hooks).
func TestInterruptDuringPreCreateWindows(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat := sb.Path("repo.worktrees", "feat")
	userConfig(t, sb, hooksConfig("pre-create", `"`+sleeper+`"`, "echo x> second"))

	cmd := exec.Command(filepath.Join(binDir, exeName()), "create", "feat", "--no-cd")
	cmd.Dir = repo
	cmd.Env = sb.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(filepath.Join(repo, "started")) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the hook did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Skipf("cannot send CTRL_BREAK_EVENT without a console (%v); see task 5.3 of hooks", err)
	}

	err := cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 130 {
		t.Fatalf("git-wt: %v, want exit status 130\nstderr:\n%s", err, stderr.String())
	}
	if fileExists(filepath.Join(repo, "second")) {
		t.Error("the second command of the event ran")
	}
	if !strings.Contains(stderr.String(), "wt: interrupted: pre-create hook did not finish") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if fileExists(feat) {
		t.Error("the worktree's directory was created")
	}
	if out := sb.Git(repo, "branch", "--list", "feat"); out != "" {
		t.Errorf("the branch was created: %q", out)
	}
}

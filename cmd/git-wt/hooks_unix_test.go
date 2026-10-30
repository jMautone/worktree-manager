//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// Scenario "Signals to the terminal (macOS, Linux)": wt runs in a process
// group of its own, as a job of an interactive shell does, and once it
// exits, that group gets the SIGHUP of a closing terminal and the SIGINT of
// a Ctrl-C. The background hook is in a session of its own, so neither
// reaches it.
func TestPostRemoveSurvivesTheTerminal(t *testing.T) {
	sb, repo, _ := hookRepo(t)
	userConfig(t, sb, hooksConfig("post-remove", waitTwoSecondsThenDone()))
	log := filepath.Join(repo, ".git", "wt", "logs", "feat", "post-remove.log")

	cmd := exec.Command(filepath.Join(binDir, exeName()), "remove", "feat")
	cmd.Dir = repo
	cmd.Env = sb.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git-wt: %v\n%s", err, out)
	}
	pgid := cmd.Process.Pid
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGINT} {
		// ESRCH: nothing is left in the group, the hook included.
		if err := syscall.Kill(-pgid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("kill %v: %v", sig, err)
		}
	}
	waitFor(t, "the log of post-remove", func() bool { return finished(log) })
	if !fileExists(filepath.Join(repo, "done.txt")) {
		t.Errorf("the hook did not finish: log %q", logLines(log))
	}
	if lines := logLines(log); lines[len(lines)-1] != "==> exited with 0" {
		t.Errorf("log = %q", lines)
	}
}

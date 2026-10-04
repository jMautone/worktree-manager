//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// A Ctrl-C reaches the whole process group; here only git-wt gets it, to
// show that it neither dies nor ends the command: it waits for the command
// and exits with its code.
func TestInterruptWhileTheCommandRuns(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat := sb.Path("repo.worktrees", "feat")

	cmd := exec.Command(filepath.Join(binDir, exeName()), "create", "feat", "--no-cd", "-x", "touch started; sleep 1; touch finished; exit 7")
	cmd.Dir = repo
	cmd.Env = sb.Environ()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(filepath.Join(feat, "started")) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	err := cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 7 {
		t.Fatalf("git-wt: %v, want exit status 7", err)
	}
	if !fileExists(filepath.Join(feat, "finished")) {
		t.Error("git-wt exited before the command did")
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

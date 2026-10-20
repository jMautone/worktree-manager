//go:build !windows

package main

import (
	"bytes"
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

// Scenarios "Interrupt while a command runs (macOS, Linux)" of
// batch-execution and "Interrupted batch (macOS, Linux)" of cli-contract.
// As above, only git-wt gets the SIGINT: the command keeps running, and
// git-wt waits for it, runs no other and exits with 130, whatever the
// command's exit code.
func TestInterruptEach(t *testing.T) {
	for _, tc := range []struct {
		name, exit, hint string
	}{
		{"command succeeds", "", ""},
		{"command fails", "; exit 3", "hint: ^ exited with 3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sb := testutil.New(t)
			repo := sb.Path("repo")
			sb.InitRepo(repo)
			feat := sb.Path("repo.worktrees", "feat")
			sb.AddWorktree(repo, feat, "feat")

			cmd := exec.Command(filepath.Join(binDir, exeName()), "each", "touch started; sleep 1; touch finished"+tc.exit)
			cmd.Dir = repo
			cmd.Env = sb.Environ()
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
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}

			err := cmd.Wait()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 130 {
				t.Fatalf("git-wt: %v, want exit status 130\nstderr:\n%s", err, stderr.String())
			}
			if !fileExists(filepath.Join(repo, "finished")) {
				t.Error("git-wt exited before the command did")
			}
			if fileExists(filepath.Join(feat, "started")) {
				t.Error("the command ran in feat after the interrupt")
			}
			if want := "wt: interrupted: 1 of 2 commands did not run\n" + tc.hint; stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}

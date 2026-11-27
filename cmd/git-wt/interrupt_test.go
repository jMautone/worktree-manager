//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// Scenario "Interrupt during pre-create (macOS, Linux)" of hooks. As above,
// only git-wt gets the SIGINT: the hook keeps running, and git-wt waits for
// it, creates nothing and exits with 130, though the hook exits with 0.
func TestInterruptDuringPreCreate(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat := sb.Path("repo.worktrees", "feat")
	userConfig(t, sb, hooksConfig("pre-create", "touch started; sleep 1; touch finished", "touch second"))

	cmd := exec.Command(filepath.Join(binDir, exeName()), "create", "feat", "--no-cd")
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
			t.Fatal("the hook did not start")
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
		t.Error("git-wt exited before the hook did")
	}
	if fileExists(filepath.Join(repo, "second")) {
		t.Error("the second command of the event ran")
	}
	if !strings.Contains(stderr.String(), "wt: interrupted: pre-create hook did not finish\n") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if out := sb.Git(repo, "branch", "--list", "feat"); out != "" || fileExists(feat) {
		t.Errorf("something was created: branch %q, directory there: %v", out, fileExists(feat))
	}
	if out := sb.Git(repo, "worktree", "list", "--porcelain"); strings.Count(out, "worktree ") != 1 {
		t.Errorf("worktrees:\n%s", out)
	}
}

// Scenario "Interrupt during a rebase (macOS, Linux)" of sync-worktrees. A
// pre-rebase hook keeps git busy while feat is rebased; as above, only
// git-wt gets the SIGINT, so git finishes that rebase. git-wt waits for it,
// leaves no rebase in progress, does not rebase fix, and exits with 130.
func TestInterruptSync(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	repo := sb.Path("repo")
	sb.Clone(origin, repo)
	feat, fix := sb.Path("repo.worktrees", "feat"), sb.Path("repo.worktrees", "fix")
	sb.AddWorktree(repo, feat, "feat")
	sb.AddWorktree(repo, fix, "fix")
	sb.Commit(feat, "feat")
	fixHead := sb.Commit(fix, "fix")
	sb.PushFromAnotherClone(origin, "main")

	started, finished := sb.Path("started"), sb.Path("finished")
	hooks := sb.Path("hooks")
	sb.WriteFile(filepath.Join(hooks, "pre-rebase"), "#!/bin/sh\ntouch '"+started+"'\nsleep 1\ntouch '"+finished+"'\n")
	if err := os.Chmod(filepath.Join(hooks, "pre-rebase"), 0o755); err != nil {
		t.Fatal(err)
	}
	sb.Git(repo, "config", "core.hooksPath", hooks)

	cmd := exec.Command(filepath.Join(binDir, exeName()), "sync", "--all")
	cmd.Dir = repo
	cmd.Env = sb.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(started) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the rebase did not start")
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
	if !fileExists(finished) {
		t.Error("git-wt exited before git did")
	}
	for _, p := range []string{"rebase-merge", "rebase-apply"} {
		if fileExists(filepath.Join(sb.Git(feat, "rev-parse", "--absolute-git-dir"), p)) {
			t.Errorf("feat has a rebase in progress (%s)", p)
		}
	}
	if got := sb.Git(fix, "rev-parse", "HEAD"); got != fixHead {
		t.Error("fix was rebased after the interrupt")
	}
	if !strings.HasPrefix(stderr.String(), "wt: interrupted: 1 of 2 worktrees not synced") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// Scenario "Interrupt during the rebase (macOS, Linux)" of merge-worktree:
// the SIGINT reaches only git-wt, which waits for git, moves nothing and
// exits with 130.
func TestInterruptMerge(t *testing.T) {
	sb := testutil.New(t)
	origin := sb.Path("origin.git")
	sb.InitRemote(origin)
	repo := sb.Path("repo")
	sb.Clone(origin, repo)
	feat := sb.Path("repo.worktrees", "feat")
	sb.AddWorktree(repo, feat, "feat")
	sb.Commit(feat, "feat")
	mainHead := sb.Commit(repo, "main")

	started, finished := sb.Path("started"), sb.Path("finished")
	hooks := sb.Path("hooks")
	sb.WriteFile(filepath.Join(hooks, "pre-rebase"), "#!/bin/sh\ntouch '"+started+"'\nsleep 1\ntouch '"+finished+"'\n")
	if err := os.Chmod(filepath.Join(hooks, "pre-rebase"), 0o755); err != nil {
		t.Fatal(err)
	}
	sb.Git(repo, "config", "core.hooksPath", hooks)

	cmd := exec.Command(filepath.Join(binDir, exeName()), "merge")
	cmd.Dir = feat
	cmd.Env = sb.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(started) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the rebase did not start")
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
	if !fileExists(finished) {
		t.Error("git-wt exited before git did")
	}
	for _, p := range []string{"rebase-merge", "rebase-apply"} {
		if fileExists(filepath.Join(sb.Git(feat, "rev-parse", "--absolute-git-dir"), p)) {
			t.Errorf("feat has a rebase in progress (%s)", p)
		}
	}
	if got := sb.Git(repo, "rev-parse", "main"); got != mainHead {
		t.Errorf("main moved to %s", got)
	}
	if !fileExists(feat) {
		t.Error("feat was removed")
	}
	if !strings.HasPrefix(stderr.String(), "wt: interrupted: feat was not merged into main") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// Scenario "Interrupt in the editor (macOS, Linux)" of merge-steps. The
// standard input of the test is not a terminal, so the message comes from
// -m, and a commit-msg hook of git that takes a second stands for the
// editor: either way the interrupt arrives while git commits.
func TestInterruptSquash(t *testing.T) {
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	feat := sb.Path("repo.worktrees", "feat")
	sb.AddWorktree(repo, feat, "feat")
	sb.Commit(feat, "one")
	sb.Commit(feat, "two")
	featHead := sb.Commit(feat, "three")

	started, finished := sb.Path("started"), sb.Path("finished")
	hooks := sb.Path("hooks")
	sb.WriteFile(filepath.Join(hooks, "commit-msg"), "#!/bin/sh\ntouch '"+started+"'\nsleep 1\ntouch '"+finished+"'\n")
	if err := os.Chmod(filepath.Join(hooks, "commit-msg"), 0o755); err != nil {
		t.Fatal(err)
	}
	sb.Git(repo, "config", "core.hooksPath", hooks)

	cmd := exec.Command(filepath.Join(binDir, exeName()), "step", "squash", "main", "-m", "all")
	cmd.Dir = feat
	cmd.Env = sb.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !fileExists(started) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the commit did not start")
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
	if !fileExists(finished) {
		t.Error("git-wt exited before git did")
	}
	if got := sb.Git(feat, "rev-parse", "HEAD"); got != featHead {
		t.Errorf("feat = %s, want %s", got, featHead)
	}
	if got := sb.Git(feat, "status", "--porcelain"); got != "" {
		t.Errorf("status = %q, want clean", got)
	}
}

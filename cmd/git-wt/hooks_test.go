package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// These tests run the hooks of the real binary, for what only real
// processes show: that wt does not wait for a background hook, and that the
// process it starts outlives it. The order of the events and the contents
// of their commands' output are tested in process, in internal/cli.

// userConfig writes the sandbox's user configuration file.
func userConfig(t *testing.T, sb *testutil.Sandbox, content string) {
	t.Helper()
	dir := sb.Getenv("XDG_CONFIG_HOME")
	if runtime.GOOS == "windows" {
		dir = sb.Getenv("APPDATA")
	}
	sb.WriteFile(filepath.Join(dir, "wt", "config.toml"), content)
}

// hooksConfig is a [hooks] table that sets event to commands.
func hooksConfig(event string, commands ...string) string {
	quoted := make([]string, len(commands))
	for i, c := range commands {
		quoted[i] = strconv.Quote(c)
	}
	return "[hooks]\n" + event + " = [" + strings.Join(quoted, ", ") + "]\n"
}

// waitTwoSecondsThenDone is a command that waits two seconds and then
// creates done.txt in its working directory.
func waitTwoSecondsThenDone() string {
	if runtime.GOOS == "windows" {
		return "ping -n 3 127.0.0.1 > nul& echo x> done.txt"
	}
	return "sleep 2; touch done.txt"
}

// waitFor polls cond until it holds, or fails the test after 20 seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// logLines reads a log as lines, without the \r of cmd.exe; it returns nil
// while the file does not exist.
func logLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
}

// finished reports whether the log at path ends with a footer.
func finished(path string) bool {
	lines := logLines(path)
	if len(lines) == 0 {
		return false
	}
	last := lines[len(lines)-1]
	return strings.HasPrefix(last, "==> exited with ") || strings.HasPrefix(last, "==> could not start: ")
}

// hookRepo creates <sandbox>/repo with the clean linked worktree
// <sandbox>/repo.worktrees/feat on the branch feat.
func hookRepo(t *testing.T) (sb *testutil.Sandbox, repo, feat string) {
	t.Helper()
	sb = testutil.New(t)
	repo = sb.Path("repo")
	sb.InitRepo(repo)
	feat = sb.Path("repo.worktrees", "feat")
	sb.AddWorktree(repo, feat, "feat")
	return sb, repo, feat
}

// Scenario "wt does not wait", and the log of a real background process.
func TestPostRemoveDoesNotWait(t *testing.T) {
	sb, repo, _ := hookRepo(t)
	userConfig(t, sb, hooksConfig("post-remove", waitTwoSecondsThenDone(), "echo bye"))
	done := filepath.Join(repo, "done.txt")
	log := filepath.Join(repo, ".git", "wt", "logs", "feat", "post-remove.log")

	start := time.Now()
	r := wt(t, sb, repo, "remove", "feat")
	if r.code != 0 {
		t.Fatalf("exit %d\nstderr: %s", r.code, r.stderr)
	}
	if fileExists(done) {
		t.Fatalf("done.txt exists when wt returns, after %v: wt waited for the hook", time.Since(start))
	}
	if !strings.Contains(r.stderr, "==> post-remove  running in the background, log: ") {
		t.Errorf("stderr = %q", r.stderr)
	}
	waitFor(t, "the log of post-remove", func() bool { return finished(log) && len(logLines(log)) >= 5 })
	if !fileExists(done) {
		t.Error("the main worktree has no done.txt")
	}
	lines := logLines(log)
	want := []string{"==> post-remove  " + waitTwoSecondsThenDone(), "==> exited with 0", "==> post-remove  echo bye", "bye", "==> exited with 0"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// Scenario "Protocol variables not passed" of hooks: the function sets
// WT_DIRECTIVE_CD_FILE and WT_PREVIOUS_DIR for the binary, which passes
// neither to a blocking or a background hook.
func TestHooksDoNotSeeTheProtocolVariables(t *testing.T) {
	sb, repo, _ := hookRepo(t)
	record := sb.Path("env.txt")
	recorder := `"` + recordEnv + `" "` + record + `"`
	userConfig(t, sb, "[hooks]\npost-create = ["+strconv.Quote(recorder)+"]\npost-cd = ["+strconv.Quote(recorder)+"]\n")
	directive := sb.Path("directive")
	sb.WriteFile(directive, "")
	sb.Setenv("WT_DIRECTIVE_CD_FILE", directive)
	sb.Setenv("WT_PREVIOUS_DIR", repo)

	for _, args := range [][]string{{"create", "rec"}, {"cd", "rec"}} {
		if r := wt(t, sb, repo, args...); r.code != 0 {
			t.Fatalf("wt %v: exit %d\nstderr: %s", args, r.code, r.stderr)
		}
	}
	waitFor(t, "both hooks to record their environment", func() bool {
		b, _ := os.ReadFile(record)
		return strings.Count(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n--\n") == 2
	})
	b, _ := os.ReadFile(record)
	for _, kv := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		key, _, _ := strings.Cut(kv, "=")
		for _, v := range []string{"WT_DIRECTIVE_CD_FILE", "WT_PREVIOUS_DIR"} {
			if strings.EqualFold(key, v) {
				t.Errorf("a hook received %s", kv)
			}
		}
	}
}

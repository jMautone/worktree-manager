package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs git in dir for a test, ignoring the user's signing config.
func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=relcheck", "GIT_AUTHOR_EMAIL=relcheck@example.com",
		"GIT_COMMITTER_NAME=relcheck", "GIT_COMMITTER_EMAIL=relcheck@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a real git repository whose main branch has one empty
// commit per subject, oldest first.
func newRepo(t *testing.T, subjects ...string) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	for _, s := range append([]string{"initial commit"}, subjects...) {
		gitT(t, dir, "commit", "-q", "--no-verify", "--allow-empty", "-m", s)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runT(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"publish"},
		{"merge", "-h"},
		{"merge", "--bogus"},
		{"merge", "HEAD~3"}, // a positional rev must not silently check HEAD
		{"pr", "--title", "ci(x): y"},
		{"pr", "--branch", "ci/x"},
		{"notes"},
	} {
		if code, _, stderr := runT(args...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Errorf("run(%v) = %d, %q; want 2 and usage", args, code, stderr)
		}
	}
}

func TestRunPR(t *testing.T) {
	dir := newRepo(t, "chore(ci): add dependabot config (#2)")
	writeFile(t, filepath.Join(dir, "openspec", "changes", "archive", "2026-09-30-walking-skeleton", "proposal.md"), "x")

	code, stdout, stderr := runT("pr", "--root", dir, "--base", "main",
		"--branch", "v0.1/walking-skeleton",
		"--title", "feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]")
	if code != 0 || stdout != "ok: publishes v0.1.0-alpha.1\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}

	code, stdout, _ = runT("pr", "--root", dir, "--base", "main",
		"--branch", "ci/release-conventions",
		"--title", "ci(release): add branch, title and release conventions")
	if code != 0 || stdout != "ok: publishes nothing\n" {
		t.Errorf("got %d, %q", code, stdout)
	}
}

func TestRunPRReadsPublishedVersionsFromBase(t *testing.T) {
	// The PR branch forked before the alpha landed on main, so only --base
	// knows about it.
	dir := newRepo(t)
	gitT(t, dir, "branch", "v0.1/shell-integration")
	gitT(t, dir, "commit", "-q", "--no-verify", "--allow-empty", "-m", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	gitT(t, dir, "switch", "-q", "v0.1/shell-integration")
	writeFile(t, filepath.Join(dir, "openspec", "changes", "shell-integration", "proposal.md"), "x")

	code, _, stderr := runT("pr", "--root", dir, "--base", "main",
		"--branch", "v0.1/shell-integration",
		"--title", "feat(shell-integration): add wt cd [v0.1.0-alpha.1]")
	if code != 1 || !strings.Contains(stderr, "relcheck: v0.1.0-alpha.1 is already published; the next alpha is v0.1.0-alpha.2") {
		t.Errorf("got %d, %q", code, stderr)
	}
}

func TestRunPRReportsBranchAndTitleTogether(t *testing.T) {
	dir := newRepo(t)
	code, _, stderr := runT("pr", "--root", dir, "--base", "main", "--branch", "feature/x", "--title", "Add stuff")
	if code != 1 || strings.Count(stderr, "relcheck: ") != 2 {
		t.Errorf("got %d, %q; want both errors, one per line", code, stderr)
	}
}

func TestRunPRRelease(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	args := []string{"pr", "--root", dir, "--base", "main", "--branch", "release/v0.1.0", "--title", "chore(release): close M1 [v0.1.0]"}

	if code, _, stderr := runT(args...); code != 1 || !strings.Contains(stderr, `no "## [0.1.0]" section`) {
		t.Errorf("without changelog: got %d, %q", code, stderr)
	}
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [0.1.0] — 2026-10-15\n\n- `wt list`.\n")
	if code, stdout, stderr := runT(args...); code != 0 || stdout != "ok: publishes v0.1.0\n" {
		t.Errorf("with changelog: got %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunMerge(t *testing.T) {
	cases := []struct {
		name     string
		subjects []string
		code     int
		stdout   string
		stderr   string
	}{
		{"publishes nothing", []string{"chore(ci): bump actions/checkout from 4 to 7 (#5)"}, 0, "", ""},
		{"first alpha", []string{"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)"}, 0, "v0.1.0-alpha.1\n", ""},
		{"second alpha", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"chore(ci): bump actions/checkout from 4 to 7 (#5)",
			"feat(shell-integration): add wt cd [v0.1.0-alpha.2] (#6)",
		}, 0, "v0.1.0-alpha.2\n", ""},
		{"duplicate", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"feat(shell-integration): add wt cd [v0.1.0-alpha.1] (#6)",
		}, 1, "", "already published"},
		{"final without changelog", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"chore(release): close M1 [v0.1.0] (#9)",
		}, 1, "", `no "## [0.1.0]" section`},
	}
	for _, c := range cases {
		dir := newRepo(t, c.subjects...)
		code, stdout, stderr := runT("merge", "--root", dir)
		if code != c.code || stdout != c.stdout || !strings.Contains(stderr, c.stderr) {
			t.Errorf("%s: got %d, %q, %q", c.name, code, stdout, stderr)
		}
	}
}

func TestRunMergeIsIdempotentAfterTagging(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	gitT(t, dir, "tag", "-a", "v0.1.0-alpha.1", "-m", "v0.1.0-alpha.1")

	// Re-running release.yml after GoReleaser failed must not fail on the tag
	// the first run already pushed.
	if code, stdout, stderr := runT("merge", "--root", dir); code != 0 || stdout != "v0.1.0-alpha.1\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunMergeRejectsTagOnAnotherCommit(t *testing.T) {
	dir := newRepo(t, "chore(ci): something (#4)", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	gitT(t, dir, "tag", "v0.1.0-alpha.1", "HEAD~1")

	if code, _, stderr := runT("merge", "--root", dir); code != 1 || !strings.Contains(stderr, "already exists on another commit") {
		t.Errorf("got %d, %q", code, stderr)
	}
}

func TestRunNotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] — 2026-10-15\n\n### Added\n\n- `wt list`.\n\n---\n\n## PowerShell 0.9.0\n")

	code, stdout, stderr := runT("notes", "--root", dir, "--version", "v0.1.0")
	if code != 0 || stdout != "### Added\n\n- `wt list`.\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}
	if code, _, _ := runT("notes", "--root", dir, "--version", "v0.2.0"); code != 1 {
		t.Errorf("missing section: got %d, want 1", code)
	}
}

func TestRunNotesRejectsAlphas(t *testing.T) {
	// Alpha notes are generated from the merged titles. Even a matching
	// section must not turn notes into a second source for them.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [0.1.0-alpha.1]\n\n- `wt list`.\n")

	code, stdout, stderr := runT("notes", "--root", dir, "--version", "v0.1.0-alpha.1")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "relcheck: v0.1.0-alpha.1 is an alpha") {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunPRReleaseRejectsEmptyChangelogSection(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	// [0.1.0] added above [Unreleased] instead of renaming it: no notes.
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [0.1.0] — 2026-10-15\n\n## [Unreleased]\n\n- `wt list`.\n")

	code, _, stderr := runT("pr", "--root", dir, "--base", "main", "--branch", "release/v0.1.0", "--title", "chore(release): close M1 [v0.1.0]")
	if code != 1 || !strings.Contains(stderr, `no "## [0.1.0]" section`) {
		t.Errorf("got %d, %q", code, stderr)
	}
}

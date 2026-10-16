package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

// Scenario "Words kept whole (macOS, Linux)": each word reaches the program
// as one argument, unchanged, through sh.
func TestEachWordsKeptWhole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh's quoting; see TestEachWordsKeptWholeWindows")
	}
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)

	r := wt(t, sb, repo, "each", printArgs, "a b", "a;b", "$HOME", "it's", "")
	if r.code != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", r.code, r.stderr)
	}
	want := "==> ^  " + testutil.Comparable(t, repo) + "\na b\na;b\n$HOME\nit's\n\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

// Scenarios "Words kept whole (Windows)" and "Environment variable
// reference (Windows)": each word reaches a program that splits its command
// line as the Microsoft C runtime does, as Go's does, as one argument,
// through cmd.exe, which only expands %NAME%.
func TestEachWordsKeptWholeWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only; see TestEachWordsKeptWhole")
	}
	sb := testutil.New(t)
	repo := sb.Path("repo")
	sb.InitRepo(repo)
	sb.Setenv("WT_TEST", "x")

	r := wt(t, sb, repo, "each", printArgs, "a b", "a&b", `say "hi"`, `C:\dir\`, "", "a %WT_TEST% b")
	if r.code != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", r.code, r.stderr)
	}
	want := "==> ^  " + testutil.Comparable(t, repo) + "\na b\na&b\nsay \"hi\"\nC:\\dir\\\n\na x b\n"
	if got := strings.ReplaceAll(r.stdout, "\r\n", "\n"); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

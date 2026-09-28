package git

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const head = "eb2ebf2c565f36af195d565dd2f4f3ed67a284bf"

func parseFixture(t *testing.T, name string) []WorktreeEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseWorktreeList(b)
	if err != nil {
		t.Fatalf("ParseWorktreeList(%s): %v", name, err)
	}
	return entries
}

func TestParseWorktreeListLinked(t *testing.T) {
	got := parseFixture(t, "linked.z")
	want := []WorktreeEntry{
		{Path: "/src/repo", Head: head, Branch: "main"},
		{Path: "/src/wt-det", Head: head, Detached: true},
		{Path: "/src/wt-feat", Head: head, Branch: "feature/abc1"},
		{Path: "/src/wt-lock", Head: head, Branch: "l1", Locked: true, LockedReason: "on usb drive"},
		{Path: "/src/wt-lock2", Head: head, Branch: "l2", Locked: true},
		{Path: "/src/wt-prune", Head: head, Branch: "p", Prunable: true, PrunableReason: "gitdir file points to non-existent location"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseWorktreeListBare(t *testing.T) {
	got := parseFixture(t, "bare.z")
	want := []WorktreeEntry{
		{Path: "/src/bare.git", Bare: true},
		{Path: "/src/bare-wt", Head: head, Branch: "main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseWorktreeListWindowsPaths(t *testing.T) {
	got := parseFixture(t, "windows.z")
	want := []WorktreeEntry{
		{Path: "C:/src/repo", Head: head, Branch: "main"},
		{Path: "C:/src/repo.worktrees/feature-abc1", Head: head, Branch: "feature/abc1", Locked: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseWorktreeListKeepsSpacesAndNewlinesInPaths(t *testing.T) {
	in := "worktree /src/my repo\nwith newline\x00HEAD " + head + "\x00branch refs/heads/main\x00\x00"
	got, err := ParseWorktreeList([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "/src/my repo\nwith newline" {
		t.Errorf("got %+v", got)
	}
}

func TestParseWorktreeListIgnoresUnknownAttributes(t *testing.T) {
	in := "worktree /src/repo\x00HEAD " + head + "\x00branch refs/heads/main\x00future-attribute x\x00\x00"
	got, err := ParseWorktreeList([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Branch != "main" {
		t.Errorf("got %+v", got)
	}
}

func TestParseWorktreeListRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{
		"HEAD " + head + "\x00\x00",      // record without a worktree line
		"worktree /src/repo\nHEAD x\n\n", // not -z output
	} {
		if _, err := ParseWorktreeList([]byte(in)); err == nil {
			t.Errorf("no error for %q", in)
		}
	}
}

func TestParseWorktreeListEmpty(t *testing.T) {
	got, err := ParseWorktreeList(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

package worktree

import (
	"reflect"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
)

func entries(paths ...string) []git.WorktreeEntry {
	var es []git.WorktreeEntry
	for _, p := range paths {
		es = append(es, git.WorktreeEntry{Path: p, Head: "0123456789abcdef0123456789abcdef01234567", Branch: "b"})
	}
	return es
}

// current returns the path of the current worktree, or "" if none.
func current(ws []Worktree) string {
	cur := ""
	for _, w := range ws {
		if w.Current {
			if cur != "" {
				return "MORE THAN ONE"
			}
			cur = w.Path
		}
	}
	return cur
}

func TestBuildCurrentWorktree(t *testing.T) {
	for _, tc := range []struct {
		name    string
		goos    string
		entries []git.WorktreeEntry
		cwd     string
		want    string
	}{
		{"cwd is the worktree root", "linux", entries("/a/repo", "/a/wt"), "/a/wt", "/a/wt"},
		{"cwd in a subdirectory", "linux", entries("/a/repo", "/a/wt"), "/a/wt/src/pkg", "/a/wt"},
		{"cwd with trailing separator", "linux", entries("/a/repo", "/a/wt"), "/a/wt/", "/a/wt"},
		{"prefix by components, not by string", "linux", entries("/a/repo", "/a/repo2"), "/a/repo2/x", "/a/repo2"},
		{"not inside a prefix-named sibling", "linux", entries("/a/repo2", "/a/repo"), "/a/repo/x", "/a/repo"},
		{"nested worktree: deepest wins", "linux", entries("/a/repo", "/a/repo/.worktrees/x"), "/a/repo/.worktrees/x/src", "/a/repo/.worktrees/x"},
		{"nested worktree: parent when outside the child", "linux", entries("/a/repo", "/a/repo/.worktrees/x"), "/a/repo/.worktrees", "/a/repo"},
		{"outside every worktree", "linux", entries("/a/repo", "/a/wt"), "/b", ""},
		{"case differs on linux: no match", "linux", entries("/a/repo"), "/A/Repo/sub", ""},
		{"case differs on macOS: match", "darwin", entries("/a/repo"), "/A/Repo/sub", "/a/repo"},
		{"case differs on windows: match", "windows", entries("C:/src/repo"), `c:\SRC\Repo\sub`, `C:\src\repo`},
		{"windows components", "windows", entries("C:/src/repo", "C:/src/repo2"), `C:\src\repo2`, `C:\src\repo2`},
		{"windows cwd with forward slashes", "windows", entries("C:/src/repo"), `C:/src/repo/x`, `C:\src\repo`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := current(Build(tc.entries, tc.cwd, tc.goos)); got != tc.want {
				t.Errorf("current = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildBareRepositoryAsCurrent(t *testing.T) {
	es := []git.WorktreeEntry{
		{Path: "/a/bare.git", Bare: true},
		{Path: "/a/wt", Head: "0123456789abcdef0123456789abcdef01234567", Branch: "main"},
	}
	ws := Build(es, "/a/bare.git/refs/heads", "linux")
	if current(ws) != "/a/bare.git" || !ws[0].Main || !ws[0].Bare {
		t.Errorf("got %+v, want the bare repository as current and main", ws)
	}
}

func TestBuildNativePaths(t *testing.T) {
	ws := Build(entries("C:/src/repo", "C:/src/repo.worktrees/feature-abc1"), `D:\elsewhere`, "windows")
	if ws[0].Path != `C:\src\repo` || ws[1].Path != `C:\src\repo.worktrees\feature-abc1` {
		t.Errorf("windows paths = %q, %q", ws[0].Path, ws[1].Path)
	}
	if ws[1].Name != "feature-abc1" {
		t.Errorf("windows name = %q", ws[1].Name)
	}
	ws = Build(entries("/src/repo", "/src/wt-a"), "/", "darwin")
	if ws[1].Path != "/src/wt-a" || ws[1].Name != "wt-a" {
		t.Errorf("unix path/name = %q, %q", ws[1].Path, ws[1].Name)
	}
}

func TestBuildMainIsFirstEntry(t *testing.T) {
	ws := Build(entries("/a/zzz-main", "/a/aaa"), "/", "linux")
	if !ws[0].Main || ws[0].Path != "/a/zzz-main" || ws[1].Main {
		t.Errorf("got %+v, want the first git entry as main and listed first", ws)
	}
}

func TestBuildOrder(t *testing.T) {
	ws := Build(entries("/r/main", "/r/zeta", "/r/Alpha", "/r/beta", "/q/beta"), "/", "linux")
	var got []string
	for _, w := range ws {
		got = append(got, w.Path)
	}
	want := []string{"/r/main", "/r/Alpha", "/q/beta", "/r/beta", "/r/zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestBuildCopiesState(t *testing.T) {
	es := []git.WorktreeEntry{
		{Path: "/a/repo", Head: "h1", Branch: "main"},
		{Path: "/a/det", Head: "h2", Detached: true, Locked: true, LockedReason: "on usb drive"},
		{Path: "/a/gone", Head: "h3", Branch: "x", Prunable: true, PrunableReason: "gitdir file points to non-existent location"},
	}
	ws := Build(es, "/a/det", "linux")
	want := []Worktree{
		{Name: "repo", Path: "/a/repo", Head: "h1", Branch: "main", Main: true},
		{Name: "det", Path: "/a/det", Head: "h2", Detached: true, Current: true, Locked: true, LockedReason: "on usb drive"},
		{Name: "gone", Path: "/a/gone", Head: "h3", Branch: "x", Prunable: true, PrunableReason: "gitdir file points to non-existent location"},
	}
	if !reflect.DeepEqual(ws, want) {
		t.Errorf("got\n%+v\nwant\n%+v", ws, want)
	}
}

func TestBuildEmpty(t *testing.T) {
	if ws := Build(nil, "/", "linux"); len(ws) != 0 {
		t.Errorf("got %+v", ws)
	}
}

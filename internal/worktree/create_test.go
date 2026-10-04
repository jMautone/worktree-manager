package worktree

import (
	"errors"
	"testing"
)

func TestValidateName(t *testing.T) {
	for _, name := range []string{"feat", "feature/abc1", "my task", "a..b", ".hidden", "x.", `fix\x`, "nul"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "../x", "a/../b", `..\x`, `a\.\b`, "x/.", "x/.."} {
		var e *InvalidNameError
		if err := ValidateName(name); !errors.As(err, &e) || e.Name != name {
			t.Errorf("ValidateName(%q) = %v, want an *InvalidNameError", name, err)
		}
	}
}

func TestBranchFor(t *testing.T) {
	for _, tc := range []struct{ name, flag, prefix, want string }{
		{"feature/abc1", "", "", "feature/abc1"},
		{"auth", "", "jm/", "jm/auth"},
		{"api", "feature/api-v2", "jm/", "feature/api-v2"},
		{"my task", "task", "", "task"},
	} {
		if got := BranchFor(tc.name, tc.flag, tc.prefix); got != tc.want {
			t.Errorf("BranchFor(%q, %q, %q) = %q, want %q", tc.name, tc.flag, tc.prefix, got, tc.want)
		}
	}
}

func TestPathVars(t *testing.T) {
	for _, tc := range []struct {
		goos                   string
		main                   Worktree
		repo, parent, repoPath string
	}{
		{"darwin", Worktree{Path: "/src/repo"}, "repo", "/src", "/src/repo"},
		{"linux", Worktree{Path: "/repo"}, "repo", "/", "/repo"},
		{"linux", Worktree{Path: "/src/proj.git", Bare: true}, "proj", "/src", "/src/proj.git"},
		{"linux", Worktree{Path: "/src/proj.git"}, "proj.git", "/src", "/src/proj.git"},
		{"linux", Worktree{Path: "/src/proj/.bare", Bare: true}, ".bare", "/src/proj", "/src/proj/.bare"},
		{"linux", Worktree{Path: "/src/.git", Bare: true}, ".git", "/src", "/src/.git"},
		{"windows", Worktree{Path: `C:\src\repo`}, "repo", `C:\src`, `C:\src\repo`},
		{"windows", Worktree{Path: `C:\repo`}, "repo", `C:\`, `C:\repo`},
		{"windows", Worktree{Path: `\\server\share\repo`}, "repo", `\\server\share\`, `\\server\share\repo`},
	} {
		got := PathVars(tc.main, "n", "b", tc.goos)
		if got["repo"] != tc.repo || got["repo_parent"] != tc.parent || got["repo_path"] != tc.repoPath ||
			got["name"] != "n" || got["branch"] != "b" {
			t.Errorf("%s %+v: %v", tc.goos, tc.main, got)
		}
	}
}

func TestResolvePath(t *testing.T) {
	for _, tc := range []struct {
		goos, rendered, repoPath, home, want string
	}{
		{"darwin", "/src/repo.worktrees/feat", "/src/repo", "/Users/me", "/src/repo.worktrees/feat"},
		{"linux", "/src/repo.worktrees/feat", "/src/repo", "/home/me", "/src/repo.worktrees/feat"},
		{"linux", "//src///repo.worktrees//feat/", "/src/repo", "/home/me", "/src/repo.worktrees/feat"},
		{"linux", "~", "/src/repo", "/home/me", "/home/me"},
		{"linux", "~/wt/repo/feat", "/src/repo", "/home/me/", "/home/me/wt/repo/feat"},
		{"linux", "~x/feat", "/src/repo", "/home/me", "/src/repo/~x/feat"},
		{"linux", `~\x`, "/src/repo", "/home/me", `/src/repo/~\x`},
		{"linux", ".worktrees/feat", "/src/repo", "/home/me", "/src/repo/.worktrees/feat"},
		{"linux", "../repo-feat", "/src/repo", "/home/me", "/src/repo-feat"},
		{"linux", "/src/repo/../repo-feat", "/src/repo", "/home/me", "/src/repo-feat"},
		{"linux", "/../../x", "/src/repo", "/home/me", "/x"},
		{"linux", "/src/./a/./b", "/src/repo", "/home/me", "/src/a/b"},
		{"darwin", `/src/a\b`, "/src/repo", "/Users/me", `/src/a\b`},

		{"windows", `C:\src/repo.worktrees/feat`, `C:\src\repo`, `C:\Users\me`, `C:\src\repo.worktrees\feat`},
		{"windows", `C:/src//repo.worktrees\\feat\`, `C:\src\repo`, `C:\Users\me`, `C:\src\repo.worktrees\feat`},
		{"windows", "~", `C:\src\repo`, `C:\Users\me`, `C:\Users\me`},
		{"windows", `~\wt\repo\feat`, `C:\src\repo`, `C:\Users\me`, `C:\Users\me\wt\repo\feat`},
		{"windows", "~/wt/repo/feat", `C:\src\repo`, `C:\Users\me`, `C:\Users\me\wt\repo\feat`},
		{"windows", ".worktrees/feat", `C:\src\repo`, `C:\Users\me`, `C:\src\repo\.worktrees\feat`},
		{"windows", `C:\src\repo/../repo-feat`, `C:\src\repo`, `C:\Users\me`, `C:\src\repo-feat`},
		{"windows", `C:\..\x`, `C:\src\repo`, `C:\Users\me`, `C:\x`},
		{"windows", `\wt\feat`, `D:\src\repo`, `C:\Users\me`, `D:\wt\feat`},
		{"windows", `d:/wt/feat`, `C:\src\repo`, `C:\Users\me`, `d:\wt\feat`},
		{"windows", `\\server\share\repo.worktrees\feat`, `\\server\share\repo`, `C:\Users\me`, `\\server\share\repo.worktrees\feat`},
		{"windows", `//server/share/../x`, `C:\src\repo`, `C:\Users\me`, `\\server\share\x`},
		{"windows", `x`, `\\server\share\repo`, `C:\Users\me`, `\\server\share\repo\x`},
		{"windows", `C:x`, `C:\src\repo`, `C:\Users\me`, `C:\src\repo\C:x`},
	} {
		got, err := ResolvePath(tc.rendered, tc.repoPath, tc.home, tc.goos)
		if err != nil || got != tc.want {
			t.Errorf("%s ResolvePath(%q, %q, %q) = %q, %v; want %q", tc.goos, tc.rendered, tc.repoPath, tc.home, got, err, tc.want)
		}
	}
	if _, err := ResolvePath("~/x", "/src/repo", "", "linux"); err == nil {
		t.Error("~ expanded without a home directory")
	}
}

func TestCheckWindowsPath(t *testing.T) {
	invalid := []string{
		`C:\src\nul`, `C:\src\NUL`, `C:\src\con.txt`, `C:\src\Com1\x`, `C:\src\lpt9.tar.gz`,
		`C:\src\a<b`, `C:\src\a>b`, `C:\src\a:b`, `C:\src\a"b`, `C:\src\a|b`, `C:\src\a?b`, `C:\src\a*b`,
		"C:\\src\\a\tb", "C:\\src\\a\x7fb",
		`C:\src\x.`, `C:\src\x `, `C:\src\x.\y`, `C:\src\repo\C:x`,
		`\\server\share\aux`,
	}
	valid := []string{
		`C:\src\repo.worktrees\feat`, `C:\src\nul-`, `C:\src\Con-.txt`, `C:\src\console`, `C:\src\com10`,
		`C:\src\my task`, `C:\src\café`, `C:\`, `\\server\share`, `\\server\share\x`, `C:\src\.hidden`,
	}
	for _, p := range invalid {
		var e *InvalidPathError
		if err := CheckWindowsPath(p, "windows"); !errors.As(err, &e) || e.Path != p {
			t.Errorf("windows %q: %v, want an *InvalidPathError", p, err)
		}
		for _, goos := range []string{"darwin", "linux"} {
			if err := CheckWindowsPath(p, goos); err != nil {
				t.Errorf("%s %q: %v, want no check", goos, p, err)
			}
		}
	}
	for _, p := range valid {
		if err := CheckWindowsPath(p, "windows"); err != nil {
			t.Errorf("windows %q: %v", p, err)
		}
	}
	if err := CheckWindowsPath("/src/nul", "darwin"); err != nil {
		t.Errorf("darwin /src/nul: %v", err)
	}
}

func TestRegisteredAt(t *testing.T) {
	ws := []Worktree{
		{Name: "repo", Path: "/src/repo", Main: true},
		{Name: "feat", Path: "/src/repo.worktrees/feat", Prunable: true},
	}
	for _, tc := range []struct {
		goos, path string
		want       bool
	}{
		{"linux", "/src/repo.worktrees/feat", true},
		{"linux", "/src/repo.worktrees/Feat", false},
		{"darwin", "/src/repo.worktrees/Feat", true},
		{"linux", "/src/repo.worktrees", false},
		{"linux", "/src/repo.worktrees/feat/x", false},
	} {
		if _, got := RegisteredAt(ws, tc.path, tc.goos); got != tc.want {
			t.Errorf("%s %s: %v, want %v", tc.goos, tc.path, got, tc.want)
		}
	}
	win := []Worktree{{Name: "feat", Path: `C:\src\repo.worktrees\feat`}}
	if w, ok := RegisteredAt(win, `c:\SRC\repo.worktrees\FEAT`, "windows"); !ok || w.Name != "feat" {
		t.Errorf("windows: %v %v", w, ok)
	}
}

func TestDefaultBranch(t *testing.T) {
	for _, tc := range []struct {
		name       string
		originHEAD string
		main       Worktree
		bareHEAD   string
		want       string
		ok         bool
	}{
		{"origin/HEAD", "origin/main", Worktree{Branch: "trunk"}, "", "origin/main", true},
		{"no origin/HEAD: main worktree's branch", "", Worktree{Branch: "trunk"}, "", "trunk", true},
		{"detached main worktree", "", Worktree{Detached: true}, "", "", false},
		{"detached main worktree with origin/HEAD", "origin/main", Worktree{Detached: true}, "", "origin/main", true},
		{"bare", "", Worktree{Bare: true}, "main", "main", true},
		{"bare with origin/HEAD", "origin/dev", Worktree{Bare: true}, "main", "origin/dev", true},
		{"bare with detached HEAD", "", Worktree{Bare: true}, "", "", false},
	} {
		got, ok := DefaultBranch(tc.originHEAD, tc.main, tc.bareHEAD)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFetchRemote(t *testing.T) {
	remotes := []string{"origin", "origin/fork", "upstream"}
	for _, tc := range []struct {
		base, want string
	}{
		{"origin/main", "origin"},
		{"origin/fork/main", "origin/fork"},
		{"upstream/release/1.0", "upstream"},
		{"main", ""},
		{"origin", ""},
		{"originx/main", ""},
		{"v1.0", ""},
	} {
		got, ok := FetchRemote(tc.base, remotes)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("FetchRemote(%q) = %q, %v; want %q", tc.base, got, ok, tc.want)
		}
	}
	if _, ok := FetchRemote("origin/main", nil); ok {
		t.Error("a remote with no remotes configured")
	}
}

func TestBranchTaken(t *testing.T) {
	ws := []Worktree{
		{Name: "repo", Path: "/src/repo", Branch: "main", Main: true},
		{Name: "feat", Path: "/src/repo.worktrees/feat", Branch: "feat"},
	}
	refs := []string{"refs/heads/main", "refs/heads/feat", "refs/heads/free", "refs/remotes/origin/main", "refs/remotes/origin/fork/x", "refs/remotes/origin/pushed", "refs/tags/v1"}
	remotes := []string{"origin", "origin/fork"}

	check := func(branch string) *BranchExistsError {
		t.Helper()
		err := BranchTaken(branch, refs, remotes, ws)
		if err == nil {
			return nil
		}
		var e *BranchExistsError
		if !errors.As(err, &e) {
			t.Fatalf("%s: %v, want a *BranchExistsError", branch, err)
		}
		return e
	}

	if e := check("free"); e == nil || e.Remote != "" || e.Worktree != nil {
		t.Errorf("free: %+v, want local and in no worktree", e)
	}
	if e := check("feat"); e == nil || e.Worktree == nil || e.Worktree.Name != "feat" {
		t.Errorf("feat: %+v, want checked out in worktree feat", e)
	}
	if e := check("pushed"); e == nil || e.Remote != "origin" {
		t.Errorf("pushed: %+v, want remote origin", e)
	}
	if e := check("x"); e == nil || e.Remote != "origin/fork" {
		t.Errorf("x: %+v, want remote origin/fork", e)
	}
	if e := check("new"); e != nil {
		t.Errorf("new: %+v", e)
	}
	if e := check("v1"); e != nil {
		t.Errorf("a tag is not a branch: %+v", e)
	}
	if e := check("main"); e == nil || e.Remote != "" {
		t.Errorf("main: %+v, want the local branch first", e)
	}
}

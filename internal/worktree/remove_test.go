package worktree

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
)

var allOSes = []string{"darwin", "linux", "windows"}

// osPath writes a /-separated absolute path in goos's form: on windows it
// gains the drive C:.
func osPath(p, goos string) string {
	if goos == "windows" {
		return native("C:"+p, goos)
	}
	return p
}

func TestContains(t *testing.T) {
	for _, goos := range allOSes {
		for _, tc := range []struct {
			dir, p string
			want   bool
		}{
			{"/a/repo", "/a/repo", true},
			{"/a/repo", "/a/repo/x/y", true},
			{"/a/repo/", "/a/repo/x", true},
			{"/a/repo", "/a/repo2", false},
			{"/a/repo", "/a/repo2/x", false},
			{"/a/repo/x", "/a/repo", false},
			{"/a/repo", "/b/repo", false},
			{"/a/repo", "/a/Repo/x", goos != "linux"},
		} {
			dir, p := osPath(tc.dir, goos), osPath(tc.p, goos)
			if got := Contains(dir, p, goos); got != tc.want {
				t.Errorf("%s: Contains(%q, %q) = %v, want %v", goos, dir, p, got, tc.want)
			}
		}
	}
	// On windows, git's forward slashes are native too.
	if !Contains("C:/a/repo", `C:\a\repo\x`, "windows") {
		t.Error("windows: forward slashes in dir are not separators")
	}
}

// removable builds the worktrees of a repository at /a/repo, with the
// linked worktrees at the given /-separated paths, in goos's form.
func removable(goos string, linked ...string) []Worktree {
	es := []git.WorktreeEntry{{Path: "/a/repo", Head: "c0", Branch: "main"}}
	for _, p := range linked {
		es = append(es, git.WorktreeEntry{Path: p, Head: "c1", Branch: "b"})
	}
	for i := range es {
		if goos == "windows" {
			es[i].Path = "C:" + es[i].Path
		}
	}
	return Build(es, "/elsewhere", goos)
}

func byName(ws []Worktree, name string) Worktree {
	for _, w := range ws {
		if w.Name == name {
			return w
		}
	}
	panic("no worktree " + name)
}

func TestCheckRemovable(t *testing.T) {
	for _, goos := range allOSes {
		t.Run(goos, func(t *testing.T) {
			ws := removable(goos, "/a/feat", "/a/repo2", "/a/outer", "/a/outer/inner", "/a/case", "/a/CASE/x")
			bare := Build([]git.WorktreeEntry{{Path: "/a/repo.git", Bare: true}, {Path: "/a/wt", Head: "c1"}}, "/", goos)

			if err := CheckRemovable(byName(ws, "feat"), ws, goos); err != nil {
				t.Errorf("feat: %v", err)
			}
			// /a/repo does not contain /a/repo2: the main worktree is
			// rejected for being main, and repo2 is not inside it.
			var me *MainWorktreeError
			for _, w := range []Worktree{byName(ws, "repo"), bare[0]} {
				if err := CheckRemovable(w, ws, goos); !errors.As(err, &me) || err.Error() != "cannot remove the main worktree" {
					t.Errorf("%s: %v, want *MainWorktreeError", w.Name, err)
				}
			}
			if err := CheckRemovable(byName(ws, "repo2"), ws, goos); err != nil {
				t.Errorf("repo2: %v", err)
			}

			var ce *ContainsError
			err := CheckRemovable(byName(ws, "outer"), ws, goos)
			if !errors.As(err, &ce) || ce.Outer.Name != "outer" || ce.Inner.Name != "inner" {
				t.Errorf("outer: %v, want *ContainsError naming inner", err)
			} else if want := `worktree "outer" contains the worktree "inner" at ` + osPath("/a/outer/inner", goos); err.Error() != want {
				t.Errorf("message %q, want %q", err, want)
			}
			if err := CheckRemovable(byName(ws, "inner"), ws, goos); err != nil {
				t.Errorf("inner: %v", err)
			}

			// /a/CASE/x is inside /a/case where the filesystem ignores case.
			err = CheckRemovable(byName(ws, "case"), ws, goos)
			if blocks := errors.As(err, &ce); blocks != (goos != "linux") {
				t.Errorf("case: %v; want a block only on darwin and windows", err)
			}
		})
	}
}

func TestCheckRemovableLocked(t *testing.T) {
	for _, tc := range []struct {
		reason, want string
	}{
		{"on usb drive", `worktree "feat" is locked: on usb drive`},
		{"", `worktree "feat" is locked`},
	} {
		ws := removable("linux", "/a/feat")
		ws[1].Locked, ws[1].LockedReason = true, tc.reason
		var le *LockedError
		if err := CheckRemovable(ws[1], ws, "linux"); !errors.As(err, &le) || err.Error() != tc.want {
			t.Errorf("%v, want %q", err, tc.want)
		}
	}
}

func TestCheckRemovableOrder(t *testing.T) {
	ws := removable("linux", "/a/outer", "/a/outer/inner")
	outer := byName(ws, "outer")
	outer.Locked = true
	// Locked and nested: the lock is reported.
	var le *LockedError
	if err := CheckRemovable(outer, ws, "linux"); !errors.As(err, &le) {
		t.Errorf("locked and nested: %v, want *LockedError", err)
	}
	// Main, locked and containing others: main is reported.
	main := byName(ws, "repo")
	main.Locked = true
	ws = removable("linux", "/a/repo/.worktrees/x")
	var me *MainWorktreeError
	if err := CheckRemovable(main, ws, "linux"); !errors.As(err, &me) {
		t.Errorf("main: %v, want *MainWorktreeError", err)
	}
	// With several nested worktrees, the first in Build's order.
	ws = removable("linux", "/a/outer", "/a/outer/zeta", "/a/outer/alpha")
	var ce *ContainsError
	if err := CheckRemovable(byName(ws, "outer"), ws, "linux"); !errors.As(err, &ce) || ce.Inner.Name != "alpha" {
		t.Errorf("several nested: %v, want alpha", err)
	}
}

func TestBranchOutcome(t *testing.T) {
	type out struct {
		del    bool
		reason string
	}
	base := func(in, up bool) Merged { return Merged{Base: "origin/main", InBase: in, InUpstream: up} }
	for _, tc := range []struct {
		name              string
		branch            string
		keep, forceDelete bool
		m                 Merged
		want              out
	}{
		{"default, merged into neither", "feat", false, false, base(false, false), out{false, "not merged into origin/main"}},
		{"default, in the base", "feat", false, false, base(true, false), out{true, ""}},
		{"default, in the upstream", "feat", false, false, base(false, true), out{true, ""}},
		{"default, in both", "feat", false, false, base(true, true), out{true, ""}},
		{"keep, merged into neither", "feat", true, false, base(false, false), out{false, ""}},
		{"keep, in the base", "feat", true, false, base(true, false), out{false, ""}},
		{"keep, in the upstream", "feat", true, false, base(false, true), out{false, ""}},
		{"keep, in both", "feat", true, false, base(true, true), out{false, ""}},
		{"-D, merged into neither", "feat", false, true, base(false, false), out{true, ""}},
		{"-D, in the base", "feat", false, true, base(true, false), out{true, ""}},
		{"-D, in the upstream", "feat", false, true, base(false, true), out{true, ""}},
		{"-D, in both", "feat", false, true, base(true, true), out{true, ""}},
		{"no base, not in the upstream", "feat", false, false, Merged{}, out{false, "not merged"}},
		{"no base, in the upstream", "feat", false, false, Merged{InUpstream: true}, out{true, ""}},
		{"detached", "", false, false, base(true, true), out{false, ""}},
		{"detached with -D", "", false, true, base(false, false), out{false, ""}},
		{"detached with --keep-branch", "", true, false, base(false, false), out{false, ""}},
	} {
		del, reason := BranchOutcome(tc.branch, tc.keep, tc.forceDelete, tc.m)
		if got := (out{del, reason}); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCheckLockable(t *testing.T) {
	ws := removable("linux", "/a/feat", "/a/usb", "/a/plain")
	for i := range ws {
		switch ws[i].Name {
		case "usb":
			ws[i].Locked, ws[i].LockedReason = true, "review"
		case "plain":
			ws[i].Locked = true
		}
	}

	var me *MainWorktreeError
	if err := CheckLockable(ws[0]); !errors.As(err, &me) || err.Error() != "the main worktree cannot be locked" {
		t.Errorf("lock main: %v", err)
	}
	if err := CheckUnlockable(ws[0]); !errors.As(err, &me) || err.Error() != "the main worktree cannot be unlocked" {
		t.Errorf("unlock main: %v", err)
	}
	if err := CheckLockable(byName(ws, "feat")); err != nil {
		t.Errorf("lock feat: %v", err)
	}
	var le *LockedError
	if err := CheckLockable(byName(ws, "usb")); !errors.As(err, &le) || err.Error() != `worktree "usb" is already locked: review` {
		t.Errorf("lock usb: %v", err)
	}
	if err := CheckLockable(byName(ws, "plain")); !errors.As(err, &le) || err.Error() != `worktree "plain" is already locked` {
		t.Errorf("lock plain: %v", err)
	}
	var nle *NotLockedError
	if err := CheckUnlockable(byName(ws, "feat")); !errors.As(err, &nle) || err.Error() != `worktree "feat" is not locked` {
		t.Errorf("unlock feat: %v", err)
	}
	if err := CheckUnlockable(byName(ws, "usb")); err != nil {
		t.Errorf("unlock usb: %v", err)
	}
}

func TestPrunable(t *testing.T) {
	es := []git.WorktreeEntry{
		{Path: "/a/repo", Head: "c0", Branch: "main"},
		{Path: "/a/zeta", Head: "c1", Prunable: true, PrunableReason: "gitdir file points to non-existent location"},
		{Path: "/a/live", Head: "c1"},
		{Path: "/a/usb", Head: "c1", Prunable: true, Locked: true},
		{Path: "/a/alpha", Head: "c1", Prunable: true},
	}
	ws := Build(es, "/", "linux")
	var names []string
	for _, w := range Prunable(ws) {
		names = append(names, w.Name)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Prunable = %q, want %q", names, want)
	}
	if got := Prunable(ws[:1]); got != nil {
		t.Errorf("nothing prunable: %v", got)
	}
}

package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func TestRoots(t *testing.T) {
	for _, tc := range []struct {
		goos, home, raw string
		want            string // "" when the root is skipped
	}{
		{"darwin", "/Users/me", "~", "/Users/me"},
		{"linux", "/home/me", "~", "/home/me"},
		{"windows", `C:\Users\me`, "~", `C:\Users\me`},
		{"darwin", "/Users/me", "~/GIT", "/Users/me/GIT"},
		{"darwin", "/Users/me/", "~/GIT", "/Users/me/GIT"},
		{"windows", `C:\Users\me`, "~/GIT", `C:\Users\me\GIT`},
		{"windows", `C:\Users\me`, `~\GIT`, `C:\Users\me\GIT`},
		{"darwin", "/Users/me", `~\GIT`, `~\GIT`},
		{"linux", "/home/me", `~\GIT`, `~\GIT`},
		{"darwin", "/Users/me", "~GIT", "~GIT"},
		{"windows", `C:\Users\me`, "C:/Repos", `C:\Repos`},
		{"windows", `C:\Users\me`, `C:\Repos\`, `C:\Repos`},
		{"windows", `C:\Users\me`, `C:\`, `C:\`},
		{"windows", `C:\Users\me`, `C:\a\\b\..\c\.`, `C:\a\c`},
		{"windows", `C:\Users\me`, `\\srv\share\repos\..\x`, `\\srv\share\x`},
		{"windows", `C:\Users\me`, `\\srv\share`, `\\srv\share\`},
		{"darwin", "/Users/me", "/a//b/../c", "/a/c"},
		{"linux", "/home/me", "/a/b/./", "/a/b"},
		{"darwin", "/Users/me", "/", "/"},
		{"darwin", "", "/a//b/../c", "/a/c"},
		{"windows", "", `D:\Work`, `D:\Work`},
	} {
		roots, warnings := Roots([]string{tc.raw}, tc.goos, tc.home)
		want := []Root{{Raw: tc.raw, Path: tc.want}}
		if !reflect.DeepEqual(roots, want) || len(warnings) != 0 {
			t.Errorf("%s, home %q: Roots(%q) = %+v, %q; want %+v and no warning", tc.goos, tc.home, tc.raw, roots, warnings, want)
		}
	}
}

func TestRootsWithoutHome(t *testing.T) {
	for _, tc := range []struct {
		goos, raw, variable string
	}{
		{"darwin", "~", "HOME"},
		{"linux", "~/GIT", "HOME"},
		{"windows", `~\Repos`, "USERPROFILE"},
	} {
		roots, warnings := Roots([]string{tc.raw, "/x"}, tc.goos, "")
		if len(roots) != 1 || roots[0].Raw != "/x" {
			t.Errorf("%s: Roots(%q, /x) = %+v, want only /x", tc.goos, tc.raw, roots)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], tc.raw) || !strings.Contains(warnings[0], tc.variable+" is not set") {
			t.Errorf("%s: warnings = %q, want one naming %s and %s", tc.goos, warnings, tc.raw, tc.variable)
		}
	}
}

func TestRootsKeepsOrderAndRepeats(t *testing.T) {
	roots, _ := Roots([]string{"/b", "~/a", "/b"}, "darwin", "/h")
	want := []Root{{"/b", "/b"}, {"~/a", "/h/a"}, {"/b", "/b"}}
	if !reflect.DeepEqual(roots, want) {
		t.Errorf("Roots = %+v, want %+v", roots, want)
	}
	if roots, warnings := Roots(nil, "darwin", "/h"); roots != nil || warnings != nil {
		t.Errorf("Roots(nil) = %+v, %q", roots, warnings)
	}
}

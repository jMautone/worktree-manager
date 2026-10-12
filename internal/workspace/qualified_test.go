package workspace

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSplitQualified(t *testing.T) {
	for _, tc := range []struct {
		target, repo, name string
		qualified          bool
	}{
		{"api:feat", "api", "feat", true},
		{"api:", "api", "", true},
		{":feat", "", "feat", true},
		{":", "", "", true},
		{"api:a:b", "api", "a:b", true},
		{"feat", "", "", false},
	} {
		repo, name, qualified := SplitQualified(tc.target)
		if repo != tc.repo || name != tc.name || qualified != tc.qualified {
			t.Errorf("SplitQualified(%q) = %q, %q, %v; want %q, %q, %v", tc.target, repo, name, qualified, tc.repo, tc.name, tc.qualified)
		}
	}
}

func TestMatchSide(t *testing.T) {
	twoRoots := repos("darwin", "/Users/me/Repos/api", "/Users/me/Documents/GIT/api", "/Users/me/Documents/GIT/TrendFisher")
	windows := repos("windows", `C:\Repos\api`, `D:\GIT\api`)
	for _, tc := range []struct {
		name  string
		repos []Repo
		side  string
		goos  string
		want  string
	}{
		{"exact name", twoRoots, "TrendFisher", "darwin", "/Users/me/Documents/GIT/TrendFisher"},
		{"name ignoring case", twoRoots, "trendfisher", "darwin", "/Users/me/Documents/GIT/TrendFisher"},
		{"name by prefix", twoRoots, "trend", "darwin", "/Users/me/Documents/GIT/TrendFisher"},
		{"suffix", twoRoots, "GIT/api", "darwin", "/Users/me/Documents/GIT/api"},
		{"other suffix", twoRoots, "Repos/api", "darwin", "/Users/me/Repos/api"},
		{"suffix ignoring case", twoRoots, "git/API", "darwin", "/Users/me/Documents/GIT/api"},
		{"repeated and edge separators", twoRoots, "/GIT//api/", "darwin", "/Users/me/Documents/GIT/api"},
		{"whole path", twoRoots, "/Users/me/Repos/api", "darwin", "/Users/me/Repos/api"},
		{"backslash suffix on windows", windows, `GIT\api`, "windows", `D:\GIT\api`},
		{"slash suffix on windows", windows, "GIT/api", "windows", `D:\GIT\api`},
		{"suffix with the drive on windows", windows, `c:/repos/api`, "windows", `C:\Repos\api`},
	} {
		got, err := MatchSide(tc.repos, tc.side, tc.goos)
		if err != nil || got.Path != tc.want {
			t.Errorf("%s: MatchSide(%q) = %q, %v; want %q", tc.name, tc.side, got.Path, err, tc.want)
		}
	}
}

func TestMatchSideNotFound(t *testing.T) {
	darwin := repos("darwin", "/Users/me/Documents/GIT/api", "/Users/me/Documents/GIT/web")
	for _, tc := range []struct {
		name  string
		repos []Repo
		side  string
		goos  string
	}{
		{"a suffix is not a prefix", darwin, "GIT/ap", "darwin"},
		{"a component is whole", darwin, "IT/api", "darwin"},
		{"backslash is not a separator on darwin", darwin, `GIT\api`, "darwin"},
		{"longer than the path", darwin, "/x/Users/me/Documents/GIT/api", "darwin"},
		{"only separators", darwin, "//", "darwin"},
		{"empty side", darwin, "", "darwin"},
	} {
		_, err := MatchSide(tc.repos, tc.side, tc.goos)
		var nf *NotFoundError
		if !errors.As(err, &nf) || nf.Target != tc.side {
			t.Errorf("%s: MatchSide(%q) error = %v, want not found", tc.name, tc.side, err)
		}
	}
}

func TestMatchSideAmbiguous(t *testing.T) {
	rs := repos("darwin", "/a/work/api", "/b/work/api", "/c/oss/api")
	for _, tc := range []struct {
		side string
		want []string
	}{
		{"work/api", []string{"/a/work/api", "/b/work/api"}},
		{"api", []string{"/a/work/api", "/b/work/api", "/c/oss/api"}},
	} {
		_, err := MatchSide(rs, tc.side, "darwin")
		var amb *AmbiguousError
		if !errors.As(err, &amb) || amb.Target != tc.side || !reflect.DeepEqual(paths(amb.Candidates), tc.want) {
			t.Errorf("MatchSide(%q) error = %v, want an ambiguity among %q", tc.side, err, tc.want)
		}
	}
}

func TestDisplayNames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		repos []Repo
		goos  string
		want  []string
	}{
		{"unique names", repos("darwin", "/r/api", "/r/web"), "darwin", []string{"api", "web"}},
		{"same name under two roots", repos("darwin", "/Users/me/Repos/api", "/Users/me/Documents/GIT/api", "/Users/me/Repos/web"), "darwin",
			[]string{"GIT/api", "Repos/api", "web"}},
		{"organisation folders", repos("linux", "/r/work/api", "/r/oss/api"), "linux", []string{"oss/api", "work/api"}},
		{"names that differ only in case", repos("darwin", "/one/Api", "/two/api"), "darwin", []string{"Api", "api"}},
		{"forward slashes on windows", repos("windows", `C:\Repos\api`, `D:\GIT\api`), "windows", []string{"Repos/api", "GIT/api"}},
		{"no distinguishing suffix", repos("darwin", "/x/api", "/y/x/api"), "darwin", []string{"/x/api", "y/x/api"}},
		{"same parent ignoring case", repos("darwin", "/a/GIT/api", "/b/git/api", "/c/api"), "darwin", []string{"a/GIT/api", "b/git/api", "c/api"}},
	} {
		got := DisplayNames(tc.repos, tc.goos)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: DisplayNames = %q, want %q", tc.name, got, tc.want)
			continue
		}
		// Typed back, a display name with / leads to its repository.
		for i, name := range got {
			if name == tc.repos[i].Path || !strings.Contains(name, "/") {
				continue
			}
			if r, err := MatchSide(tc.repos, name, tc.goos); err != nil || r.Path != tc.repos[i].Path {
				t.Errorf("%s: MatchSide(%q) = %q, %v; want %q", tc.name, name, r.Path, err, tc.repos[i].Path)
			}
		}
	}
}

func TestRepositoryErrorsQuoteAsTyped(t *testing.T) {
	if got := (&NotFoundError{Target: `GIT\api`}).Error(); got != `no repository named "GIT\api"` {
		t.Errorf("NotFoundError = %q", got)
	}
	if got := (&AmbiguousError{Target: `a\b`}).Error(); got != `"a\b" matches more than one repository` {
		t.Errorf("AmbiguousError = %q", got)
	}
}

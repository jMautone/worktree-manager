package workspace

import (
	"errors"
	"reflect"
	"testing"
)

func id(n uint64) FileID { return FileID{Dev: 1, Ino: n} }

// paths returns the paths of repos, for comparisons.
func paths(repos []Repo) []string {
	var ps []string
	for _, r := range repos {
		ps = append(ps, r.Path)
	}
	return ps
}

func TestListDeduplicatesByIdentity(t *testing.T) {
	// The walk's order: root /w first, then /d (which contains /w), and
	// within /d, by name.
	found := []Found{
		{Path: "/w/api", Root: "/w", ID: id(1), GitdirID: id(11)},
		{Path: "/d/web", Root: "/d", ID: id(2)},
		{Path: "/d/w/api", Root: "/d", ID: id(1), GitdirID: id(11)},
		{Path: "/d/zz-web", Root: "/d", ID: id(2)},
		{Path: "/d/other/api", Root: "/d", ID: id(3)},
	}
	got := List(found, "darwin")
	want := []Repo{
		{Name: "api", Path: "/d/other/api", Root: "/d", ID: id(3)},
		{Name: "api", Path: "/w/api", Root: "/w", ID: id(1), GitdirID: id(11)},
		{Name: "web", Path: "/d/web", Root: "/d", ID: id(2)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List =\n%+v\nwant\n%+v", got, want)
	}
}

func TestListOrder(t *testing.T) {
	found := []Found{
		{Path: "/r/zeta", ID: id(1)},
		{Path: "/r/Alpha", ID: id(2)},
		{Path: "/r/beta", ID: id(3)},
		{Path: "/r/work/api", ID: id(4)},
		{Path: "/r/oss/api", ID: id(5)},
	}
	want := []string{"/r/Alpha", "/r/oss/api", "/r/work/api", "/r/beta", "/r/zeta"}
	if got := paths(List(found, "linux")); !reflect.DeepEqual(got, want) {
		t.Errorf("List order = %q, want %q", got, want)
	}
	if got := List(nil, "darwin"); got == nil || len(got) != 0 {
		t.Errorf("List(nil) = %#v, want an empty, non-nil slice", got)
	}
}

func TestListNames(t *testing.T) {
	for _, tc := range []struct {
		goos, path, want string
	}{
		{"darwin", "/r/api", "api"},
		{"linux", "/r/org/.dotfiles", ".dotfiles"},
		{"windows", `C:\Repos\api`, "api"},
		{"windows", `\\srv\share\web`, "web"},
	} {
		if got := List([]Found{{Path: tc.path, ID: id(1)}}, tc.goos)[0].Name; got != tc.want {
			t.Errorf("%s: name of %s = %q, want %q", tc.goos, tc.path, got, tc.want)
		}
	}
}

func TestCurrent(t *testing.T) {
	repos := []Repo{
		{Name: "api", ID: id(1), GitdirID: id(11)},
		{Name: "proj", ID: id(2), GitdirID: id(12)},
	}
	for _, tc := range []struct {
		name string
		id   FileID
		want int
	}{
		{"the repository's directory", id(1), 0},
		{"the bare repository of the .bare layout", id(12), 1},
		{"another directory", id(9), -1},
		{"no identity", FileID{}, -1},
	} {
		if got := Current(repos, tc.id); got != tc.want {
			t.Errorf("%s: Current = %d, want %d", tc.name, got, tc.want)
		}
	}
	// A repository whose git directory could not be identified does not
	// match a zero identity.
	if got := Current([]Repo{{ID: id(1)}}, FileID{}); got != -1 {
		t.Errorf("Current with zero identities = %d, want -1", got)
	}
}

func repos(goos string, ps ...string) []Repo {
	var found []Found
	for i, p := range ps {
		found = append(found, Found{Path: p, ID: id(uint64(i + 1))})
	}
	return List(found, goos)
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		repos  []Repo
		target string
		want   string
	}{
		{"exact before ignoring case", repos("darwin", "/one/Api", "/two/api"), "api", "/two/api"},
		{"ignoring case when nothing is exact", repos("darwin", "/one/Api", "/two/web"), "api", "/one/Api"},
		{"exact before prefix", repos("darwin", "/r/api", "/r/api-gateway"), "api", "/r/api"},
		{"letter case ignored", repos("darwin", "/r/TrendFisher", "/r/api"), "trendfisher", "/r/TrendFisher"},
		{"unique prefix", repos("darwin", "/r/TrendFisher", "/r/api"), "trend", "/r/TrendFisher"},
		{"prefix in upper case", repos("darwin", "/r/trendfisher"), "TREND", "/r/trendfisher"},
		{"non-ASCII prefix", repos("darwin", "/r/Ñandú", "/r/nada"), "ñan", "/r/Ñandú"},
		{"prefix with a rune of another length", repos("darwin", "/r/ſtore", "/r/web"), "ST", "/r/ſtore"},
		{"Kelvin sign", repos("darwin", "/r/kube"), "\u212Au", "/r/kube"},
		{"windows names", repos("windows", `C:\Repos\TrendFisher`), "trend", `C:\Repos\TrendFisher`},
	} {
		got, err := Match(tc.repos, tc.target)
		if err != nil || got.Path != tc.want {
			t.Errorf("%s: Match(%q) = %q, %v; want %q", tc.name, tc.target, got.Path, err, tc.want)
		}
	}
}

func TestMatchAmbiguous(t *testing.T) {
	for _, tc := range []struct {
		name   string
		repos  []Repo
		target string
		want   []string
	}{
		{"prefix", repos("darwin", "/r/cv-scorer-front", "/r/cv-scorer-backend", "/r/web"), "cv", []string{"/r/cv-scorer-backend", "/r/cv-scorer-front"}},
		{"same name", repos("darwin", "/r/work/api", "/r/oss/api"), "api", []string{"/r/oss/api", "/r/work/api"}},
		{"ignoring case", repos("darwin", "/r/API", "/s/Api"), "api", []string{"/r/API", "/s/Api"}},
	} {
		_, err := Match(tc.repos, tc.target)
		var amb *AmbiguousError
		if !errors.As(err, &amb) || amb.Target != tc.target || !reflect.DeepEqual(paths(amb.Candidates), tc.want) {
			t.Errorf("%s: Match(%q) error = %v, want an ambiguity among %q", tc.name, tc.target, err, tc.want)
			continue
		}
		if want := `"` + tc.target + `" matches more than one repository`; amb.Error() != want {
			t.Errorf("%s: message %q, want %q", tc.name, amb.Error(), want)
		}
	}
}

func TestMatchNotFound(t *testing.T) {
	rs := repos("darwin", "/r/api", "/r/web")
	for _, target := range []string{"", "nope", "apix", "pi"} {
		_, err := Match(rs, target)
		var nf *NotFoundError
		if !errors.As(err, &nf) || nf.Target != target {
			t.Errorf("Match(%q) error = %v, want not found", target, err)
		}
	}
	if _, err := Match(nil, "api"); err == nil {
		t.Error("Match among no repositories found one")
	}
	if got := (&NotFoundError{Target: "x"}).Error(); got != `no repository named "x"` {
		t.Errorf("message %q", got)
	}
}

func TestHasPrefixFold(t *testing.T) {
	for _, tc := range []struct {
		s, prefix string
		want      bool
	}{
		{"TrendFisher", "trend", true},
		{"TrendFisher", "", true},
		{"", "", true},
		{"", "a", false},
		{"api", "apix", false},
		{"Ñandú", "ÑAN", true},
		{"ſtore", "st", true},
		{"straße", "STRASSE", false},
		{"web", "wex", false},
	} {
		if got := HasPrefixFold(tc.s, tc.prefix); got != tc.want {
			t.Errorf("HasPrefixFold(%q, %q) = %v, want %v", tc.s, tc.prefix, got, tc.want)
		}
	}
}

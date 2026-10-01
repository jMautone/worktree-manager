package shell

import (
	"slices"
	"testing"
)

func TestActive(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", map[string]string{}, false},
		{"empty", map[string]string{DirectiveVar: ""}, false},
		{"set", map[string]string{DirectiveVar: "/tmp/wt.abc"}, true},
		{"only the previous directory", map[string]string{PreviousVar: "/a"}, false},
	} {
		getenv := func(k string) string { return tc.env[k] }
		if got := Active(getenv); got != tc.want {
			t.Errorf("%s: Active = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChildEnviron(t *testing.T) {
	for _, tc := range []struct {
		name    string
		goos    string
		environ []string
		want    []string
	}{
		{
			"removes both variables and keeps the rest in order",
			"darwin",
			[]string{"PATH=/bin", DirectiveVar + "=/tmp/wt.x", "HOME=/h", PreviousVar + "=/a", "WT_DEFAULT_BASE=dev"},
			[]string{"PATH=/bin", "HOME=/h", "WT_DEFAULT_BASE=dev"},
		},
		{
			"empty values are removed too",
			"linux",
			[]string{DirectiveVar + "=", PreviousVar + "=", "A=1"},
			[]string{"A=1"},
		},
		{
			"a value that mentions a variable is not a match",
			"linux",
			[]string{"X=" + DirectiveVar, "WT_DIRECTIVE_CD_FILE_X=1"},
			[]string{"X=" + DirectiveVar, "WT_DIRECTIVE_CD_FILE_X=1"},
		},
		{
			"case matters on unix",
			"darwin",
			[]string{"wt_directive_cd_file=/tmp/x", "Wt_Previous_Dir=/a"},
			[]string{"wt_directive_cd_file=/tmp/x", "Wt_Previous_Dir=/a"},
		},
		{
			"case is ignored on windows",
			"windows",
			[]string{"wt_directive_cd_file=C:\\t\\x", "Wt_Previous_Dir=C:\\a", "Path=C:\\bin"},
			[]string{"Path=C:\\bin"},
		},
		{
			"nothing to remove",
			"windows",
			[]string{},
			[]string{},
		},
	} {
		got := ChildEnviron(tc.environ, tc.goos)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: ChildEnviron = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestChildEnvironDoesNotModifyItsInput(t *testing.T) {
	in := []string{DirectiveVar + "=/x", "A=1"}
	ChildEnviron(in, "linux")
	if in[0] != DirectiveVar+"=/x" || in[1] != "A=1" {
		t.Errorf("input modified: %q", in)
	}
}

func TestNames(t *testing.T) {
	if want := []string{"zsh", "bash", "fish", "pwsh"}; !slices.Equal(Names, want) {
		t.Errorf("Names = %q, want %q", Names, want)
	}
}

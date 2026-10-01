package main

import (
	"strings"
	"testing"
)

func TestParseBranch(t *testing.T) {
	cases := map[string]Branch{
		"v0.1/walking-skeleton":                        {Kind: ChangeBranch, Minor: Minor{0, 1}, Change: "walking-skeleton"},
		"v1.0/doctor":                                  {Kind: ChangeBranch, Minor: Minor{1, 0}, Change: "doctor"},
		"fix/list-sort-order":                          {Kind: FixBranch},
		"release/v0.1.0":                               {Kind: ReleaseBranch, Release: Version{Minor: 1}},
		"chore/dependabot-config":                      {Kind: PlainBranch, Prefix: "chore"},
		"ci/release-conventions":                       {Kind: PlainBranch, Prefix: "ci"},
		"docs/contributing":                            {Kind: PlainBranch, Prefix: "docs"},
		"refactor/table-writer":                        {Kind: PlainBranch, Prefix: "refactor"},
		"test/e2e-windows":                             {Kind: PlainBranch, Prefix: "test"},
		"dependabot/github_actions/actions/checkout-7": {Kind: DependabotBranch},
	}
	for in, want := range cases {
		got, err := ParseBranch(in)
		if err != nil || got != want {
			t.Errorf("ParseBranch(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
}

func TestParseBranchErrors(t *testing.T) {
	for _, in := range []string{
		"main",
		"v1/walking-skeleton",    // no minor
		"v0.1.x",                 // maintenance line, never a PR head to main
		"v0.1/Walking_Skeleton",  // not kebab-case
		"v0.1/walking-skeleton/", // trailing slash
		"feat/shell-integration", // features go through vX.Y/<change>
		"fix/",                   // empty slug
		"release/v0.1.0-alpha.1", // a release branch publishes a final
		"release/v0.1.1",         // patches come from vX.Y.x
		"release/0.1.0",
		"powershell/v0.9.x",
	} {
		if _, err := ParseBranch(in); err == nil {
			t.Errorf("ParseBranch(%q) succeeded, want error", in)
		}
	}
	_, err := ParseBranch("feature/x")
	if err == nil || !strings.Contains(err.Error(), "CONTRIBUTING.md") {
		t.Errorf("error should point to CONTRIBUTING.md: %v", err)
	}
}

func TestArchivedAs(t *testing.T) {
	cases := []struct {
		dir, change string
		want        bool
	}{
		{"2026-09-30-walking-skeleton", "walking-skeleton", true},
		{"2026-09-30-shell-integration", "integration", false},
		{"2026-09-30-shell-integration", "shell", false},
		{"walking-skeleton", "walking-skeleton", false},
		{"2026-9-30-walking-skeleton", "walking-skeleton", false},
	}
	for _, c := range cases {
		if got := ArchivedAs(c.dir, c.change); got != c.want {
			t.Errorf("ArchivedAs(%q, %q) = %v, want %v", c.dir, c.change, got, c.want)
		}
	}
}

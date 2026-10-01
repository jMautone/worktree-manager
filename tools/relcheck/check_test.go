package main

import (
	"strings"
	"testing"
)

func checkPR(t *testing.T, branch, title string, f Facts) (*Version, []error) {
	t.Helper()
	b, err := ParseBranch(branch)
	if err != nil {
		t.Fatal(err)
	}
	ti, err := ParseTitle(title)
	if err != nil {
		t.Fatal(err)
	}
	return CheckPR(b, ti, f)
}

func TestCheckPRValid(t *testing.T) {
	alpha1 := mustVersions(t, "v0.1.0-alpha.1")
	cases := []struct {
		branch, title string
		facts         Facts
		publishes     string // empty means nothing
	}{
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1]",
			Facts{ChangeExists: true}, "v0.1.0-alpha.1"},
		{"fix/list-sort-order", "fix(list): sort names case-insensitively [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "v0.1.0-alpha.2"},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{Published: alpha1, ChangelogSection: true}, "v0.1.0"},
		{"ci/release-conventions", "ci(release): add branch, title and release conventions",
			Facts{}, ""},
		{"dependabot/go_modules/golang.org/x/sys-0.49.0", "chore(deps): bump golang.org/x/sys from 0.48.0 to 0.49.0",
			Facts{Published: alpha1}, ""},
	}
	for _, c := range cases {
		v, errs := checkPR(t, c.branch, c.title, c.facts)
		if len(errs) > 0 {
			t.Errorf("%s / %s: unexpected errors %v", c.branch, c.title, errs)
			continue
		}
		got := ""
		if v != nil {
			got = v.String()
		}
		if got != c.publishes {
			t.Errorf("%s: publishes %q, want %q", c.branch, got, c.publishes)
		}
	}
}

func TestCheckPRInvalid(t *testing.T) {
	alpha1 := mustVersions(t, "v0.1.0-alpha.1")
	cases := []struct {
		branch, title string
		facts         Facts
		want          string
	}{
		{"v0.1/walking-skeleton", "feat(list): add wt list [v0.1.0-alpha.1]",
			Facts{ChangeExists: true}, `must be the change name "walking-skeleton"`},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1]",
			Facts{}, "does not exist, archived or not"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list",
			Facts{ChangeExists: true}, `end the title with " [v0.1.0-alpha.1]"`},
		{"v0.1/shell-integration", "feat(shell-integration): add wt cd [v0.1.0-alpha.2]",
			Facts{ChangeExists: true}, "next alpha of 0.1 is v0.1.0-alpha.1"},
		{"v0.2/workspace-discovery", "feat(workspace-discovery): find repos [v0.2.0-alpha.1]",
			Facts{ChangeExists: true, Published: alpha1}, "open minor is 0.1"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.2.0-alpha.1]",
			Facts{ChangeExists: true}, "publishes an alpha of 0.1"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0]",
			Facts{ChangeExists: true, Published: alpha1}, "publishes an alpha of 0.1"},
		// Two PRs took the same alpha; the second must update its title.
		{"v0.1/shell-integration", "feat(shell-integration): add wt cd [v0.1.0-alpha.1]",
			Facts{ChangeExists: true, Published: alpha1}, "already published"},
		{"fix/list-sort-order", "feat(list): sort names [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "needs type fix"},
		{"fix/list-sort-order", "fix(list): sort names [v0.1.0]",
			Facts{Published: alpha1}, "publishes an alpha, not v0.1.0"},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{Published: alpha1}, `no "## [0.1.0]" section`},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{ChangelogSection: true}, "at least one alpha"},
		{"release/v0.1.0", "feat(release): close M1 [v0.1.0]",
			Facts{Published: alpha1, ChangelogSection: true}, "needs chore(release)"},
		{"ci/release-conventions", "ci(release): add conventions [v0.1.0-alpha.1]",
			Facts{}, "does not publish; remove the version suffix"},
		{"ci/release-conventions", "docs(release): add conventions",
			Facts{}, "needs type ci"},
		{"dependabot/github_actions/actions/checkout-7", "chore(ci): bump actions/checkout [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "does not publish"},
	}
	for _, c := range cases {
		v, errs := checkPR(t, c.branch, c.title, c.facts)
		if v != nil {
			t.Errorf("%s / %s: publishes %s despite errors", c.branch, c.title, v)
		}
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		if all := strings.Join(msgs, "\n"); !strings.Contains(all, c.want) {
			t.Errorf("%s / %s: errors %q, want one containing %q", c.branch, c.title, all, c.want)
		}
	}
}

func TestCheckPRReportsEveryViolation(t *testing.T) {
	_, errs := checkPR(t, "v0.1/walking-skeleton", "docs(list): add wt list", Facts{})
	if len(errs) != 3 { // wrong scope, missing change, missing version
		t.Errorf("got %d errors, want 3: %v", len(errs), errs)
	}
}

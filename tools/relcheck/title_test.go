package main

import (
	"strings"
	"testing"
)

func TestParseTitle(t *testing.T) {
	got, err := ParseTitle("feat(walking-skeleton)!: add wt list, wt config and CLI contract [v0.1.0-alpha.1]")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "feat" || got.Scope != "walking-skeleton" || !got.Breaking ||
		got.Summary != "add wt list, wt config and CLI contract" ||
		got.Version == nil || got.Version.String() != "v0.1.0-alpha.1" {
		t.Errorf("ParseTitle = %+v", got)
	}

	got, err = ParseTitle("chore(ci): bump actions/checkout from 4 to 7")
	if err != nil || got.Version != nil || got.Breaking {
		t.Errorf("ParseTitle without version = %+v, %v", got, err)
	}

	// The 72-character limit does not count the version suffix.
	long := "docs(x): " + strings.Repeat("a", 63) // 72 characters
	if _, err := ParseTitle(long + " [v0.1.0-alpha.1]"); err != nil {
		t.Errorf("72 characters plus suffix: %v", err)
	}
}

func TestParseTitleErrors(t *testing.T) {
	cases := map[string]string{
		"add wt list":                         "does not match",
		"feat: add wt list":                   "does not match",
		"feat(): add wt list":                 "kebab-case",
		"feat(Walking): add wt list":          "kebab-case",
		"feature(list): add wt list":          "is not one of",
		"feat(list): Add wt list":             "lowercase",
		"feat(list): add wt list.":            "period",
		"feat(list):  add wt list":            "leading or trailing spaces",
		"feat(list): add wt list [0.1.0]":     "version suffix [0.1.0]",
		"feat(list): add wt list [v0.1]":      "version suffix [v0.1]",
		"feat(list): add wt list [wip]":       "version suffix [wip]",
		"docs(x): " + strings.Repeat("a", 64): "the limit is 72",
	}
	for in, want := range cases {
		_, err := ParseTitle(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseTitle(%q) = %v, want error containing %q", in, err, want)
		}
	}
}

func TestPublishedVersions(t *testing.T) {
	subjects := []string{
		"feat(shell-integration): add wt shell init and wt cd [v0.1.0-alpha.2] (#6)",
		"chore(ci): bump actions/checkout from 4 to 7 (#5)",
		"feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1] (#3)",
		"v1: reescritura de cero en Go, multiplataforma (M0 bootstrap) (#1)",
		"feat(console): improve Clear-WtConsoleScreen function",
		"",
	}
	got, err := PublishedVersions(subjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != "v0.1.0-alpha.2" || got[1].String() != "v0.1.0-alpha.1" {
		t.Errorf("PublishedVersions = %v", got)
	}

	if _, err := PublishedVersions([]string{"feat(x): y [v0.1] (#9)"}); err == nil {
		t.Error("a malformed suffix on main must be an error, not be skipped")
	}
}

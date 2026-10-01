package main

import (
	"strings"
	"testing"
)

func mustVersions(t *testing.T, ss ...string) []Version {
	t.Helper()
	var vs []Version
	for _, s := range ss {
		v, err := ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	return vs
}

func TestParseVersion(t *testing.T) {
	good := map[string]Version{
		"v0.1.0":          {0, 1, 0, 0},
		"v0.1.0-alpha.1":  {0, 1, 0, 1},
		"v1.0.0":          {1, 0, 0, 0},
		"v10.20.3":        {10, 20, 3, 0},
		"v0.2.0-alpha.12": {0, 2, 0, 12},
	}
	for in, want := range good {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
	}
	for _, in := range []string{"", "0.1.0", "v0.1", "v01.1.0", "v0.1.0-alpha.0", "v0.1.0-beta.1", "v0.1.0-alpha", "v0.1.0 "} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) succeeded, want error", in)
		}
	}
}

func TestLess(t *testing.T) {
	ordered := mustVersions(t, "v0.1.0-alpha.1", "v0.1.0-alpha.2", "v0.1.0-alpha.10", "v0.1.0", "v0.2.0-alpha.1", "v0.4.0", "v1.0.0-alpha.1", "v1.0.0")
	for i := 0; i < len(ordered)-1; i++ {
		a, b := ordered[i], ordered[i+1]
		if !a.Less(b) || b.Less(a) {
			t.Errorf("want %s < %s", a, b)
		}
	}
}

func TestOpenMinor(t *testing.T) {
	cases := []struct {
		published []string
		want      Minor
	}{
		{nil, Minor{0, 1}},
		{[]string{"v0.1.0-alpha.1"}, Minor{0, 1}},
		{[]string{"v0.1.0-alpha.1", "v0.1.0"}, Minor{0, 2}},
		{[]string{"v0.1.0", "v0.1.0-alpha.1"}, Minor{0, 2}}, // order of input does not matter
		{[]string{"v0.4.0-alpha.3", "v0.4.0"}, Minor{1, 0}}, // M5 ships 1.0
		{[]string{"v1.0.0"}, Minor{1, 1}},
	}
	for _, c := range cases {
		got, err := OpenMinor(mustVersions(t, c.published...))
		if err != nil || got != c.want {
			t.Errorf("OpenMinor(%v) = %v, %v; want %v", c.published, got, err, c.want)
		}
	}
}

func TestOpenMinorErrors(t *testing.T) {
	for _, published := range [][]string{{"v1.1.0"}, {"v0.5.0"}} {
		if _, err := OpenMinor(mustVersions(t, published...)); err == nil {
			t.Errorf("OpenMinor(%v) succeeded, want error", published)
		}
	}
}

func TestNextAlpha(t *testing.T) {
	cases := map[string][]string{
		"v0.1.0-alpha.1": nil,
		"v0.1.0-alpha.3": {"v0.1.0-alpha.1", "v0.1.0-alpha.2"},
		"v0.2.0-alpha.1": {"v0.1.0-alpha.1", "v0.1.0"},
	}
	for want, published := range cases {
		got, err := NextAlpha(mustVersions(t, published...))
		if err != nil || got.String() != want {
			t.Errorf("NextAlpha(%v) = %v, %v; want %s", published, got, err, want)
		}
	}
}

func TestNextCheck(t *testing.T) {
	cases := []struct {
		v         string
		published []string
		wantErr   string // empty means valid
	}{
		{"v0.1.0-alpha.1", nil, ""},
		{"v0.1.0-alpha.2", []string{"v0.1.0-alpha.1"}, ""},
		{"v0.1.0", []string{"v0.1.0-alpha.1"}, ""},
		{"v0.2.0-alpha.1", []string{"v0.1.0-alpha.1", "v0.1.0"}, ""},
		{"v1.0.0-alpha.1", []string{"v0.4.0-alpha.1", "v0.4.0"}, ""},
		// Two PRs raced for the same alpha: the second one must fail.
		{"v0.1.0-alpha.1", []string{"v0.1.0-alpha.1"}, "already published; the next alpha is v0.1.0-alpha.2"},
		{"v0.1.0", []string{"v0.1.0-alpha.1", "v0.1.0"}, "already published; the open minor is now 0.2"},
		{"v0.1.0-alpha.3", []string{"v0.1.0-alpha.1"}, "next alpha of 0.1 is v0.1.0-alpha.2"},
		{"v0.2.0-alpha.1", []string{"v0.1.0-alpha.1"}, "open minor is 0.1"},
		{"v0.1.0-alpha.2", []string{"v0.1.0-alpha.1", "v0.1.0"}, "open minor is 0.2"},
		{"v0.1.0", nil, "at least one alpha"},
		{"v0.1.1", []string{"v0.1.0-alpha.1", "v0.1.0"}, "vX.Y.x branch"},
		{"v0.5.0-alpha.1", []string{"v0.4.0-alpha.1", "v0.4.0"}, "open minor is 1.0"},
	}
	for _, c := range cases {
		err := NextCheck(mustVersions(t, c.v)[0], mustVersions(t, c.published...))
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("NextCheck(%s, %v) = %v, want nil", c.v, c.published, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("NextCheck(%s, %v) = %v, want error containing %q", c.v, c.published, err, c.wantErr)
		}
	}
}

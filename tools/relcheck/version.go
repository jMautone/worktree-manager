// Command relcheck enforces the branch, PR title and version conventions of
// docs/decisions/0002-versionado-y-releases.md. The pr-conventions workflow
// runs it on every pull request to main, and release.yml runs it on every
// push to main.
//
// The decisions live in pure functions (version.go, title.go, branch.go,
// check.go, changelog.go). main.go only reads git and the filesystem.
package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version is a release version: vMAJOR.MINOR.PATCH, optionally -alpha.N.
type Version struct {
	Major, Minor, Patch int
	Alpha               int // 0 for a final release
}

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-alpha\.([1-9][0-9]*))?$`)

// ParseVersion parses "v0.1.0" or "v0.1.0-alpha.2".
func ParseVersion(s string) (Version, error) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a version: want vX.Y.Z or vX.Y.Z-alpha.N", s)
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	if m[4] != "" {
		v.Alpha, _ = strconv.Atoi(m[4])
	}
	return v, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Alpha > 0 {
		s += fmt.Sprintf("-alpha.%d", v.Alpha)
	}
	return s
}

// IsAlpha reports whether v is a pre-release.
func (v Version) IsAlpha() bool { return v.Alpha > 0 }

// MinorOf returns the minor line v belongs to.
func (v Version) MinorOf() Minor { return Minor{v.Major, v.Minor} }

// Less orders versions by SemVer: v0.1.0-alpha.2 < v0.1.0 < v0.2.0-alpha.1.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	if v.Patch != w.Patch {
		return v.Patch < w.Patch
	}
	if v.IsAlpha() != w.IsAlpha() {
		return v.IsAlpha() // a pre-release sorts before its final
	}
	return v.Alpha < w.Alpha
}

// Minor is a MAJOR.MINOR line. Each milestone ships as one minor.
type Minor struct{ Major, Minor int }

func (m Minor) String() string { return fmt.Sprintf("%d.%d", m.Major, m.Minor) }

// Milestones maps M1..M6 to their minor, in order. M5 ships 1.0.0.
// Keep it in sync with the table in docs/decisions/0002-versionado-y-releases.md.
var Milestones = []Minor{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 0}, {1, 1}}

// OpenMinor returns the minor that merges to main publish into: the minor of
// the latest published alpha, or the milestone after the latest final.
func OpenMinor(published []Version) (Minor, error) {
	if len(published) == 0 {
		return Milestones[0], nil
	}
	latest := published[0]
	for _, v := range published[1:] {
		if latest.Less(v) {
			latest = v
		}
	}
	if latest.IsAlpha() {
		return latest.MinorOf(), nil
	}
	for i, m := range Milestones {
		if m != latest.MinorOf() {
			continue
		}
		if i+1 == len(Milestones) {
			return Minor{}, fmt.Errorf("%s closed the last milestone in the table; extend Milestones and ADR 0002", latest)
		}
		return Milestones[i+1], nil
	}
	return Minor{}, fmt.Errorf("latest version %s is not a milestone minor (ADR 0002)", latest)
}

// NextAlpha returns the alpha the next change or fix publishes.
func NextAlpha(published []Version) (Version, error) {
	open, err := OpenMinor(published)
	if err != nil {
		return Version{}, err
	}
	return Version{Major: open.Major, Minor: open.Minor, Alpha: lastAlpha(published, open) + 1}, nil
}

func lastAlpha(published []Version, m Minor) int {
	last := 0
	for _, p := range published {
		if p.MinorOf() == m && p.Alpha > last {
			last = p.Alpha
		}
	}
	return last
}

// NextCheck reports whether v may be published next, given every version
// main already published.
func NextCheck(v Version, published []Version) error {
	if v.Patch != 0 {
		return fmt.Errorf("%s: patch releases come from a vX.Y.x branch, not from main", v)
	}
	open, err := OpenMinor(published)
	if err != nil {
		return err
	}
	for _, p := range published {
		if p != v {
			continue
		}
		// Another PR published it first: say what to use instead.
		if v.IsAlpha() {
			next := Version{Major: open.Major, Minor: open.Minor, Alpha: lastAlpha(published, open) + 1}
			return fmt.Errorf("%s is already published; the next alpha is %s", v, next)
		}
		return fmt.Errorf("%s is already published; the open minor is now %s", v, open)
	}
	if v.MinorOf() != open {
		return fmt.Errorf("%s targets %s, but the open minor is %s", v, v.MinorOf(), open)
	}
	last := lastAlpha(published, open)
	if v.IsAlpha() {
		if v.Alpha != last+1 {
			next := Version{Major: open.Major, Minor: open.Minor, Alpha: last + 1}
			return fmt.Errorf("%s: the next alpha of %s is %s", v, open, next)
		}
		return nil
	}
	if last == 0 {
		return fmt.Errorf("%s: a final needs at least one alpha of %s published first", v, open)
	}
	return nil
}

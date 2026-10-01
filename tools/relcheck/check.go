package main

import (
	"fmt"
	"strings"
)

// Facts are what CheckPR needs from the repository besides branch and title.
type Facts struct {
	Published        []Version // versions already published on main
	ChangeExists     bool      // ChangeBranch: openspec/changes/<change>/ exists, archived or not
	ChangelogSection bool      // ReleaseBranch: CHANGELOG.md has a "## [X.Y.0]" section
}

// CheckPR validates a pull request against ADR 0002. It returns the version
// the merge publishes (nil if none), or every violation found.
func CheckPR(b Branch, t Title, f Facts) (*Version, []error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	switch b.Kind {
	case ChangeBranch:
		if t.Scope != b.Change {
			fail("scope %q must be the change name %q", t.Scope, b.Change)
		}
		if !f.ChangeExists {
			fail("openspec/changes/%s/ does not exist, archived or not", b.Change)
		}
	case FixBranch:
		if t.Type != "fix" {
			fail("a fix/ branch needs type fix, not %q", t.Type)
		}
	case ReleaseBranch:
		if t.Type != "chore" || t.Scope != "release" {
			fail("a release/ branch needs chore(release), not %s(%s)", t.Type, t.Scope)
		}
		if !f.ChangelogSection {
			fail("CHANGELOG.md has no \"## [%s]\" section", strings.TrimPrefix(b.Release.String(), "v"))
		}
	case PlainBranch:
		if t.Type != b.Prefix {
			fail("a %s/ branch needs type %s, not %q", b.Prefix, b.Prefix, t.Type)
		}
	}

	switch {
	case !b.Publishes() && t.Version != nil:
		fail("this branch does not publish; remove the version suffix [%s]", t.Version)
	case b.Publishes() && t.Version == nil:
		fail("this branch publishes; end the title with \" [%s]\"", suggestion(b, f.Published))
	case b.Publishes():
		v := *t.Version
		switch b.Kind {
		case ChangeBranch:
			if !v.IsAlpha() || v.MinorOf() != b.Minor {
				fail("a v%s/ branch publishes an alpha of %s, not %s", b.Minor, b.Minor, v)
			}
		case FixBranch:
			if !v.IsAlpha() {
				fail("a fix/ branch publishes an alpha, not %s", v)
			}
		case ReleaseBranch:
			if v != b.Release {
				fail("release/%s publishes %s, not %s", b.Release, b.Release, v)
			}
		}
		if err := NextCheck(v, f.Published); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return t.Version, nil
}

// suggestion is the version a publishing branch should put in its title.
func suggestion(b Branch, published []Version) string {
	if b.Kind == ReleaseBranch {
		return b.Release.String()
	}
	if v, err := NextAlpha(published); err == nil {
		return v.String()
	}
	return "vX.Y.0-alpha.N"
}

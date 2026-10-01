package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// titleTypes are the Conventional Commits types a title may use.
var titleTypes = map[string]bool{
	"feat": true, "fix": true, "docs": true, "chore": true,
	"ci": true, "refactor": true, "test": true, "perf": true,
}

// maxTitleLen caps the title without its version suffix.
const maxTitleLen = 72

// Title is a PR title: <type>(<scope>)[!]: <summary> [<version>].
type Title struct {
	Type, Scope, Summary string
	Breaking             bool
	Version              *Version // nil when the title has no version suffix
}

var (
	titleRE    = regexp.MustCompile(`^([a-z]+)\(([^()]*)\)(!?): (.*)$`)
	suffixRE   = regexp.MustCompile(` \[([^\[\]]*)\]$`)
	prNumberRE = regexp.MustCompile(` \(#[0-9]+\)$`)
	kebabRE    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// ParseTitle parses a PR title and checks its format. A trailing " [...]" is
// always read as the version suffix, so a malformed version is an error and
// never silently becomes part of the summary.
func ParseTitle(s string) (Title, error) {
	var t Title
	head := s
	if m := suffixRE.FindStringSubmatchIndex(s); m != nil {
		raw := s[m[2]:m[3]]
		v, err := ParseVersion(raw)
		if err != nil {
			return Title{}, fmt.Errorf("version suffix [%s]: %v", raw, err)
		}
		t.Version = &v
		head = s[:m[0]]
	}
	if n := utf8.RuneCountInString(head); n > maxTitleLen {
		return Title{}, fmt.Errorf("title is %d characters without the version suffix; the limit is %d", n, maxTitleLen)
	}
	m := titleRE.FindStringSubmatch(head)
	if m == nil {
		return Title{}, fmt.Errorf("title %q does not match <type>(<scope>)[!]: <summary> [<version>]", s)
	}
	t.Type, t.Scope, t.Breaking, t.Summary = m[1], m[2], m[3] == "!", m[4]
	if !titleTypes[t.Type] {
		return Title{}, fmt.Errorf("type %q is not one of feat, fix, docs, chore, ci, refactor, test, perf", t.Type)
	}
	if !kebabRE.MatchString(t.Scope) {
		return Title{}, fmt.Errorf("scope %q must be non-empty kebab-case", t.Scope)
	}
	switch first, _ := utf8.DecodeRuneInString(t.Summary); {
	case t.Summary == "":
		return Title{}, fmt.Errorf("summary is empty")
	case strings.TrimSpace(t.Summary) != t.Summary:
		return Title{}, fmt.Errorf("summary %q has leading or trailing spaces", t.Summary)
	case unicode.IsUpper(first):
		return Title{}, fmt.Errorf("summary %q must start in lowercase", t.Summary)
	case strings.HasSuffix(t.Summary, "."):
		return Title{}, fmt.Errorf("summary %q must not end with a period", t.Summary)
	}
	return t, nil
}

// SubjectVersion returns the version a squash subject on main published,
// "<title> [vX.Y.Z] (#N)". ok is false when the subject has no suffix.
func SubjectVersion(subject string) (v Version, ok bool, err error) {
	m := suffixRE.FindStringSubmatch(prNumberRE.ReplaceAllString(subject, ""))
	if m == nil {
		return Version{}, false, nil
	}
	v, err = ParseVersion(m[1])
	if err != nil {
		return Version{}, false, fmt.Errorf("subject %q: %v", subject, err)
	}
	return v, true, nil
}

// PublishedVersions collects the versions the given subjects of main
// published. Subjects without a version suffix are skipped.
func PublishedVersions(subjects []string) ([]Version, error) {
	var vs []Version
	for _, s := range subjects {
		v, ok, err := SubjectVersion(s)
		if err != nil {
			return nil, err
		}
		if ok {
			vs = append(vs, v)
		}
	}
	return vs, nil
}

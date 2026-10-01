package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// BranchKind is what a branch is for, which decides whether it publishes.
type BranchKind int

const (
	ChangeBranch     BranchKind = iota // vX.Y/<change>: publishes an alpha
	FixBranch                          // fix/<slug>: publishes an alpha
	ReleaseBranch                      // release/vX.Y.0: publishes a final
	PlainBranch                        // chore|docs|ci|refactor|test/<slug>: publishes nothing
	DependabotBranch                   // dependabot/**: publishes nothing
)

// Branch is a parsed head branch name.
type Branch struct {
	Kind    BranchKind
	Prefix  string  // PlainBranch: chore, docs, ci, refactor or test
	Minor   Minor   // ChangeBranch: the minor of the change's milestone
	Change  string  // ChangeBranch: the OpenSpec change name
	Release Version // ReleaseBranch: the final it publishes
}

// Publishes reports whether merging the branch publishes a version.
func (b Branch) Publishes() bool {
	return b.Kind == ChangeBranch || b.Kind == FixBranch || b.Kind == ReleaseBranch
}

var (
	changeBranchRE  = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)/([a-z0-9]+(?:-[a-z0-9]+)*)$`)
	slugBranchRE    = regexp.MustCompile(`^(fix|chore|docs|ci|refactor|test)/([a-z0-9]+(?:-[a-z0-9]+)*)$`)
	releaseBranchRE = regexp.MustCompile(`^release/(.*)$`)
	archivedRE      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-(.+)$`)
)

// ParseBranch classifies a head branch name.
func ParseBranch(s string) (Branch, error) {
	if m := changeBranchRE.FindStringSubmatch(s); m != nil {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		return Branch{Kind: ChangeBranch, Minor: Minor{major, minor}, Change: m[3]}, nil
	}
	if m := slugBranchRE.FindStringSubmatch(s); m != nil {
		if m[1] == "fix" {
			return Branch{Kind: FixBranch}, nil
		}
		return Branch{Kind: PlainBranch, Prefix: m[1]}, nil
	}
	if m := releaseBranchRE.FindStringSubmatch(s); m != nil {
		v, err := ParseVersion(m[1])
		if err != nil || v.IsAlpha() || v.Patch != 0 {
			return Branch{}, fmt.Errorf("branch %q: a release branch is release/vX.Y.0", s)
		}
		return Branch{Kind: ReleaseBranch, Release: v}, nil
	}
	if strings.HasPrefix(s, "dependabot/") {
		return Branch{Kind: DependabotBranch}, nil
	}
	return Branch{}, fmt.Errorf("branch %q matches no allowed pattern: vX.Y/<change>, fix/<slug>, release/vX.Y.0, chore|docs|ci|refactor|test/<slug> (see CONTRIBUTING.md)", s)
}

// ArchivedAs reports whether dir, a directory name under
// openspec/changes/archive, is the archive of change. OpenSpec names archives
// YYYY-MM-DD-<change>, so "2026-09-30-shell-integration" archives
// "shell-integration" and not "integration".
func ArchivedAs(dir, change string) bool {
	m := archivedRE.FindStringSubmatch(dir)
	return m != nil && m[1] == change
}

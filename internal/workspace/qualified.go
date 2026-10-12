package workspace

import "strings"

// SplitQualified splits a target at its first ":" into its repository side
// and its name side. qualified is false when target has no ":"; then repo
// and name are empty. The name side may itself contain ":".
func SplitQualified(target string) (repo, name string, qualified bool) {
	repo, name, qualified = strings.Cut(target, ":")
	if !qualified {
		return "", "", false
	}
	return repo, name, true
}

// MatchSide resolves the repository side of a qualified target among repos,
// as List returns them.
//
//   - A side without a path separator (/ on every OS; / or \ on windows) is
//     a name, resolved as Match does.
//   - A side with one is a path suffix: it matches the repositories whose
//     path ends with its components, each compared whole and ignoring case
//     on every OS. Empty components (separators at either end, or repeated)
//     are ignored. There is no prefix match.
//
// The errors are those of Match, with the side as the target. goos is a
// parameter, not filepath's, so that the rules of windows are testable
// anywhere.
func MatchSide(repos []Repo, side, goos string) (Repo, error) {
	seps := separators(goos)
	if !strings.ContainsAny(side, seps) {
		return Match(repos, side)
	}
	want := splitPath(side, seps)
	if len(want) == 0 {
		return Repo{}, &NotFoundError{Target: side}
	}
	var found []Repo
	for _, r := range repos {
		if hasSuffixFold(splitPath(r.Path, seps), want) {
			found = append(found, r)
		}
	}
	switch len(found) {
	case 0:
		return Repo{}, &NotFoundError{Target: side}
	case 1:
		return found[0], nil
	}
	return Repo{}, &AmbiguousError{Target: side, Candidates: found}
}

// DisplayNames returns the name wt shows for each repository of repos, as
// List returns them, in the same order: its name when no other repository
// has exactly the same name; otherwise the shortest suffix of its path, of
// at least two components joined with /, that MatchSide resolves to that
// repository alone; and its path when no suffix does. Deciding with
// MatchSide is what makes a display name, typed back as a repository side,
// lead to the same repository.
func DisplayNames(repos []Repo, goos string) []string {
	count := map[string]int{}
	for _, r := range repos {
		count[r.Name]++
	}
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Path
		if count[r.Name] == 1 {
			names[i] = r.Name
			continue
		}
		parts := splitPath(r.Path, separators(goos))
		for k := 2; k <= len(parts); k++ {
			suffix := strings.Join(parts[len(parts)-k:], "/")
			if m, err := MatchSide(repos, suffix, goos); err == nil && m.Path == r.Path {
				names[i] = suffix
				break
			}
		}
	}
	return names
}

// separators are the path separators of a repository side on goos.
func separators(goos string) string {
	if goos == "windows" {
		return `/\`
	}
	return "/"
}

// splitPath splits p at any of seps into its non-empty components.
func splitPath(p, seps string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return strings.ContainsRune(seps, r) })
}

// hasSuffixFold reports whether the components of p end with those of
// suffix, compared with strings.EqualFold.
func hasSuffixFold(p, suffix []string) bool {
	if len(suffix) > len(p) {
		return false
	}
	p = p[len(p)-len(suffix):]
	for i := range suffix {
		if !strings.EqualFold(p[i], suffix[i]) {
			return false
		}
	}
	return true
}

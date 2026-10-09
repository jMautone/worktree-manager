package workspace

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FileID identifies a directory on disk, whatever path, link, junction or
// letter case led to it: device and inode on macOS and Linux, volume serial
// number and file index on Windows. The zero FileID identifies nothing.
type FileID struct {
	Dev, Ino uint64
}

// Found is a repository as the walk found it, before deduplication.
type Found struct {
	Path     string // native path under the root, links not resolved
	Root     string // the Path of the root it was found under
	ID       FileID // the repository's directory
	GitdirID FileID // its repository (git) directory; zero if unknown
}

// Repo is a discovered repository, as wt repos lists it.
type Repo struct {
	Name     string // last component of Path
	Path     string
	Root     string
	ID       FileID
	GitdirID FileID
}

// List turns what the walk found, in the walk's order, into the
// repositories wt lists: each directory once, ordered by name.
//
//   - A directory found more than once (overlapping roots, a root given
//     twice, a link to a repository already found) keeps the first path the
//     walk found: the walk goes root by root in the order of repos_root and,
//     within a root, depth first by name.
//   - Order: by name ignoring case, ties broken by path, as wt list does.
func List(found []Found, goos string) []Repo {
	seen := map[FileID]bool{}
	repos := []Repo{}
	for _, f := range found {
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		repos = append(repos, Repo{Name: base(goos, f.Path), Path: f.Path, Root: f.Root, ID: f.ID, GitdirID: f.GitdirID})
	}
	sort.SliceStable(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Path < b.Path
	})
	return repos
}

// base returns the last component of a native path.
func base(goos, p string) string {
	seps := "/"
	if goos == "windows" {
		seps = `\/`
	}
	p = strings.TrimRight(p, seps)
	return p[strings.LastIndexAny(p, seps)+1:]
}

// Current returns the index of the repository that id belongs to, or -1.
// id is the directory of the main worktree git reports for the working
// directory: the repository's own directory for a clone or a
// --separate-git-dir checkout (Repo.ID), and the bare repository for the
// .bare layout (Repo.GitdirID).
func Current(repos []Repo, id FileID) int {
	if id == (FileID{}) {
		return -1
	}
	for i, r := range repos {
		if r.ID == id || r.GitdirID == id {
			return i
		}
	}
	return -1
}

// NotFoundError means no repository matched the target.
type NotFoundError struct {
	Target string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("no repository named %q", e.Target) }

// AmbiguousError means more than one repository matched the target in the
// step that matched. Candidates are in the order of List.
type AmbiguousError struct {
	Target     string
	Candidates []Repo
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches more than one repository", e.Target)
}

// Match resolves target among repos, as List returns them, in three steps,
// each only when the previous one matched none: the name equals target;
// equals it ignoring case; starts with it ignoring case. The rules are the
// same on every OS. An empty target matches nothing.
func Match(repos []Repo, target string) (Repo, error) {
	if target == "" {
		return Repo{}, &NotFoundError{Target: target}
	}
	for _, matches := range []func(string) bool{
		func(name string) bool { return name == target },
		func(name string) bool { return strings.EqualFold(name, target) },
		func(name string) bool { return HasPrefixFold(name, target) },
	} {
		var found []Repo
		for _, r := range repos {
			if matches(r.Name) {
				found = append(found, r)
			}
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], nil
		default:
			return Repo{}, &AmbiguousError{Target: target, Candidates: found}
		}
	}
	return Repo{}, &NotFoundError{Target: target}
}

// HasPrefixFold reports whether s starts with prefix, ignoring case as
// strings.EqualFold does: rune by rune, with simple Unicode case folding.
// Lowering both strings first would not do: ToLower can change how many
// bytes a string takes.
func HasPrefixFold(s, prefix string) bool {
	for _, p := range prefix {
		r, size := utf8.DecodeRuneInString(s)
		if size == 0 || !equalFoldRune(r, p) {
			return false
		}
		s = s[size:]
	}
	return true
}

func equalFoldRune(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

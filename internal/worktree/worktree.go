// Package worktree turns what git reports about a repository's worktrees into
// what wt presents: native paths, which worktree is main and which is
// current, and the order they are listed in.
//
// Decision/effect boundary: everything here is pure. It does not run git or
// read the filesystem; the OS is a parameter, so the rules of every OS are
// testable from any OS.
package worktree

import (
	"sort"
	"strings"

	"github.com/jMautone/worktree-manager/internal/git"
)

// Worktree is one worktree as wt presents it.
type Worktree struct {
	Name           string // last component of Path
	Path           string // native absolute path
	Branch         string // short name; empty when detached or bare
	Head           string // full commit id; empty for a bare repository
	Detached       bool
	Bare           bool
	Main           bool
	Current        bool
	Locked         bool
	LockedReason   string
	Prunable       bool
	PrunableReason string
}

// Build decides main, current, native paths and order.
//
// cwdReal is the working directory with symbolic links already resolved, and
// the entries' paths are expected resolved the same way (git reports real
// paths; resolving is the caller's job because it touches the filesystem).
//
//   - Main is the first entry git reports; for a bare repository, the bare
//     repository itself.
//   - Current is the worktree whose path contains cwdReal, compared by whole
//     path components (/a/repo does not contain /a/repo2). With nested
//     worktrees the deepest match wins. The comparison ignores case on darwin
//     and windows, whose default filesystems do.
//   - Order: main first, then by name ignoring case, ties broken by path.
func Build(entries []git.WorktreeEntry, cwdReal, goos string) []Worktree {
	fold := goos == "darwin" || goos == "windows"
	cwd := components(native(cwdReal, goos), goos)

	ws := make([]Worktree, 0, len(entries))
	current, depth := -1, -1
	for i, e := range entries {
		path := native(e.Path, goos)
		parts := components(path, goos)
		name := path
		if len(parts) > 0 {
			name = parts[len(parts)-1]
		}
		ws = append(ws, Worktree{
			Name:           name,
			Path:           path,
			Branch:         e.Branch,
			Head:           e.Head,
			Detached:       e.Detached,
			Bare:           e.Bare,
			Main:           i == 0,
			Locked:         e.Locked,
			LockedReason:   e.LockedReason,
			Prunable:       e.Prunable,
			PrunableReason: e.PrunableReason,
		})
		if contains(parts, cwd, fold) && len(parts) > depth {
			current, depth = i, len(parts)
		}
	}
	if current >= 0 {
		ws[current].Current = true
	}

	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i], ws[j]
		if a.Main != b.Main {
			return a.Main
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Path < b.Path
	})
	return ws
}

// native converts a path as git reports it (forward slashes everywhere) to
// goos's form.
func native(p, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// components splits a native path into its non-empty components.
func components(p, goos string) []string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	var parts []string
	for _, c := range strings.Split(p, sep) {
		if c != "" {
			parts = append(parts, c)
		}
	}
	return parts
}

// contains reports whether the path with components dir contains (or is) the
// path with components p.
func contains(dir, p []string, fold bool) bool {
	if len(dir) > len(p) {
		return false
	}
	for i := range dir {
		if fold && !strings.EqualFold(dir[i], p[i]) || !fold && dir[i] != p[i] {
			return false
		}
	}
	return true
}

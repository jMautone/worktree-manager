package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// Walk finds the repositories under roots, down to depth levels below each
// one. It is the only part of this package that reads the filesystem, and
// it runs no git.
//
//   - Roots are walked in order; within a directory, entries by name, depth
//     first. The roots themselves are not candidates.
//   - A candidate is a directory, or a symbolic link (on Windows, also a
//     junction, which Lstat reports as irregular) that leads to one.
//   - A candidate with a .git entry of any kind is not walked into; it is
//     found when Classify says it is a repository.
//   - A directory that cannot be read is skipped without a warning. A root
//     that does not exist or is not a directory is skipped with one.
//
// Cycles of links need no detection: depth bounds the walk, and List drops
// what was found twice.
func Walk(roots []Root, depth int) ([]Found, []string) {
	var found []Found
	var warnings []string
	for _, r := range roots {
		info, err := os.Stat(r.Path)
		switch {
		case err != nil:
			warnings = append(warnings, "repository root not found: "+r.Path)
			continue
		case !info.IsDir():
			warnings = append(warnings, "repository root is not a directory: "+r.Path)
			continue
		}
		found = walkDir(found, r.Path, r.Path, 1, depth)
	}
	return found, warnings
}

// walkDir appends to found the repositories among the entries of dir, which
// are at the given level below root.
func walkDir(found []Found, root, dir string, level, depth int) []Found {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return found
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !isDir(path, e) {
			continue
		}
		g := readGitEntry(path)
		switch Classify(g) {
		case KindNone:
			if level < depth {
				found = walkDir(found, root, path, level+1, depth)
			}
		case KindRepo:
			id, err := IDOf(path)
			if err != nil {
				continue
			}
			gitdirID, _ := IDOf(g.Gitdir)
			found = append(found, Found{Path: path, Root: root, ID: id, GitdirID: gitdirID})
		}
	}
	return found
}

// isDir reports whether the entry at path is a directory or leads to one.
func isDir(path string, e fs.DirEntry) bool {
	t := e.Type()
	if t.IsDir() {
		return true
	}
	if t&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readGitEntry gathers the facts about dir/.git that Classify decides on.
func readGitEntry(dir string) GitEntry {
	p := filepath.Join(dir, ".git")
	info, err := os.Lstat(p)
	if err != nil {
		return GitEntry{}
	}
	g := GitEntry{Exists: true}
	if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		if info, err = os.Stat(p); err != nil {
			return g
		}
	}
	if info.IsDir() {
		g.Dir, g.Gitdir, g.GitdirIsDir = true, p, true
		return g
	}
	content, err := os.ReadFile(p)
	if err != nil {
		return g
	}
	gitdir, ok := ParseGitFile(content, dir, runtime.GOOS)
	if !ok {
		return g
	}
	g.Gitdir = gitdir
	if info, err := os.Stat(gitdir); err == nil && info.IsDir() {
		g.GitdirIsDir = true
		_, err := os.Stat(filepath.Join(gitdir, "commondir"))
		g.Commondir = err == nil
	}
	return g
}

package workspace

import "strings"

// Kind is what a directory under a root is, judging by its .git entry.
type Kind int

const (
	// KindNone: no .git entry. Not a repository; the walk goes down into it.
	KindNone Kind = iota
	// KindRepo: a repository, with a .git directory or a .git file that
	// points to a repository directory of its own (.bare, --separate-git-dir).
	KindRepo
	// KindLinked: a linked worktree of another repository.
	KindLinked
	// KindBroken: a .git entry that leads nowhere, such as an orphaned
	// worktree whose repository was deleted.
	KindBroken
)

// GitEntry holds the facts the walk read about a directory's .git entry,
// with links followed, before anything is decided from them.
type GitEntry struct {
	Exists bool // the .git entry exists, of whatever type
	Dir    bool // .git is a directory
	// Gitdir is the repository directory: the .git directory itself, or
	// where a .git file points (see ParseGitFile). Empty when a .git file
	// could not be read or parsed.
	Gitdir string
	// GitdirIsDir reports that Gitdir exists and is a directory.
	GitdirIsDir bool
	// Commondir reports that Gitdir has a commondir file, the mark git
	// leaves in the directory of a linked worktree (gitrepository-layout(5)).
	Commondir bool
}

// Classify decides what a directory is from its .git entry. Any .git entry,
// even a broken one, makes it something other than KindNone, so the walk
// does not go into repositories or worktrees.
func Classify(g GitEntry) Kind {
	switch {
	case !g.Exists:
		return KindNone
	case g.Dir:
		return KindRepo
	case !g.GitdirIsDir:
		return KindBroken
	case g.Commondir:
		return KindLinked
	}
	return KindRepo
}

// ParseGitFile reads the content of a .git file, "gitdir: <path>", and
// returns the path in goos's native form. A relative path, as in the .bare
// layout's "gitdir: ./.bare", is resolved against dir, the directory that
// holds the .git file. Spaces around the path and a trailing \r\n are
// ignored. It reports false for anything else.
func ParseGitFile(content []byte, dir, goos string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !ok {
		return "", false
	}
	p := strings.TrimSpace(rest)
	if p == "" || strings.ContainsAny(p, "\r\n") {
		return "", false
	}
	if isAbs(goos, p) {
		return clean(goos, p), true
	}
	return join(goos, dir, p), true
}

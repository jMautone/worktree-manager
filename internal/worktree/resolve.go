package worktree

import (
	"errors"
	"fmt"
)

// ErrNoCurrent means `@` was asked for where no worktree is current.
var ErrNoCurrent = errors.New("not inside a worktree")

// NotFoundError means no worktree has the target as its name or branch.
type NotFoundError struct {
	Target string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("no worktree named %q", e.Target) }

// AmbiguousError means more than one worktree matched the target in the step
// that matched.
type AmbiguousError struct {
	Target     string
	Candidates []Worktree
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches more than one worktree", e.Target)
}

// Resolve picks the destination of `wt cd` among the worktrees of one
// repository, as Build returns them.
//
//   - `^` is the main worktree; for a bare repository, the bare repository.
//   - `@` is the current worktree, or ErrNoCurrent.
//   - Anything else is the worktree whose Name equals target and, only when
//     none does, the worktree whose Branch equals it.
//
// Comparisons are exact and case-sensitive on every OS: branch names are, and
// a per-OS rule for names would be one more emergent difference. `-` is not
// resolved here: it needs no repository.
func Resolve(ws []Worktree, target string) (Worktree, error) {
	switch target {
	case "^":
		for _, w := range ws {
			if w.Main {
				return w, nil
			}
		}
		return Worktree{}, &NotFoundError{Target: target}
	case "@":
		for _, w := range ws {
			if w.Current {
				return w, nil
			}
		}
		return Worktree{}, ErrNoCurrent
	case "":
		// Detached and bare worktrees have no branch, which is not a
		// branch named "".
		return Worktree{}, &NotFoundError{Target: target}
	}
	for _, field := range []func(Worktree) string{
		func(w Worktree) string { return w.Name },
		func(w Worktree) string { return w.Branch },
	} {
		var matches []Worktree
		for _, w := range ws {
			if field(w) == target {
				matches = append(matches, w)
			}
		}
		switch len(matches) {
		case 0:
			continue
		case 1:
			return matches[0], nil
		default:
			return Worktree{}, &AmbiguousError{Target: target, Candidates: matches}
		}
	}
	return Worktree{}, &NotFoundError{Target: target}
}

// Hit is a worktree found among the worktrees of several repositories:
// Group is the index of its repository in the groups ResolveOthers got.
type Hit struct {
	Group    int
	Worktree Worktree
}

// OthersAmbiguousError means more than one linked worktree of the other
// repositories matched the target in the step that matched. Hits are in
// the order of the groups.
type OthersAmbiguousError struct {
	Target string
	Hits   []Hit
}

func (e *OthersAmbiguousError) Error() string {
	return fmt.Sprintf("%q matches more than one worktree", e.Target)
}

// ResolveOthers resolves target among the linked worktrees of several
// repositories, one group per repository, as Build returns them. Main
// worktrees and bare repositories are not candidates: they are reached by
// the repository's name. Like Resolve, it matches the Name and, only when
// no worktree of any group has it, the Branch, exactly; each step looks at
// every group at once. No match is a *NotFoundError.
func ResolveOthers(groups [][]Worktree, target string) (Hit, error) {
	if target == "" {
		return Hit{}, &NotFoundError{Target: target}
	}
	for _, field := range []func(Worktree) string{
		func(w Worktree) string { return w.Name },
		func(w Worktree) string { return w.Branch },
	} {
		var hits []Hit
		for g, ws := range groups {
			for _, w := range ws {
				if !w.Main && !w.Bare && field(w) == target {
					hits = append(hits, Hit{Group: g, Worktree: w})
				}
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil
		default:
			return Hit{}, &OthersAmbiguousError{Target: target, Hits: hits}
		}
	}
	return Hit{}, &NotFoundError{Target: target}
}

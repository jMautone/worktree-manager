package worktree

import "fmt"

// SyncState is what wt sync read of a worktree before deciding what to do
// with it.
type SyncState struct {
	Missing    bool   // its directory does not exist
	Path       string // for the reason of a missing directory
	Detached   bool
	InProgress string // "rebase", "merge", "cherry-pick", "revert" or ""
	Dirty      bool   // changes to tracked files, staged or not
	Contains   bool   // the base's commit is HEAD or an ancestor of it
}

// SyncAction is what wt sync does with a worktree.
type SyncAction int

const (
	SyncSkip SyncAction = iota
	SyncUpToDate
	SyncRebase
)

// DecideSync decides what wt sync does with a worktree in state s and, for
// SyncSkip, why. A worktree whose directory is gone is not asked anything
// else. An operation in progress comes before a detached HEAD: git reports
// a worktree stopped in a rebase as detached, and the rebase is the reason.
func DecideSync(s SyncState) (SyncAction, string) {
	switch {
	case s.Missing:
		return SyncSkip, s.Path + " does not exist"
	case s.InProgress != "":
		return SyncSkip, "a " + s.InProgress + " is in progress"
	case s.Detached:
		return SyncSkip, "HEAD is detached"
	case s.Dirty:
		return SyncSkip, "has uncommitted changes"
	case s.Contains:
		return SyncUpToDate, ""
	}
	return SyncRebase, ""
}

// SyncOutcome is how the sync of one worktree ended.
type SyncOutcome int

const (
	Rebased SyncOutcome = iota
	UpToDate
	Skipped
	Conflict
	Failed
)

// SyncExit is the exit code of wt sync for outcomes: 1 when a rebase failed,
// else 6 when one had a conflict, else 5 when a worktree was skipped, else 0.
func SyncExit(outcomes []SyncOutcome) int {
	code := 0
	for _, o := range outcomes {
		switch o {
		case Failed:
			return 1
		case Conflict:
			code = 6
		case Skipped:
			if code == 0 {
				code = 5
			}
		}
	}
	return code
}

// SyncTargets is the worktrees wt sync --all syncs among ws, in their
// order: every one but the main worktree, a bare repository and those
// whose HEAD is detached.
func SyncTargets(ws []Worktree) []Worktree {
	var out []Worktree
	for _, w := range ws {
		if w.Main || w.Bare || w.Detached {
			continue
		}
		out = append(out, w)
	}
	return out
}

// SyncLine is the line wt sync prints for a worktree: detail is the reason
// of a skip or git's message of a failure. Under dryRun, a rebase is only
// announced; the other outcomes read the same.
func SyncLine(label, base string, o SyncOutcome, detail string, dryRun bool) string {
	switch o {
	case Rebased:
		if dryRun {
			return fmt.Sprintf("would rebase %s onto %s", label, base)
		}
		return fmt.Sprintf("rebased %s onto %s", label, base)
	case UpToDate:
		return fmt.Sprintf("%s is up to date with %s", label, base)
	case Skipped:
		return fmt.Sprintf("skipped %s: %s", label, detail)
	case Conflict:
		return fmt.Sprintf("conflict in %s: rebase onto %s aborted", label, base)
	default:
		return fmt.Sprintf("failed %s: %s", label, detail)
	}
}

// SyncHint is the hint wt sync writes, after the summary, for a worktree
// that was not synced; path is the worktree's.
func SyncHint(label, base, path string, o SyncOutcome, detail string) string {
	if o == Conflict {
		return fmt.Sprintf("%s: conflicts with %s; run \"git rebase %s\" in %s to resolve", label, base, base, path)
	}
	return label + ": " + detail
}

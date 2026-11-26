package worktree

import "fmt"

// MergeTarget is the default target of wt merge for base: the base without
// its "<remote>/" prefix when it names a remote-tracking branch of one of
// remotes, the longest that matches, as FetchRemote picks it; otherwise the
// base itself.
func MergeTarget(base string, remotes []string) string {
	if r, ok := FetchRemote(base, remotes); ok {
		return base[len(r)+1:]
	}
	return base
}

// MergeState is what wt merge read of the worktree and its target before
// deciding whether it can merge.
type MergeState struct {
	Detached    bool
	InProgress  string // "rebase", "merge", "cherry-pick", "revert" or ""
	Changes     bool   // changes to tracked files or untracked files
	TargetFound bool   // the target is a local branch
	TargetIsOwn bool   // the target is the worktree's own branch
	TargetDirty string // the path of the worktree that has the target checked out with changes, or ""
	NoRebase    bool
	Contains    bool // the target's commit is HEAD or an ancestor of it
}

// MergeBlock is why wt merge cannot go on: an exit code of the contract, a
// message and its hints.
type MergeBlock struct {
	Code  int
	Msg   string
	Hints []string
}

// CheckMerge is the first check that blocks merging branch into target, or
// nil. An operation in progress comes before a detached HEAD: git reports a
// worktree stopped in a rebase as detached, and the rebase is the reason.
func CheckMerge(branch, target string, s MergeState) *MergeBlock {
	switch {
	case s.InProgress != "":
		return &MergeBlock{Code: 5, Msg: "a " + s.InProgress + " is in progress"}
	case s.Detached:
		return &MergeBlock{Code: 5, Msg: "HEAD is detached; check out a branch to merge it"}
	case !s.TargetFound:
		return &MergeBlock{Code: 3, Msg: fmt.Sprintf("target %q is not a local branch", target)}
	case s.TargetIsOwn:
		return &MergeBlock{Code: 2, Msg: fmt.Sprintf("cannot merge %s into itself", branch)}
	case s.Changes:
		return &MergeBlock{Code: 5, Msg: "the worktree has modified or untracked files", Hints: []string{"commit or stash them first"}}
	case s.TargetDirty != "":
		return &MergeBlock{Code: 5, Msg: fmt.Sprintf("%s is checked out in %s, which has uncommitted changes", target, s.TargetDirty)}
	case s.NoRebase && !s.Contains:
		return &MergeBlock{Code: 5, Msg: fmt.Sprintf("%s does not contain %s", branch, target), Hints: []string{"run it without --no-rebase to rebase first"}}
	}
	return nil
}

// MergeStep is a step of wt merge that prints a line.
type MergeStep int

const (
	MergeRebased MergeStep = iota
	MergeMerged
	MergeAlready
)

// MergeLine is the line wt merge prints for step; under dryRun, the rebase
// and the merge are only announced.
func MergeLine(step MergeStep, branch, target string, dryRun bool) string {
	switch step {
	case MergeRebased:
		if dryRun {
			return fmt.Sprintf("would rebase %s onto %s", branch, target)
		}
		return fmt.Sprintf("rebased %s onto %s", branch, target)
	case MergeMerged:
		if dryRun {
			return fmt.Sprintf("would merge %s into %s", branch, target)
		}
		return fmt.Sprintf("merged %s into %s", branch, target)
	}
	return fmt.Sprintf("%s is already in %s", branch, target)
}

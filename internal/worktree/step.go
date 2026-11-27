package worktree

import (
	"fmt"
	"strings"
)

// StepKind is a subcommand of wt step: one step of the pipeline of wt merge.
type StepKind int

const (
	StepCommit StepKind = iota
	StepSquash
	StepRebase
	StepPush
)

// String is the subcommand's name, as wt step takes it and wt.step.v1
// writes it.
func (k StepKind) String() string {
	switch k {
	case StepCommit:
		return "commit"
	case StepSquash:
		return "squash"
	case StepRebase:
		return "rebase"
	}
	return "push"
}

// Commits reports whether the step makes a commit: commit and squash.
func (k StepKind) Commits() bool { return k == StepCommit || k == StepSquash }

// StepState is what a step read of the worktree and its target before
// deciding what to do. MergeState carries the state of the worktree and of
// the target; its Changes and NoRebase are not used.
type StepState struct {
	MergeState
	Pending     bool // pending changes: tracked or untracked, as wt remove counts them
	Tracked     bool // changes to tracked files: what blocks rebase and push
	Own         int  // commits of the branch that the target does not contain
	InTarget    bool // HEAD is the target's commit or an ancestor of it
	MessageFlag bool // -m was given
	Terminal    bool // the standard input is a terminal and --json was not given
	DryRun      bool
}

// CheckStep is the first check that blocks the step, in the order of the
// spec, or nil. The target is checked only by the steps that take one.
func CheckStep(kind StepKind, branch, target string, s StepState) *MergeBlock {
	switch {
	case s.InProgress != "":
		return &MergeBlock{Code: 5, Msg: "a " + s.InProgress + " is in progress"}
	case s.Detached:
		return &MergeBlock{Code: 5, Msg: "HEAD is detached; check out a branch first"}
	}
	if kind != StepCommit {
		switch {
		case !s.TargetFound:
			return &MergeBlock{Code: 3, Msg: fmt.Sprintf("target %q is not a local branch", target)}
		case s.TargetIsOwn:
			return &MergeBlock{Code: 2, Msg: fmt.Sprintf("the target cannot be %s itself", branch)}
		}
	}
	if kind == StepRebase || kind == StepPush {
		if s.Tracked {
			return &MergeBlock{Code: 5, Msg: "the worktree has uncommitted changes to tracked files", Hints: []string{"wt step commit commits them"}}
		}
	}
	if kind == StepPush {
		switch {
		case s.TargetDirty != "":
			return &MergeBlock{Code: 5, Msg: fmt.Sprintf("%s is checked out in %s, which has uncommitted changes", target, s.TargetDirty)}
		case !s.Contains && !s.InTarget:
			return &MergeBlock{Code: 5, Msg: fmt.Sprintf("%s does not contain %s", branch, target), Hints: []string{"wt step rebase rebases it first"}}
		}
	}
	if NeedsMessage(kind, s) && !s.Terminal {
		return MessageBlock()
	}
	return nil
}

// MessageBlock is the error of a commit that needs a message when there is
// no terminal to ask for one in.
func MessageBlock() *MergeBlock {
	return &MergeBlock{Code: 2, Msg: "a commit message is needed", Hints: []string{"-m <msg> gives it; without -m, git's editor opens in a terminal"}}
}

// StepNoop is the line a step prints when it has nothing to do, and true;
// or "", false.
func StepNoop(kind StepKind, branch, target string, s StepState) (string, bool) {
	switch kind {
	case StepCommit:
		if !s.Pending {
			return "nothing to commit", true
		}
	case StepSquash:
		if !s.Pending && s.Own <= 1 {
			return "nothing to squash", true
		}
	case StepRebase:
		if s.Contains {
			return fmt.Sprintf("%s already contains %s", branch, target), true
		}
	case StepPush:
		if s.InTarget {
			return MergeLine(MergeAlready, branch, target, false), true
		}
	}
	return "", false
}

// SquashAsCommit reports whether a squash is only a commit of the pending
// changes: the branch has no commit of its own.
func SquashAsCommit(s StepState) bool { return s.Own == 0 }

// NeedsMessage reports whether the step is going to ask for a message: it
// makes a commit, -m was not given, and it is not a dry run.
func NeedsMessage(kind StepKind, s StepState) bool {
	if !kind.Commits() || s.MessageFlag || s.DryRun {
		return false
	}
	_, noop := StepNoop(kind, "", "", s)
	return !noop
}

// SquashMessage is the message the editor of a squash opens with: a comment
// line with the count, then the full messages of the commits squashed,
// oldest first, each followed by a blank line.
func SquashMessage(branch string, messages []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Squashing %d commits of %s. Lines starting with '#' are ignored.\n\n", len(messages), branch)
	for _, m := range messages {
		b.WriteString(strings.TrimRight(m, "\n") + "\n\n")
	}
	return b.String()
}

// StepLine is the line a step prints once done, or under dryRun what it
// would do. short is the new commit's short id; n the commits squashed.
func StepLine(kind StepKind, branch, target, short string, n int, dryRun bool) string {
	switch kind {
	case StepCommit:
		if dryRun {
			return "would commit pending changes to " + branch
		}
		return fmt.Sprintf("committed %s to %s", short, branch)
	case StepSquash:
		if dryRun {
			return fmt.Sprintf("would squash %d commits of %s", n, branch)
		}
		return fmt.Sprintf("squashed %d commits of %s into %s", n, branch, short)
	case StepRebase:
		return MergeLine(MergeRebased, branch, target, dryRun)
	}
	return MergeLine(MergeMerged, branch, target, dryRun)
}

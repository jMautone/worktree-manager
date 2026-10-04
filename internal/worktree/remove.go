package worktree

import "fmt"

// RemovePlan is everything wt remove decided, before any effect. The same
// plan feeds --dry-run, --json and the removal itself.
type RemovePlan struct {
	Worktree     Worktree // the target, as Build returns it
	Force        bool     // git worktree remove --force
	Branch       string   // "" when HEAD is detached
	DeleteBranch bool
	KeepReason   string // "" or "not merged into origin/main"
	CD           string // the main worktree's path when the shell moves; "" when not
}

// MainWorktreeError means a command that changes a linked worktree was
// given the main worktree. Op is "remove", "lock" or "unlock".
type MainWorktreeError struct {
	Op string
}

func (e *MainWorktreeError) Error() string {
	if e.Op == "remove" {
		return "cannot remove the main worktree"
	}
	return "the main worktree cannot be " + e.Op + "ed"
}

// LockedError means the worktree is locked. Already is set when the command
// was wt lock itself.
type LockedError struct {
	Name    string
	Reason  string // "" when locked without one
	Already bool
}

func (e *LockedError) Error() string {
	msg := fmt.Sprintf("worktree %q is locked", e.Name)
	if e.Already {
		msg = fmt.Sprintf("worktree %q is already locked", e.Name)
	}
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// NotLockedError means wt unlock was given a worktree that is not locked.
type NotLockedError struct {
	Name string
}

func (e *NotLockedError) Error() string { return fmt.Sprintf("worktree %q is not locked", e.Name) }

// ContainsError means the worktree to remove contains another worktree,
// which git worktree remove --force would delete with it.
type ContainsError struct {
	Outer, Inner Worktree
}

func (e *ContainsError) Error() string {
	return fmt.Sprintf("worktree %q contains the worktree %q at %s", e.Outer.Name, e.Inner.Name, e.Inner.Path)
}

// Contains reports whether the directory dir contains the path p, or is p,
// comparing by whole components (/a/repo does not contain /a/repo2) and
// ignoring case on darwin and windows, as Build does for the current
// worktree.
func Contains(dir, p, goos string) bool {
	fold := goos == "darwin" || goos == "windows"
	return contains(components(native(dir, goos), goos), components(native(p, goos), goos), fold)
}

// CheckRemovable applies, in this order, the checks of wt remove that need
// nothing but the worktree list: the main worktree (*MainWorktreeError), a
// lock (*LockedError) and another worktree inside it (*ContainsError). The
// order puts first what not even --force gets past. ws is walked in Build's
// order, so with several nested worktrees the error always names the same.
func CheckRemovable(w Worktree, ws []Worktree, goos string) error {
	if w.Main {
		return &MainWorktreeError{Op: "remove"}
	}
	if w.Locked {
		return &LockedError{Name: w.Name, Reason: w.LockedReason}
	}
	for _, x := range ws {
		if x.Path != w.Path && Contains(w.Path, x.Path, goos) {
			return &ContainsError{Outer: w, Inner: x}
		}
	}
	return nil
}

// Merged is what the edge read to judge whether a branch is merged.
type Merged struct {
	Base       string // the base as written; "" when there is none
	InBase     bool   // the branch's tip is the base's commit or an ancestor of it
	InUpstream bool   // the same, with the branch's upstream
}

// BranchOutcome decides what wt remove does with the worktree's branch:
// --keep-branch keeps it, -D deletes it, and otherwise it is deleted only
// when merged. keepReason says why a branch is kept when that was not asked
// for. A detached worktree (branch "") has no branch to delete.
func BranchOutcome(branch string, keep, forceDelete bool, m Merged) (del bool, keepReason string) {
	switch {
	case branch == "" || keep:
		return false, ""
	case forceDelete || m.InBase || m.InUpstream:
		return true, ""
	case m.Base == "":
		return false, "not merged"
	}
	return false, "not merged into " + m.Base
}

// CheckLockable rejects the main worktree and a worktree already locked.
func CheckLockable(w Worktree) error {
	switch {
	case w.Main:
		return &MainWorktreeError{Op: "lock"}
	case w.Locked:
		return &LockedError{Name: w.Name, Reason: w.LockedReason, Already: true}
	}
	return nil
}

// CheckUnlockable rejects the main worktree and a worktree that is not
// locked.
func CheckUnlockable(w Worktree) error {
	switch {
	case w.Main:
		return &MainWorktreeError{Op: "unlock"}
	case !w.Locked:
		return &NotLockedError{Name: w.Name}
	}
	return nil
}

// Prunable returns the worktrees git reports as prunable, in Build's order.
// git never reports a locked worktree as prunable; a locked one is left out
// here as well.
func Prunable(ws []Worktree) []Worktree {
	var out []Worktree
	for _, w := range ws {
		if w.Prunable && !w.Locked {
			out = append(out, w)
		}
	}
	return out
}

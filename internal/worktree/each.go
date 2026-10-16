package worktree

// EachGroup is the worktrees of one repository that wt each visits.
type EachGroup struct {
	Repo      string     // display name of the repository; "" outside --all-repos
	Worktrees []Worktree // in the order of Build
}

// EachTarget is one worktree wt each runs the command in, or skips.
type EachTarget struct {
	Label    string // the worktree as wt cd takes it: "^", "feat", "api:^", "api:feat"
	Worktree Worktree
}

// EachPlan is what wt each does.
type EachPlan struct {
	Run  []EachTarget // in order: the groups', and within each its own
	Gone []EachTarget // directory does not exist: one warning each
}

// PlanEach decides which worktrees of groups wt each runs the command in,
// in the order given. A bare repository has no working tree and is left out
// silently. A worktree whose path is in gone goes to Gone instead, whatever
// git says of it (prunable, locked): what counts is that its directory does
// not exist, which the caller found out.
func PlanEach(groups []EachGroup, gone map[string]bool) EachPlan {
	var plan EachPlan
	for _, g := range groups {
		for _, w := range g.Worktrees {
			if w.Bare {
				continue
			}
			t := EachTarget{Label: Label(g.Repo, w), Worktree: w}
			if gone[w.Path] {
				plan.Gone = append(plan.Gone, t)
			} else {
				plan.Run = append(plan.Run, t)
			}
		}
	}
	return plan
}

// Label is w as wt cd takes it: ^ for the main worktree and its Name for
// any other, after "<repo>:" when repo is not empty.
func Label(repo string, w Worktree) string {
	label := w.Name
	if w.Main {
		label = "^"
	}
	if repo != "" {
		label = repo + ":" + label
	}
	return label
}

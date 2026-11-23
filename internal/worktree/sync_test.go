package worktree

import (
	"slices"
	"testing"
)

func TestDecideSync(t *testing.T) {
	tests := []struct {
		name   string
		state  SyncState
		action SyncAction
		reason string
	}{
		{"missing and dirty", SyncState{Missing: true, Path: "/w/feat", Dirty: true}, SyncSkip, "/w/feat does not exist"},
		{"detached and dirty", SyncState{Detached: true, Dirty: true}, SyncSkip, "HEAD is detached"},
		{"rebase in progress, so detached", SyncState{Detached: true, InProgress: "rebase"}, SyncSkip, "a rebase is in progress"},
		{"rebase in progress and dirty", SyncState{InProgress: "rebase", Dirty: true}, SyncSkip, "a rebase is in progress"},
		{"merge in progress", SyncState{InProgress: "merge"}, SyncSkip, "a merge is in progress"},
		{"dirty and contained", SyncState{Dirty: true, Contains: true}, SyncSkip, "has uncommitted changes"},
		{"contained", SyncState{Contains: true}, SyncUpToDate, ""},
		{"behind", SyncState{}, SyncRebase, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, reason := DecideSync(tt.state)
			if action != tt.action || reason != tt.reason {
				t.Errorf("DecideSync = %v, %q; want %v, %q", action, reason, tt.action, tt.reason)
			}
		})
	}
}

func TestSyncExit(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []SyncOutcome
		want     int
	}{
		{"none", nil, 0},
		{"rebased and up to date", []SyncOutcome{Rebased, UpToDate}, 0},
		{"skipped", []SyncOutcome{Rebased, Skipped}, 5},
		{"skipped and conflict", []SyncOutcome{Skipped, Conflict, Skipped}, 6},
		{"conflict then skipped", []SyncOutcome{Conflict, Skipped}, 6},
		{"failed", []SyncOutcome{Conflict, Failed, Skipped}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SyncExit(tt.outcomes); got != tt.want {
				t.Errorf("SyncExit = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSyncTargets(t *testing.T) {
	ws := []Worktree{
		{Name: "repo", Main: true, Branch: "main"},
		{Name: "feat", Branch: "feat"},
		{Name: "scratch", Detached: true},
		{Name: "fix", Branch: "fix"},
		{Name: ".bare", Bare: true},
	}
	var names []string
	for _, w := range SyncTargets(ws) {
		names = append(names, w.Name)
	}
	if want := []string{"feat", "fix"}; !slices.Equal(names, want) {
		t.Errorf("SyncTargets = %v, want %v", names, want)
	}
}

func TestSyncLine(t *testing.T) {
	tests := []struct {
		outcome SyncOutcome
		detail  string
		dryRun  bool
		want    string
	}{
		{Rebased, "", false, "rebased feat onto origin/main"},
		{Rebased, "", true, "would rebase feat onto origin/main"},
		{UpToDate, "", false, "feat is up to date with origin/main"},
		{UpToDate, "", true, "feat is up to date with origin/main"},
		{Skipped, "has uncommitted changes", false, "skipped feat: has uncommitted changes"},
		{Skipped, "has uncommitted changes", true, "skipped feat: has uncommitted changes"},
		{Conflict, "", false, "conflict in feat: rebase onto origin/main aborted"},
		{Failed, "error: oops", false, "failed feat: error: oops"},
	}
	for _, tt := range tests {
		if got := SyncLine("feat", "origin/main", tt.outcome, tt.detail, tt.dryRun); got != tt.want {
			t.Errorf("SyncLine(%v, %q, %v) = %q, want %q", tt.outcome, tt.detail, tt.dryRun, got, tt.want)
		}
	}
}

func TestSyncHint(t *testing.T) {
	if got, want := SyncHint("fix", "origin/main", "/w/fix", Conflict, ""), `fix: conflicts with origin/main; run "git rebase origin/main" in /w/fix to resolve`; got != want {
		t.Errorf("conflict hint = %q, want %q", got, want)
	}
	if got, want := SyncHint("feat", "origin/main", "/w/feat", Skipped, "has uncommitted changes"), "feat: has uncommitted changes"; got != want {
		t.Errorf("skip hint = %q, want %q", got, want)
	}
}

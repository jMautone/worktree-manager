package worktree

import (
	"strings"
	"testing"
)

func TestMergeTarget(t *testing.T) {
	tests := []struct {
		base    string
		remotes []string
		want    string
	}{
		{"origin/main", []string{"origin"}, "main"},
		{"main", []string{"origin"}, "main"},
		{"up/stream/main", []string{"up", "up/stream"}, "main"},
		{"up/main", []string{"up", "up/stream"}, "main"},
		{"origin/main", nil, "origin/main"},
	}
	for _, tt := range tests {
		if got := MergeTarget(tt.base, tt.remotes); got != tt.want {
			t.Errorf("MergeTarget(%q, %v) = %q, want %q", tt.base, tt.remotes, got, tt.want)
		}
	}
}

func TestCheckMerge(t *testing.T) {
	ok := MergeState{TargetFound: true}
	tests := []struct {
		name  string
		state MergeState
		code  int
		msg   string
		hint  string
	}{
		{"rebase in progress, so detached", MergeState{Detached: true, InProgress: "rebase", TargetFound: true}, 5, "a rebase is in progress", ""},
		{"detached", MergeState{Detached: true, TargetFound: true}, 5, "HEAD is detached", ""},
		{"target not found", MergeState{Changes: true}, 3, `"main"`, ""},
		{"own branch", MergeState{TargetFound: true, TargetIsOwn: true, Changes: true}, 2, "into itself", ""},
		{"changes", MergeState{TargetFound: true, Changes: true}, 5, "modified or untracked files", ""},
		{"target dirty", MergeState{TargetFound: true, TargetDirty: "/src/repo"}, 5, "main is checked out in /src/repo", ""},
		{"no rebase, behind", MergeState{TargetFound: true, NoRebase: true}, 5, "feat does not contain main", "--no-rebase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := CheckMerge("feat", "main", tt.state)
			if b == nil {
				t.Fatal("CheckMerge = nil")
			}
			if b.Code != tt.code || !strings.Contains(b.Msg, tt.msg) {
				t.Errorf("CheckMerge = %d %q; want %d containing %q", b.Code, b.Msg, tt.code, tt.msg)
			}
			if tt.hint != "" && (len(b.Hints) == 0 || !strings.Contains(b.Hints[0], tt.hint)) {
				t.Errorf("hints = %v, want one naming %q", b.Hints, tt.hint)
			}
		})
	}
	if b := CheckMerge("feat", "main", ok); b != nil {
		t.Errorf("CheckMerge(clean) = %+v, want nil", b)
	}
	if b := CheckMerge("feat", "main", MergeState{TargetFound: true, NoRebase: true, Contains: true}); b != nil {
		t.Errorf("CheckMerge(no rebase, contained) = %+v, want nil", b)
	}
}

func TestMergeLine(t *testing.T) {
	tests := []struct {
		step   MergeStep
		dryRun bool
		want   string
	}{
		{MergeRebased, false, "rebased feat onto main"},
		{MergeRebased, true, "would rebase feat onto main"},
		{MergeMerged, false, "merged feat into main"},
		{MergeMerged, true, "would merge feat into main"},
		{MergeAlready, false, "feat is already in main"},
		{MergeAlready, true, "feat is already in main"},
	}
	for _, tt := range tests {
		if got := MergeLine(tt.step, "feat", "main", tt.dryRun); got != tt.want {
			t.Errorf("MergeLine(%v, %v) = %q, want %q", tt.step, tt.dryRun, got, tt.want)
		}
	}
}

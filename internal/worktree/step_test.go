package worktree

import (
	"strings"
	"testing"
)

func TestCheckStep(t *testing.T) {
	ok := MergeState{TargetFound: true}
	tests := []struct {
		name  string
		kind  StepKind
		state StepState
		code  int
		msg   string
		hint  string
	}{
		{"rebase in progress, so detached", StepCommit, StepState{MergeState: MergeState{Detached: true, InProgress: "rebase"}}, 5, "a rebase is in progress", ""},
		{"detached", StepCommit, StepState{MergeState: MergeState{Detached: true}}, 5, "HEAD is detached", ""},
		{"target not found", StepSquash, StepState{}, 3, `"main"`, ""},
		{"own branch", StepRebase, StepState{MergeState: MergeState{TargetFound: true, TargetIsOwn: true}}, 2, "feat itself", ""},
		{"rebase with tracked changes", StepRebase, StepState{MergeState: ok, Tracked: true}, 5, "tracked files", "wt step commit"},
		{"push with tracked changes", StepPush, StepState{MergeState: MergeState{TargetFound: true, Contains: true}, Tracked: true}, 5, "tracked files", "wt step commit"},
		{"push, target dirty", StepPush, StepState{MergeState: MergeState{TargetFound: true, TargetDirty: "/src/repo", Contains: true}}, 5, "main is checked out in /src/repo", ""},
		{"push, behind", StepPush, StepState{MergeState: ok}, 5, "feat does not contain main", "wt step rebase"},
		{"commit without a terminal", StepCommit, StepState{Pending: true}, 2, "a commit message is needed", "-m"},
		{"squash without a terminal", StepSquash, StepState{MergeState: ok, Own: 3}, 2, "a commit message is needed", "-m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := CheckStep(tt.kind, "feat", "main", tt.state)
			if b == nil {
				t.Fatal("CheckStep = nil")
			}
			if b.Code != tt.code || !strings.Contains(b.Msg, tt.msg) {
				t.Errorf("CheckStep = %d %q; want %d containing %q", b.Code, b.Msg, tt.code, tt.msg)
			}
			if tt.hint != "" && (len(b.Hints) == 0 || !strings.Contains(b.Hints[0], tt.hint)) {
				t.Errorf("hints = %v, want one naming %q", b.Hints, tt.hint)
			}
		})
	}
	for _, tt := range []struct {
		name  string
		kind  StepKind
		state StepState
	}{
		{"commit takes no target", StepCommit, StepState{Pending: true, Terminal: true}},
		{"rebase with untracked files", StepRebase, StepState{MergeState: ok, Pending: true}},
		{"push, contained", StepPush, StepState{MergeState: MergeState{TargetFound: true, Contains: true}}},
		{"commit with -m", StepCommit, StepState{Pending: true, MessageFlag: true}},
		{"nothing to commit", StepCommit, StepState{}},
		{"dry run", StepSquash, StepState{MergeState: ok, Own: 3, DryRun: true}},
	} {
		if b := CheckStep(tt.kind, "feat", "main", tt.state); b != nil {
			t.Errorf("%s: CheckStep = %+v, want nil", tt.name, b)
		}
	}
}

func TestStepNoop(t *testing.T) {
	tests := []struct {
		name  string
		kind  StepKind
		state StepState
		want  string
	}{
		{"nothing to commit", StepCommit, StepState{}, "nothing to commit"},
		{"something to commit", StepCommit, StepState{Pending: true}, ""},
		{"no commit of its own", StepSquash, StepState{}, "nothing to squash"},
		{"one commit", StepSquash, StepState{Own: 1}, "nothing to squash"},
		{"two commits", StepSquash, StepState{Own: 2}, ""},
		{"one commit and pending changes", StepSquash, StepState{Own: 1, Pending: true}, ""},
		{"only pending changes", StepSquash, StepState{Pending: true}, ""},
		{"rebase, contained", StepRebase, StepState{MergeState: MergeState{Contains: true}}, "feat already contains main"},
		{"rebase, behind", StepRebase, StepState{}, ""},
		{"push, already in", StepPush, StepState{InTarget: true}, "feat is already in main"},
		{"push, ahead", StepPush, StepState{MergeState: MergeState{Contains: true}}, ""},
	}
	for _, tt := range tests {
		got, noop := StepNoop(tt.kind, "feat", "main", tt.state)
		if got != tt.want || noop != (tt.want != "") {
			t.Errorf("%s: StepNoop = %q, %v; want %q", tt.name, got, noop, tt.want)
		}
	}
	if !SquashAsCommit(StepState{Pending: true}) || SquashAsCommit(StepState{Own: 2}) {
		t.Error("SquashAsCommit is wrong")
	}
}

func TestNeedsMessage(t *testing.T) {
	tests := []struct {
		name  string
		kind  StepKind
		state StepState
		want  bool
	}{
		{"commit", StepCommit, StepState{Pending: true}, true},
		{"squash", StepSquash, StepState{Own: 2}, true},
		{"with -m", StepCommit, StepState{Pending: true, MessageFlag: true}, false},
		{"dry run", StepCommit, StepState{Pending: true, DryRun: true}, false},
		{"nothing to commit", StepCommit, StepState{}, false},
		{"nothing to squash", StepSquash, StepState{Own: 1}, false},
		{"rebase", StepRebase, StepState{}, false},
		{"push", StepPush, StepState{MergeState: MergeState{Contains: true}}, false},
	}
	for _, tt := range tests {
		if got := NeedsMessage(tt.kind, tt.state); got != tt.want {
			t.Errorf("%s: NeedsMessage = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSquashMessage(t *testing.T) {
	got := SquashMessage("feat", []string{"add form\n\nwith a body\n", "fix typo\n"})
	want := "# Squashing 2 commits of feat. Lines starting with '#' are ignored.\n\nadd form\n\nwith a body\n\nfix typo\n\n"
	if got != want {
		t.Errorf("SquashMessage = %q, want %q", got, want)
	}
}

func TestStepLine(t *testing.T) {
	tests := []struct {
		kind   StepKind
		dryRun bool
		want   string
	}{
		{StepCommit, false, "committed abc1234 to feat"},
		{StepCommit, true, "would commit pending changes to feat"},
		{StepSquash, false, "squashed 3 commits of feat into abc1234"},
		{StepSquash, true, "would squash 3 commits of feat"},
		{StepRebase, false, "rebased feat onto main"},
		{StepRebase, true, "would rebase feat onto main"},
		{StepPush, false, "merged feat into main"},
		{StepPush, true, "would merge feat into main"},
	}
	for _, tt := range tests {
		if got := StepLine(tt.kind, "feat", "main", "abc1234", 3, tt.dryRun); got != tt.want {
			t.Errorf("StepLine(%s, %v) = %q, want %q", tt.kind, tt.dryRun, got, tt.want)
		}
	}
	for k, name := range map[StepKind]string{StepCommit: "commit", StepSquash: "squash", StepRebase: "rebase", StepPush: "push"} {
		if k.String() != name {
			t.Errorf("String() = %q, want %q", k.String(), name)
		}
	}
}

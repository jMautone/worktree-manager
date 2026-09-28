package cli

import (
	"testing"

	"github.com/jMautone/worktree-manager/internal/worktree"
)

// git never reports a locked worktree as prunable (prune skips locked ones),
// so the combined state cannot be produced through a real repository.
func TestListRowsLockedAndPrunable(t *testing.T) {
	rows := listRows([]worktree.Worktree{{Name: "x", Path: "/x", Head: "0123456789", Locked: true, Prunable: true}})
	if got := rows[1][4].text; got != "locked,prunable" {
		t.Errorf("STATE = %q, want locked,prunable", got)
	}
}

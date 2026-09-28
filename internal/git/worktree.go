package git

import (
	"errors"
	"fmt"
	"strings"
)

// WorktreeEntry is one worktree as git reports it, before wt decides anything
// about it (current, main, order, native paths).
type WorktreeEntry struct {
	Path           string // absolute, with forward slashes on every OS
	Head           string // full commit id; empty for a bare repository
	Branch         string // short name; empty when detached or bare
	Detached       bool
	Bare           bool
	Locked         bool
	LockedReason   string // empty when locked without a reason
	Prunable       bool
	PrunableReason string
}

// ParseWorktreeList parses the output of `git worktree list --porcelain -z`.
//
// Each attribute is terminated by NUL and each record by an extra NUL. -z is
// required: without it a path containing a newline breaks the records. The
// lock and prune reasons share the attribute (`locked on usb drive`) and may
// be absent. Attributes this parser does not know are ignored, so a newer git
// does not break it.
func ParseWorktreeList(out []byte) ([]WorktreeEntry, error) {
	if len(out) == 0 {
		return nil, nil
	}
	if out[len(out)-1] != 0 {
		return nil, errors.New("malformed worktree list: output is not NUL-terminated")
	}
	var entries []WorktreeEntry
	inRecord := false
	for _, field := range strings.Split(string(out[:len(out)-1]), "\x00") {
		if field == "" {
			inRecord = false
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		if !inRecord {
			if key != "worktree" {
				return nil, fmt.Errorf("malformed worktree list: record starts with %q", key)
			}
			entries = append(entries, WorktreeEntry{Path: value})
			inRecord = true
			continue
		}
		e := &entries[len(entries)-1]
		switch key {
		case "HEAD":
			e.Head = value
		case "branch":
			e.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			e.Detached = true
		case "bare":
			e.Bare = true
		case "locked":
			e.Locked, e.LockedReason = true, value
		case "prunable":
			e.Prunable, e.PrunableReason = true, value
		}
	}
	return entries, nil
}

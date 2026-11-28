package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Snapshot is what a commit made by wt goes back to when it fails: the
// worktree's HEAD and its index, written as a tree. The worktree's files
// are never touched by a commit, so these two are all its state.
type Snapshot struct {
	Head  string
	Index string
}

// TakeSnapshot records HEAD and the index of the worktree at dir. The index
// must have no unmerged entries: wt refuses to commit with an operation in
// progress.
func TakeSnapshot(ctx context.Context, r Runner, dir string) (Snapshot, error) {
	head, err := ResolveCommit(ctx, r, dir, "HEAD")
	if err != nil {
		return Snapshot{}, err
	}
	out, err := r.Run(ctx, dir, "write-tree")
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Head: head, Index: strings.TrimSpace(string(out))}, nil
}

// Restore puts the branch back on s.Head, when moveBack, and the index
// back to s.Index, keeping the worktree's files as they are.
func (s Snapshot) Restore(ctx context.Context, r Runner, dir string, moveBack bool) error {
	if moveBack {
		if err := ResetSoft(ctx, r, dir, s.Head); err != nil {
			return err
		}
	}
	if _, err := r.Run(ctx, dir, "read-tree", s.Index); err != nil {
		return err
	}
	// read-tree leaves the index without the files' stat data, so that
	// every file looks modified until it is refreshed. A refresh that finds
	// changes exits with 1, which is not a failure.
	_, _ = r.Run(ctx, dir, "update-index", "-q", "--refresh")
	return nil
}

// StageAll stages every pending change of the worktree at dir: changes to
// tracked files and untracked files that are not ignored.
func StageAll(ctx context.Context, r Runner, dir string) error {
	_, err := r.Run(ctx, dir, "add", "-A")
	return err
}

// ResetSoft moves the branch checked out in dir to commit, keeping the
// index and the files.
func ResetSoft(ctx context.Context, r Runner, dir, commit string) error {
	_, err := r.Run(ctx, dir, "reset", "-q", "--soft", commit)
	return err
}

// MergeBase returns the best common ancestor of a and b.
func MergeBase(ctx context.Context, r Runner, dir, a, b string) (string, error) {
	out, err := r.Run(ctx, dir, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CountCommits returns how many commits head has that base does not.
func CountCommits(ctx context.Context, r Runner, dir, base, head string) (int, error) {
	out, err := r.Run(ctx, dir, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// Messages returns the full messages of the commits head has that base
// does not, oldest first.
func Messages(ctx context.Context, r Runner, dir, base, head string) ([]string, error) {
	out, err := r.Run(ctx, dir, "log", "--reverse", "--format=%B%x00", base+".."+head)
	if err != nil {
		return nil, err
	}
	var msgs []string
	for _, m := range strings.Split(string(out), "\x00") {
		if m = strings.Trim(m, "\n"); m != "" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// Commit commits the index of the worktree at dir with msg. The
// repository's own git hooks run. allowEmpty makes a commit with the same
// files as its parent, as the squash of commits that change nothing is.
func Commit(ctx context.Context, r Runner, dir, msg string, allowEmpty bool) error {
	args := []string{"commit", "-q", "-m", msg}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	_, err := r.Run(ctx, dir, args...)
	return err
}

// ErrEmptyMessage means git's editor was saved with no message: git made
// no commit.
var ErrEmptyMessage = errors.New("empty commit message")

// Terminal is how git reaches the user to open its editor: its
// environment and the terminal's files.
type Terminal struct {
	Env            []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// CommitEdit commits the index of the worktree at dir with the message the
// user writes in git's editor, which git chooses and opens in t. With
// initial, the editor opens with it; otherwise with git's own template.
// The repository's own git hooks run. allowEmpty is as for Commit.
//
// When git fails, the message it was given tells an empty message
// (ErrEmptyMessage) apart from any other failure, such as a hook of the
// repository, without reading git's output, which is translated.
func CommitEdit(ctx context.Context, r Runner, t Terminal, dir, initial string, allowEmpty bool) error {
	editmsg, err := gitPath(ctx, r, dir, "COMMIT_EDITMSG")
	if err != nil {
		return err
	}
	// A message left by an earlier commit would pass for this one's.
	if err := os.Remove(editmsg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	args := []string{"commit", "-q"}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	if initial != "" {
		f, err := os.CreateTemp(filepath.Dir(editmsg), "wt-squash-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = f.WriteString(initial)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		args = append(args, "--edit", "-F", f.Name())
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Env = dir, t.Env
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout = t.Stdin, t.Stdout
	cmd.Stderr = io.MultiWriter(t.Stderr, &stderr)
	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}
	if errors.Is(runErr, exec.ErrNotFound) {
		return ErrNotInstalled
	}
	if b, err := os.ReadFile(editmsg); err == nil && Blank(string(b)) {
		return ErrEmptyMessage
	}
	return &CommandError{Args: args, Stderr: strings.TrimSpace(stderr.String()), Err: runErr}
}

// Blank reports whether msg has no message once git's default cleanup
// removes its comment lines and its blank lines.
func Blank(msg string) bool {
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return false
		}
	}
	return true
}

// gitPath is the path git uses for name in the git directory of the
// worktree at dir, as an absolute path.
func gitPath(ctx context.Context, r Runner, dir, name string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--git-path", name)
	if err != nil {
		return "", err
	}
	p := filepath.FromSlash(strings.TrimRight(string(out), "\r\n"))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p, nil
}

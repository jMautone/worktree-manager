package cli

import (
	"context"
	"io"

	"github.com/jMautone/worktree-manager/internal/git"
)

// FailToStart makes the -x command fail to start until restore is called:
// with /bin/sh, a real one cannot be made to.
func FailToStart(err error) (restore func()) {
	saved := runCommand
	runCommand = func(string, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
		return 0, err
	}
	return func() { runCommand = saved }
}

// recorder is a git.Runner that records the arguments of every command.
type recorder struct {
	git.Runner
	calls *[][]string
}

func (r recorder) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	*r.calls = append(*r.calls, args)
	return r.Runner.Run(ctx, dir, args...)
}

// RecordGit records the arguments of every git command wt runs, until
// restore is called.
func RecordGit() (calls *[][]string, restore func()) {
	saved := gitRunner
	calls = new([][]string)
	gitRunner = func(environ []string) git.Runner { return recorder{saved(environ), calls} }
	return calls, func() { gitRunner = saved }
}

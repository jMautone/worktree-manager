package cli

import (
	"context"
	"errors"
	"io"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/hook"
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

// RecordRuns records the directory of every command -x or wt each runs,
// and still runs it, until restore is called.
func RecordRuns() (dirs *[]string, restore func()) {
	saved := runCommand
	dirs = new([]string)
	runCommand = func(dir, cmdline string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
		*dirs = append(*dirs, dir)
		return saved(dir, cmdline, env, stdin, stdout, stderr)
	}
	return dirs, func() { runCommand = saved }
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

// In a test, os.Executable is the test binary: a background hook started
// for real would run the package's tests again, detached. A test that
// starts one without RunBackgroundInProcess gets this warning instead.
func init() {
	startBackground = func(hook.Run, []string) error {
		return errors.New("a background hook in process needs cli.RunBackgroundInProcess")
	}
}

// RunBackgroundInProcess makes the background hooks run in process, with
// the loop of __run-hooks and to the end, before wt goes on, until restore
// is called: the tests of order and log contents do not wait for another
// process. The detached process itself is tested end to end.
func RunBackgroundInProcess() (restore func()) {
	saved := startBackground
	startBackground = func(r hook.Run, env []string) error {
		log, err := openLog(r.Log)
		if err != nil {
			return err
		}
		defer log.Close()
		runLogged(r, env, log)
		return nil
	}
	return func() { startBackground = saved }
}

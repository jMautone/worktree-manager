package cli

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/jMautone/worktree-manager/internal/hook"
)

func TestHookError(t *testing.T) {
	const noHooks = "--no-hooks runs the command without hooks"
	pre := hook.Run{Event: hook.PreCreate, Dir: "/src/repo", Log: "/src/repo/.git/wt/logs/feat/pre-create.log"}
	post := hook.Run{Event: hook.PostCreate, Dir: "/src/repo.worktrees/feat", Log: "/src/repo/.git/wt/logs/feat/post-create.log"}
	preCD := hook.Run{Event: hook.PreCD, Dir: "/src/repo.worktrees/feat"}
	for _, tc := range []struct {
		name        string
		r           hook.Run
		c           string
		code        int
		err         error
		interrupted bool
		json        bool
		want        *Error
	}{
		{"exits with 1", pre, "exit 1", 1, nil, false, false,
			&Error{Code: ExitHookFailed, Msg: "pre-create hook failed: exit 1 exited with 1", Hints: []string{noHooks}}},
		{"terminated by a signal", pre, "kill -INT $$", 130, nil, false, false,
			&Error{Code: ExitHookFailed, Msg: "pre-create hook failed: kill -INT $$ exited with 130", Hints: []string{noHooks}}},
		{"could not start", pre, "npm ci", 0, errors.New("chdir /x: no such file or directory"), false, false,
			&Error{Code: ExitHookFailed, Msg: "pre-create hook failed: npm ci could not start: chdir /x: no such file or directory", Hints: []string{noHooks}}},
		{"interrupted", preCD, "sleep 5", 0, nil, true, false,
			&Error{Code: ExitInterrupted, Msg: "interrupted: pre-cd hook did not finish", Hints: []string{noHooks}}},
		{"interrupted, though it exited with 0", preCD, "trap '' INT; sleep 1", 0, nil, true, false,
			&Error{Code: ExitInterrupted, Msg: "interrupted: pre-cd hook did not finish", Hints: []string{noHooks}}},
		{"post-create names the worktree", post, "exit 2", 2, nil, false, false,
			&Error{Code: ExitHookFailed, Msg: "post-create hook failed: exit 2 exited with 2",
				Hints: []string{"the worktree was created anyway: /src/repo.worktrees/feat"}}},
		{"with --json the log", post, "exit 1", 1, nil, false, true,
			&Error{Code: ExitHookFailed, Msg: "post-create hook failed: exit 1 exited with 1",
				Hints: []string{"the worktree was created anyway: /src/repo.worktrees/feat", "the output of the hook is in /src/repo/.git/wt/logs/feat/post-create.log"}}},
		{"interrupted with --json", pre, "sleep 5", 0, nil, true, true,
			&Error{Code: ExitInterrupted, Msg: "interrupted: pre-create hook did not finish",
				Hints: []string{noHooks, "the output of the hook is in /src/repo/.git/wt/logs/feat/pre-create.log"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := hookError(tc.r, tc.c, tc.code, tc.err, tc.interrupted, tc.json)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestBackground(t *testing.T) {
	saved := startBackground
	defer func() { startBackground = saved }()
	r := hook.Run{Event: hook.PostRemove, Dir: "/src/repo", Commands: []string{"echo bye"}, Log: "/src/repo/.git/wt/logs/feat/post-remove.log"}

	var stderr bytes.Buffer
	a := &app{env: Env{Stderr: &stderr, GOOS: "linux"}}
	var started []hook.Run
	startBackground = func(r hook.Run, env []string) error {
		started = append(started, r)
		return nil
	}
	a.background(r)
	if want := "==> post-remove  running in the background, log: /src/repo/.git/wt/logs/feat/post-remove.log\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if len(started) != 1 {
		t.Errorf("started %d processes", len(started))
	}

	stderr.Reset()
	a.background(hook.Run{Event: hook.PostCD})
	if stderr.Len() != 0 || len(started) != 1 {
		t.Errorf("an event with no commands started a process or wrote %q", stderr.String())
	}

	startBackground = func(hook.Run, []string) error { return errors.New("permission denied") }
	a.background(r)
	if want := "wt: warning: cannot start the post-remove hook: permission denied\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}

	stderr.Reset()
	a.json = true
	a.background(r)
	if stderr.Len() != 0 {
		t.Errorf("with --json, stderr = %q", stderr.String())
	}
}

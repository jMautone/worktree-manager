package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// StartDetached starts the program at path, with args (args[0] included),
// in dir, detached from wt's terminal, and does not wait for it. Its
// standard input is the null device, its output and errors go to out, and
// env is its whole environment. err is set when it could not be started.
//
// Detached means that the process outlives wt, and that neither an
// interrupt from the terminal (Ctrl-C) nor closing the terminal reaches
// it. How is up to each OS: see detach.
func StartDetached(path string, args []string, dir string, env []string, out *os.File) error {
	return detach(func() *exec.Cmd {
		return &exec.Cmd{Path: path, Args: args, Dir: dir, Env: env, Stdout: out, Stderr: out}
	})
}

// Creation flags of CreateProcess on Windows, and the error it returns when
// the job object a process runs in does not allow breakaway. They are
// written by value so that detachFlags is testable on every OS.
const (
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
	createNoWindow         = 0x08000000
	errorAccessDenied      = syscall.Errno(5)
)

// detachFlags are the creation flags of attempt n, from 0, to start a
// detached process on Windows, given the error of the previous attempt; ok
// is false when there is no other attempt to make.
//
//   - CREATE_NEW_PROCESS_GROUP: the process ignores Ctrl-C, and Ctrl-Break
//     sent to wt's group does not reach it.
//   - CREATE_NO_WINDOW: a console of its own, hidden, which its cmd.exe
//     inherit; no window opens, and the events of wt's console do not
//     reach it.
//   - CREATE_BREAKAWAY_FROM_JOB: out of the job object of wt, in case the
//     terminal ends the job when it closes. A job that does not allow it
//     makes CreateProcess fail with ERROR_ACCESS_DENIED, and the second
//     attempt goes without it.
func detachFlags(n int, prev error) (flags uint32, ok bool) {
	base := uint32(createNewProcessGroup | createNoWindow)
	switch {
	case n == 0:
		return base | createBreakawayFromJob, true
	case n == 1 && errors.Is(prev, errorAccessDenied):
		return base, true
	}
	return 0, false
}

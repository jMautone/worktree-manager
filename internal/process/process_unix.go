//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

const goos = "unix"

func setCmdLine(*exec.Cmd, string) {}

// exitCode is the command's exit status, or 128 plus the signal that ended
// it, as shells report it.
func exitCode(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}

// detach starts the command in a new session, with no controlling
// terminal: the SIGINT of Ctrl-C goes to the terminal's foreground process
// group and the SIGHUP of closing the terminal to its session, and the
// process is in neither. Once wt exits, init adopts it; no double fork is
// needed, since wt does not wait for it.
func detach(newCmd func() *exec.Cmd) error {
	cmd := newCmd()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

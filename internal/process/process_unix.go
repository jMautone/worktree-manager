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

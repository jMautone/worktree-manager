package process

import (
	"os"
	"os/exec"
	"syscall"
)

const goos = "windows"

// setCmdLine passes the command line to CreateProcess as built, instead of
// letting os/exec quote Args.
func setCmdLine(cmd *exec.Cmd, raw string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: raw}
}

// exitCode is the exit code of the process as is: Windows has no signals,
// and a process ended by Ctrl-C reports its own code (0xC000013A).
func exitCode(state *os.ProcessState) int {
	return state.ExitCode()
}

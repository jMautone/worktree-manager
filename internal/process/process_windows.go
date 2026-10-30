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

// detach starts the command with the creation flags detachFlags chooses,
// trying again without breakaway when the job does not allow it. Each
// attempt is a new *exec.Cmd: one that failed to start cannot be reused.
func detach(newCmd func() *exec.Cmd) error {
	var err error
	for n := 0; ; n++ {
		flags, ok := detachFlags(n, err)
		if !ok {
			return err
		}
		cmd := newCmd()
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		if err = cmd.Start(); err == nil {
			return cmd.Process.Release()
		}
	}
}

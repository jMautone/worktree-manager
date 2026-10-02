// Package process runs the command a user gives wt with -x: in the
// foreground, with wt's own terminal, through the OS's command interpreter.
//
// The interpreter is fixed per OS, the same one hooks will use: sh -c on
// macOS and Linux, cmd.exe on Windows. Never the user's interactive shell:
// a command must behave the same whoever runs it.
package process

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

// Command returns how to run cmdline with goos's interpreter: the program,
// its argv, and on windows the raw command line to pass to CreateProcess.
// comspec is the value of %ComSpec%; empty falls back to cmd.exe on the PATH.
//
// cmd.exe does not split its command line with the rules os/exec quotes
// arguments for, so on windows the line is built by hand: /d skips the
// AutoRun commands of the registry, and /s with the command in quotes makes
// cmd take everything between the first and the last quote as is.
func Command(goos, cmdline, comspec string) (path string, args []string, rawCmdLine string) {
	if goos != "windows" {
		return "/bin/sh", []string{"/bin/sh", "-c", cmdline}, ""
	}
	path = comspec
	if path == "" {
		path = "cmd.exe"
	}
	exe := path
	if strings.ContainsAny(exe, " \t") {
		exe = `"` + exe + `"`
	}
	return path, []string{path, "/d", "/s", "/c", cmdline}, exe + ` /d /s /c "` + cmdline + `"`
}

// Run runs cmdline in dir and waits for it. It returns the command's exit
// code, unchanged; err is set only when the command could not be started.
//
// The command gets stdin, stdout and stderr as given: when they are the
// terminal's files, it gets the terminal itself, with color and input. env
// is its whole environment.
//
// An interrupt from the terminal (Ctrl-C) reaches the whole process group:
// while the command runs, wt receives it and discards it, so that the
// command decides whether to end and wt reports its exit code. Notify and
// not Ignore: an ignored signal would be inherited by the command.
func Run(dir, cmdline string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	path, args, raw := Command(goos, cmdline, lookupEnv(env, "ComSpec"))
	if lp, err := exec.LookPath(path); err == nil {
		path = lp
	}
	cmd := &exec.Cmd{Path: path, Args: args, Dir: dir, Env: env, Stdin: stdin, Stdout: stdout, Stderr: stderr}
	setCmdLine(cmd, raw)

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	err := cmd.Wait()
	if cmd.ProcessState == nil {
		return 0, err
	}
	return exitCode(cmd.ProcessState), nil
}

// lookupEnv returns the value of key in env, ignoring case as Windows does.
func lookupEnv(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// Command fakeprog stands in for other programs in the end-to-end tests.
// TestMain builds it several times, and what it does depends on the name it
// was built as:
//
//   - git: appends its environment to the file $FAKEPROG_LOG, followed by a
//     line "--", and then runs the real git at $FAKEPROG_GIT with the same
//     arguments and exit code.
//   - wt: prints "fakeprog: wt", like any other program named wt (wt.exe is
//     Windows Terminal).
//   - printargs: prints each of its arguments on its own line, to show how a
//     command line was split.
//   - sleeper: creates the file "started" in its working directory and
//     waits 5 seconds, for a test to interrupt it, as cmd.exe has no sleep.
//
// It is a Go program and not a script so that it runs as wt.exe on Windows.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	switch name {
	case "wt":
		fmt.Println("fakeprog: wt")
	case "printargs":
		for _, arg := range os.Args[1:] {
			fmt.Println(arg)
		}
	case "sleeper":
		if err := os.WriteFile("started", nil, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "fakeprog:", err)
			os.Exit(1)
		}
		time.Sleep(5 * time.Second)
	case "git":
		if err := record(os.Getenv("FAKEPROG_LOG")); err != nil {
			fmt.Fprintln(os.Stderr, "fakeprog:", err)
			os.Exit(1)
		}
		cmd := exec.Command(os.Getenv("FAKEPROG_GIT"), os.Args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakeprog:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "fakeprog: unexpected name %q\n", name)
		os.Exit(1)
	}
}

func record(log string) error {
	if log == "" {
		return nil
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, kv := range os.Environ() {
		fmt.Fprintln(f, kv)
	}
	fmt.Fprintln(f, "--")
	return f.Close()
}

// Command fakeprog stands in for other programs in the shell tests. TestMain
// builds it twice, and what it does depends on the name it was built as:
//
//   - git: appends its environment to the file $FAKEPROG_LOG, followed by a
//     line "--", and then runs the real git at $FAKEPROG_GIT with the same
//     arguments and exit code.
//   - wt: prints "fakeprog: wt", like any other program named wt (wt.exe is
//     Windows Terminal).
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
)

func main() {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	switch name {
	case "wt":
		fmt.Println("fakeprog: wt")
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

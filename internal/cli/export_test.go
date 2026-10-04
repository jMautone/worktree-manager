package cli

import "io"

// FailToStart makes the -x command fail to start until restore is called:
// with /bin/sh, a real one cannot be made to.
func FailToStart(err error) (restore func()) {
	saved := runCommand
	runCommand = func(string, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
		return 0, err
	}
	return func() { runCommand = saved }
}

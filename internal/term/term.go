// Package term answers questions about the terminal wt writes to: whether
// stdout is a terminal, and whether output may carry ANSI color.
package term

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether f is a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// UseColor is the color rule: color only on a terminal, only when the
// NO_COLOR environment variable is unset or empty (https://no-color.org), and
// only without --no-color.
func UseColor(isTTY bool, noColorEnv string, noColorFlag bool) bool {
	return isTTY && noColorEnv == "" && !noColorFlag
}

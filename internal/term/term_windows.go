//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableANSI prepares f to interpret ANSI escape sequences and reports whether
// it can. It turns on virtual terminal processing for f's console; an old
// console that rejects the mode, or a handle that is not a console, cannot
// take color.
func EnableANSI(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

//go:build !windows

package term

import "os"

// EnableANSI prepares f to interpret ANSI escape sequences and reports whether
// it can. Unix terminals always interpret them.
func EnableANSI(f *os.File) bool { return true }

//go:build !windows && !linux

package process

import "syscall"

// getsid is the session id of the calling process.
func getsid() (int, error) {
	return syscall.Getsid(0)
}

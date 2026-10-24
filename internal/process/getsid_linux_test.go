package process

import "syscall"

// getsid is the session id of the calling process: the syscall package has
// no Getsid on Linux.
func getsid() (int, error) {
	sid, _, errno := syscall.RawSyscall(syscall.SYS_GETSID, 0, 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(sid), nil
}

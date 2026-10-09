//go:build !windows

package workspace

import (
	"fmt"
	"os"
	"syscall"
)

// IDOf returns the identity of the directory at path, following links: its
// device and inode.
func IDOf(path string) (FileID, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileID{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileID{}, fmt.Errorf("%s: no device and inode", path)
	}
	return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
}

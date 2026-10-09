package workspace

import (
	"os"

	"golang.org/x/sys/windows"
)

// IDOf returns the identity of the directory at path: its volume serial
// number and file index. The directory is opened without
// FILE_FLAG_OPEN_REPARSE_POINT, so a symbolic link or a junction gives the
// identity of its target; FILE_FLAG_BACKUP_SEMANTICS is what lets
// CreateFile open a directory.
func IDOf(path string) (FileID, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FileID{}, err
	}
	h, err := windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return FileID{}, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return FileID{}, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return FileID{
		Dev: uint64(info.VolumeSerialNumber),
		Ino: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, nil
}

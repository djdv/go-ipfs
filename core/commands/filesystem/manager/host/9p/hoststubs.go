package p9fsp

import (
	"syscall"

	ninelib "github.com/hugelgupf/p9/p9"
)

// hard link
func (*fid) Link(ninelib.File, string) error { return syscall.ENOSYS }

// device
func (*fid) Mknod(string, ninelib.FileMode, uint32, uint32, ninelib.UID, ninelib.GID) (ninelib.QID, error) {
	return ninelib.QID{}, syscall.ENOSYS
}

// TODO:
func (*fid) Renamed(ninelib.File, string)                {}
func (*fid) Rename(ninelib.File, string) error           { return syscall.ENOSYS }
func (*fid) RenameAt(string, ninelib.File, string) error { return syscall.ENOSYS }

// maybe
func (*fid) FSync() error { return syscall.ENOSYS }

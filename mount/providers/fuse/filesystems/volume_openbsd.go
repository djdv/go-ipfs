package fusecommon

import (
	"syscall"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"golang.org/x/sys/unix"
)

func init() { Statfs = statfsFreeBSD }

func statfsFreeBSD(path string, fStatfs *fuselib.Statfs_t) (error, int) {
	sysStat := &unix.Statfs_t{}
	if err := unix.Statfs(path, sysStat); err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			return err, int(errno)
		}
		return err, -fuselib.EACCES
	}

	// NOTE: These values are ignored by cgofuse
	// but fsid might be incorrect on some platforms too
	fStatfs.Fsid = uint64(sysStat.F_fsid.Val[0])<<32 | uint64(sysStat.F_fsid.Val[1])
	fStatfs.Flag = uint64(sysStat.F_flags)

	fStatfs.Bsize = uint64(sysStat.F_bsize)
	fStatfs.Blocks = sysStat.F_blocks
	fStatfs.Bfree = sysStat.F_bfree
	fStatfs.Bavail = uint64(sysStat.F_bavail)
	fStatfs.Files = sysStat.F_files
	fStatfs.Ffree = uint64(sysStat.F_ffree)
	fStatfs.Frsize = uint64(sysStat.F_bsize)
	fStatfs.Namemax = uint64(sysStat.F_namemax)
	return nil, OperationSuccess
}

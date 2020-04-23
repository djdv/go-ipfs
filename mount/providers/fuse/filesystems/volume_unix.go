// +build !windows

package fusecommon

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"golang.org/x/sys/unix"
)

func init() { Statfs = statfsUnix }

func statfsUnix(path string, fStatfs *fuselib.Statfs_t) (error, int) {
	sysStat := &unix.Statfs_t{}
	if err := unix.Statfs(path, sysStat); err != nil {
		return err
	}

	// NOTE: These values are ignored by cgofuse
	// but fsid might be incorrect on some platforms too
	fStatfs.Fsid = uint64(sysStat.Fsid.X__val[0])<<32 | uint64(sysStat.Fsid.X__val[1])
	fStatfs.Flag = uint64(sysStat.Flags)

	fStatfs.Bsize = uint64(sysStat.Bsize)
	fStatfs.Blocks = sysStat.Blocks
	fStatfs.Bfree = sysStat.Bfree
	fStatfs.Bavail = sysStat.Bavail
	fStatfs.Files = sysStat.Files
	fStatfs.Ffree = sysStat.Ffree
	fStatfs.Frsize = uint64(sysStat.Frsize)
	fStatfs.Namemax = uint64(sysStat.Namelen)
	return nil
}

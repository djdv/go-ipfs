package fusemount

import (
	"syscall"

	"github.com/billziss-gh/cgofuse/fuse"
)

//TODO: untested on darwin, struct is different
func (fs *FUSEIPFS) fuseFreeSize(fStatfs *fuse.Statfs_t, path string) error {
	sysStat := &syscall.Statfs_t{}
	if err := syscall.Statfs(path, sysStat); err != nil {
		return err
	}

	fStatfs.Fsid = uint64(sysStat.Fsid.Val[0])<<32 | uint64(sysStat.Fsid.Val[1])

	fStatfs.Bsize = uint64(sysStat.Bsize)
	fStatfs.Blocks = sysStat.Blocks
	fStatfs.Bfree = sysStat.Bfree
	fStatfs.Bavail = sysStat.Bavail
	fStatfs.Files = sysStat.Files
	fStatfs.Ffree = sysStat.Ffree
	fStatfs.Frsize = uint64(sysStat.Bsize) //TODO: review this; should be standard on this platform but needs to be checked again
	fStatfs.Flag = uint64(sysStat.Flags)
	fStatfs.Namemax = uint64(syscall.NAME_MAX)
	return nil
}

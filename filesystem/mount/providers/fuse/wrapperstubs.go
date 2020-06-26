//+build !nofuse

package fuse

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

// metadata methods that don't apply to our systems

func (fs *fileSystem) Access(path string, mask uint32) int {
	fs.log.Warnf("Access - Request {%X}%q", mask, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Setxattr(path string, name string, value []byte, flags int) int {
	fs.log.Warnf("Setxattr - Request {%X|%s|%d}%q", flags, name, len(value), path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Getxattr(path string, name string) (int, []byte) {
	fs.log.Warnf("Getxattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS, nil
}

func (fs *fileSystem) Removexattr(path string, name string) int {
	fs.log.Warnf("Removexattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Listxattr(path string, fill func(name string) bool) int {
	fs.log.Warnf("Listxattr - Request %q", path)
	return -fuselib.ENOSYS
}

// TODO: we could have these change for the entire system but that might be weird

func (fs *fileSystem) Chmod(path string, mode uint32) int {
	fs.log.Warnf("Chmod - Request {%X}%q", mode, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Chown(path string, uid uint32, gid uint32) int {
	fs.log.Warnf("Chown - Request {%d|%d}%q", uid, gid, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Utimens(path string, tmsp []fuselib.Timespec) int {
	fs.log.Warnf("Utimens - Request {%v}%q", tmsp, path)
	return -fuselib.ENOSYS
}

// no hard links
func (fs *fileSystem) Link(oldpath string, newpath string) int {
	fs.log.Warnf("Link - Request %q<->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

// syncing operations that generally don't apply if write operations don't apply
//  TODO: we need to utilize these for writable systems; ENOSYS for non writables

func (fs *fileSystem) Flush(path string, fh uint64) int {
	fs.log.Warnf("Flush - Request {%X}%q", fh, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Fsync(path string, datasync bool, fh uint64) int {
	fs.log.Warnf("Fsync - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

func (fs *fileSystem) Fsyncdir(path string, datasync bool, fh uint64) int {
	fs.log.Warnf("Fsyncdir - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

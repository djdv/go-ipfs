package fusecommon

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	config "github.com/ipfs/go-ipfs-config"
	logging "github.com/ipfs/go-log"
)

type SharedMethods struct{}

var log = logging.Logger("fuse/shared")

//
// generic stat of IPFS datastore, not tied to specific fuse path
//

func (*SharedMethods) Statfs(path string, stat *fuselib.Statfs_t) int {
	log.Debugf("Statfs - Request %q", path)

	target, err := config.DataStorePath("")
	if err != nil {
		log.Errorf("Statfs - Config err %q: %v", path, err)
		return -fuselib.ENOENT
	}

	goErr, errNo := Statfs(target, stat)
	if err != nil {
		log.Errorf("Statfs - err %q: %v", target, goErr)
	}
	return errNo
}

//
// metadata methods that don't apply to most of our systems
//

func (*SharedMethods) Access(path string, mask uint32) int {
	log.Warnf("Access - Request {%X}%q", mask, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Setxattr(path string, name string, value []byte, flags int) int {
	log.Warnf("Setxattr - Request {%X|%s|%d}%q", flags, name, len(value), path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Getxattr(path string, name string) (int, []byte) {
	log.Warnf("Getxattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS, nil
}

func (*SharedMethods) Removexattr(path string, name string) int {
	log.Warnf("Removexattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Listxattr(path string, fill func(name string) bool) int {
	log.Warnf("Listxattr - Request %q", path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Chmod(path string, mode uint32) int {
	log.Warnf("Chmod - Request {%X}%q", mode, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Chown(path string, uid uint32, gid uint32) int {
	log.Warnf("Chown - Request {%d|%d}%q", uid, gid, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Utimens(path string, tmsp []fuselib.Timespec) int {
	log.Warnf("Utimens - Request {%v}%q", tmsp, path)
	return -fuselib.ENOSYS
}

//
// write operations that don't apply to read only systems
//

func (*SharedMethods) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Warnf("Create - {%X|%X}%q", flags, mode, path)
	return -fuselib.ENOSYS, ErrorHandle
}

func (*SharedMethods) Mknod(path string, mode uint32, dev uint64) int {
	log.Warnf("Mknod - Request {%X|%d}%q", mode, dev, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Truncate(path string, size int64, fh uint64) int {
	log.Warnf("Truncate - Request {%X|%d}%q", fh, size, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Write(path string, buff []byte, ofst int64, fh uint64) int {
	log.Warnf("Write - Request {%X|%d|%d}%q", fh, len(buff), ofst, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Link(oldpath string, newpath string) int {
	log.Warnf("Link - Request %q<->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Unlink(path string) int {
	log.Warnf("Unlink - Request %q", path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Mkdir(path string, mode uint32) int {
	log.Warnf("Mkdir - Request {%X}%q", mode, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Rmdir(path string) int {
	log.Warnf("Rmdir - Request %q", path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Symlink(target string, newpath string) int {
	log.Warnf("Symlink - Request %q->%q", newpath, target)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Rename(oldpath string, newpath string) int {
	log.Warnf("Rename - Request %q->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

//
// syncing operations that generally don't apply if write operations don't apply
//

func (*SharedMethods) Flush(path string, fh uint64) int {
	log.Warnf("Flush - Request {%X}%q", fh, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Fsync(path string, datasync bool, fh uint64) int {
	log.Warnf("Fsync - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

func (*SharedMethods) Fsyncdir(path string, datasync bool, fh uint64) int {
	log.Warnf("Fsyncdir - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

//+build !nofuse

package fuse

import fuselib "github.com/billziss-gh/cgofuse/fuse"

func (fs *fileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Warnf("Create - {%X|%X}%q", flags, mode, path)

	// TODO: why is fuselib passing us flags and what are they?
	// both FUSE and SUS predefine what they should be (to Open)

	//return fs.open(path, fuselib.O_WRONLY|fuselib.O_CREAT|fuselib.O_TRUNC)

	// disabled until we parse relevant flags in open
	// fuse will do shenanigans to make this work
	return -fuselib.ENOSYS, errorHandle
}

func (fs *fileSystem) Mknod(path string, mode uint32, dev uint64) int {
	fs.log.Debugf("Mknod - Request {%X|%d}%q", mode, dev, path)
	if err := fs.intf.Make(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}
	return operationSuccess
}

func (fs *fileSystem) Mkdir(path string, mode uint32) int {
	fs.log.Debugf("Mkdir - Request {%X}%q", mode, path)

	if err := fs.intf.MakeDirectory(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *fileSystem) Symlink(target, newpath string) int {
	fs.log.Debugf("Symlink - Request %q->%q", newpath, target)

	if err := fs.intf.MakeLink(target, newpath); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

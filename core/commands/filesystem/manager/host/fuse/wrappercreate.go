//+build !nofuse

package fuse

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

func (fs *nodeBinding) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Warnf("Create - {%X|%X}%q", flags, mode, path)

	// TODO: why is fuselib passing us flags and what are they?
	// both FUSE and SUS predefine what they should be (to Open)

	//return nodeBinding.open(path, fuselib.O_WRONLY|fuselib.O_CREAT|fuselib.O_TRUNC)

	// disabled until we parse relevant flags in open
	// fuse will do shenanigans to make this work
	return -fuselib.ENOSYS, errorHandle
}

func (fs *nodeBinding) Mknod(path string, mode uint32, dev uint64) int {
	fs.log.Debugf("Mknod - HostRequest {%X|%d}%q", mode, dev, path)
	if err := fs.nodeInterface.Make(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}
	return operationSuccess
}

func (fs *nodeBinding) Mkdir(path string, mode uint32) int {
	fs.log.Debugf("Mkdir - HostRequest {%X}%q", mode, path)

	if err := fs.nodeInterface.MakeDirectory(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *nodeBinding) Symlink(target, newpath string) int {
	fs.log.Debugf("Symlink - HostRequest %q->%q", newpath, target)

	if err := fs.nodeInterface.MakeLink(target, newpath); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

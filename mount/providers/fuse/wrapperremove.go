package fuse

import fuselib "github.com/billziss-gh/cgofuse/fuse"

func (fs *fileSystem) Unlink(path string) int {
	fs.log.Debugf("Unlink - Request %q", path)

	if path == "/" {
		fs.log.Error(fuselib.Error(-fuselib.EPERM))
		return -fuselib.EPERM
	}

	if err := fs.intf.Remove(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *fileSystem) Rmdir(path string) int {
	fs.log.Debugf("Rmdir - Request %q", path)

	if err := fs.intf.RemoveDirectory(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

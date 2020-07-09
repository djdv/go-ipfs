//+build !nofuse

package fuse

import fuselib "github.com/billziss-gh/cgofuse/fuse"

func (fs *fuseInterface) Unlink(path string) int {
	fs.log.Debugf("Unlink - Request %q", path)

	if path == "/" {
		fs.log.Error(fuselib.Error(-fuselib.EPERM))
		return -fuselib.EPERM
	}

	if err := fs.nodeInterface.Remove(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *fuseInterface) Rmdir(path string) int {
	fs.log.Debugf("Rmdir - Request %q", path)

	if err := fs.nodeInterface.RemoveDirectory(path); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

//+build !nofuse

package fuse

import (
	"fmt"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
)

func interpretError(err error) errNo {
	if errIntf, ok := err.(filesystem.Error); ok {
		return kindToFuse[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to POSIX error %#v", err))
}

var kindToFuse = map[filesystem.Kind]errNo{
	filesystem.ErrorOther:            -fuselib.EIO,
	filesystem.ErrorInvalidItem:      -fuselib.EINVAL,
	filesystem.ErrorInvalidOperation: -fuselib.ENOSYS,
	filesystem.ErrorPermission:       -fuselib.EACCES,
	filesystem.ErrorIO:               -fuselib.EIO,
	filesystem.ErrorExist:            -fuselib.EEXIST,
	filesystem.ErrorNotExist:         -fuselib.ENOENT,
	filesystem.ErrorIsDir:            -fuselib.EISDIR,
	filesystem.ErrorNotDir:           -fuselib.ENOTDIR,
	filesystem.ErrorNotEmpty:         -fuselib.ENOTEMPTY,
}
